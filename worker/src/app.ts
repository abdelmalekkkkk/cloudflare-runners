import crypto from 'crypto';
import { App as OctoApp, Octokit } from 'octokit';
import { Config } from './config';
import { EmitterWebhookEvent } from '@octokit/webhooks';
import { Jobs } from './jobs';

export namespace App {
	export const create = (env: Env) => {
		let app: OctoApp | undefined = undefined;

		const getApp = async () => {
			if (app) {
				return app;
			}

			const config = await Config.load(env);

			const privateKey = crypto.createPrivateKey(config.pem).export({
				type: 'pkcs8',
				format: 'pem',
			});

			app = new OctoApp({
				appId: config.id,
				privateKey,
				webhooks: { secret: config.webhook_secret },
			});

			return app;
		};

		return {
			handleWebhook: handleWebhook.bind(null, getApp),
			getRunnerToken: getRunnerToken.bind(null, getApp),
		};
	};

	export type App = ReturnType<typeof create>;

	type GetApp = () => Promise<OctoApp>;

	const handleWebhook = async (getApp: GetApp, request: Request): Promise<Jobs.Job | null> => {
		const app = await getApp();

		const { promise, resolve } = Promise.withResolvers<{
			payload: EmitterWebhookEvent<'workflow_job'>['payload'];
			instance: Octokit;
		} | null>();

		app.webhooks.on('workflow_job', ({ payload, octokit }) =>
			resolve({
				payload,
				instance: octokit,
			}),
		);

		app.webhooks.onAny(() => {
			console.warn('Received an event other than workflow_job.queued or workflow_job.completed');
			resolve(null);
		});

		try {
			await app.webhooks.verifyAndReceive({
				id: request.headers.get('x-github-delivery')!,
				name: request.headers.get('x-github-event')!,
				signature: request.headers.get('x-hub-signature-256')!,
				payload: await request.text(),
			});
		} catch (e) {
			console.error('Webhook parsing/verification failed:', e);
			return null;
		}

		const event = await promise;

		if (!event) {
			return null;
		}

		const { payload } = event;

		const { installation, repository, workflow_job } = payload;

		const installationID = installation?.id;

		if (!installationID) {
			console.error('installation id not found in payload');
			return null;
		}

		return {
			ID: workflow_job.id,
			runID: workflow_job.run_id,
			repo: repository.full_name,
			labels: workflow_job.labels,
			installationID,
			createdAt: workflow_job.created_at,
			startedAt: workflow_job.started_at,
			completedAt: workflow_job.completed_at,
			status: workflow_job.status,
		};
	};

	const getRunnerToken = async (getApp: GetApp, job: Jobs.Job) => {
		const app = await getApp();

		const instance = await app.getInstallationOctokit(job.installationID);

		const [owner, repo] = job.repo.split('/');

		const {
			data: { token },
		} = await instance.request('POST /repos/{owner}/{repo}/actions/runners/registration-token', {
			owner,
			repo,
			headers: {
				'X-GitHub-Api-Version': '2022-11-28',
			},
		});

		return token;
	};
}
