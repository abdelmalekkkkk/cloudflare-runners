import crypto from 'crypto';
import { App as OctoApp, Octokit } from 'octokit';
import { Config } from './config';
import { EmitterWebhookEvent } from '@octokit/webhooks';
import { Jobs } from './jobs';

export namespace App {
	export const create = async (env: Env) => {
		const config = await Config.load(env);

		const privateKey = crypto.createPrivateKey(config.pem).export({
			type: 'pkcs8',
			format: 'pem',
		});

		const app = new OctoApp({
			appId: config.id,
			privateKey,
			webhooks: { secret: config.webhook_secret },
		});

		return {
			handleWebhook: handleWebhook.bind(null, app),
			getRunnerToken: getRunnerToken.bind(null, app),
		};
	};

	const handleWebhook = async (app: OctoApp, request: Request): Promise<Jobs.Job | null> => {
		const { promise, resolve } = Promise.withResolvers<{
			payload: EmitterWebhookEvent<'workflow_job.queued'>['payload'];
			instance: Octokit;
		} | null>();

		app.webhooks.on('workflow_job.queued', ({ payload, octokit }) =>
			resolve({
				payload,
				instance: octokit,
			}),
		);

		app.webhooks.onAny(() => {
			console.warn('Received an event other than workflow_job.queued');
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
		};
	};

	const getRunnerToken = async (app: OctoApp, job: Jobs.Job) => {
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
