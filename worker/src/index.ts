import { App } from './app';
import { Jobs } from './jobs';
import { DurableObject } from 'cloudflare:workers';
import { Runners } from './runners';
import { drizzle } from 'drizzle-orm/durable-sqlite';
import { migrate } from 'drizzle-orm/durable-sqlite/migrator';
import migrations from './migrations/migrations';

// TODO: find a way to reuse configurations to shave off some 10 seconds
export class Runner extends DurableObject<Env> {
	#container: Container;

	constructor(ctx: DurableObjectState, env: Env) {
		super(ctx, env);

		if (!ctx.container) {
			throw new Error('missing connected container');
		}

		this.#container = ctx.container;
	}

	start({ name, url, token, labels }: Runners.Params) {
		this.#container.start({
			env: {
				CF_RUNNER_NAME: name,
				CF_RUNNER_REPO_URL: url,
				CF_RUNNER_TOKEN: token,
				CF_RUNNER_LABELS: labels.join(),
			},
			enableInternet: true,
		});
	}

	running() {
		return this.#container.running;
	}
}

export class Orchestrator extends DurableObject<Env> {
	static identifier = 'orchestrator';
	static queueConfig = {
		batchDelay: 1000,
	};

	#queue: Jobs.Queue;
	#app: App.App;

	constructor(ctx: DurableObjectState, env: Env) {
		super(ctx, env);

		const db = drizzle(this.ctx.storage, { logger: false });

		this.#queue = Jobs.createQueue(db);
		this.#app = App.create(env);

		ctx.blockConcurrencyWhile(async () => {
			await migrate(db, migrations);
		});
	}

	static instance(env: Env) {
		return env.ORCHESTRATOR.getByName(Orchestrator.identifier);
	}

	async put(job: Jobs.Job) {
		await Promise.all([this.#queue.upsert(job), this.setAlarm()]);
	}

	private async setAlarm() {
		const currentAlarm = await this.ctx.storage.getAlarm();

		if (currentAlarm) {
			return;
		}

		await this.ctx.storage.setAlarm(new Date().getTime() + Orchestrator.queueConfig.batchDelay);
	}

	async alarm() {
		const jobs = await this.#queue.pending();

		await Promise.all(
			jobs.map(async (job) => {
				const { ID, repo, labels } = job;

				const token = await this.#app.getRunnerToken(job);

				const runner = this.env.RUNNER.get(this.env.RUNNER.idFromName(String(ID)));

				await runner.start({
					name: `cf-runner-${ID}`,
					url: `https://github.com/${repo}`,
					token,
					labels,
				});
			}),
		);
	}
}

export default {
	async fetch(request, env, ctx): Promise<Response> {
		const job = await App.create(env).handleWebhook(request);

		ctx.waitUntil(
			(async () => {
				if (!job) {
					return;
				}

				if (!Jobs.shouldTake(job)) {
					return;
				}

				await Orchestrator.instance(env).put(job);
			})(),
		);

		return new Response();
	},
} satisfies ExportedHandler<Env>;
