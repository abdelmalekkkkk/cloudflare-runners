export namespace Jobs {
	export type Job = {
		ID: number;
		runID: number;
		repo: string;
		labels: string[];
		installationID: number;
	};

	export const shouldTake = (job: Job) => {
		return job.labels.some((label) => label.startsWith('cf-'));
	};
}
