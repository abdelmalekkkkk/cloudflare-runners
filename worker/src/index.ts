import { App } from './app';
import { Jobs } from './jobs';
import { DurableObject } from 'cloudflare:workers';
import { Runners } from './runners';

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
}

export default {
	async fetch(request, env, ctx): Promise<Response> {
		const app = await App.create(env);
		const job = await app.handleWebhook(request);

		ctx.waitUntil(
			(async () => {
				if (!job) {
					return;
				}

				if (!Jobs.shouldTake(job)) {
					return;
				}

				await env.QUEUE.send(job);
			})(),
		);

		return new Response();
	},

	async queue(batch, env) {
		const app = await App.create(env);

		for (const message of batch.messages) {
			const job = message.body as Jobs.Job;

			const { ID, repo, labels } = job;

			const token = await app.getRunnerToken(job);

			const runner = env.RUNNER.get(env.RUNNER.idFromName(String(ID)));

			await runner.start({
				name: `cf-runner-${ID}`,
				url: `https://github.com/${repo}`,
				token,
				labels,
			});
		}
	},
} satisfies ExportedHandler<Env>;
