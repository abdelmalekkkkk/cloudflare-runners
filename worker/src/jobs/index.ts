import { type DrizzleSqliteDODatabase } from 'drizzle-orm/durable-sqlite';
import { jobsTable } from './schema.sql';
import { asc, eq } from 'drizzle-orm';

export namespace Jobs {
	export type Job = {
		ID: number;
		runID: number;
		repo: string;
		labels: string[];
		installationID: number;
		createdAt: string;
		startedAt: string | null;
		completedAt: string | null;
		status: 'completed' | 'in_progress' | 'queued' | 'waiting';
	};

	export const shouldTake = (job: Job) => {
		return job.labels.some((label) => label.startsWith('cf-'));
	};

	export const createQueue = (db: DrizzleSqliteDODatabase) => {
		return {
			upsert: upsert.bind(null, db),
			pending: pending.bind(null, db),
		};
	};

	const pending = async (db: DrizzleSqliteDODatabase) => {
		return await db.select().from(jobsTable).where(eq(jobsTable.status, 'queued')).orderBy(asc(jobsTable.createdAt));
	};

	const upsert = async (db: DrizzleSqliteDODatabase, job: Job) => {
		await db
			.insert(jobsTable)
			.values([job])
			.onConflictDoUpdate({
				target: jobsTable.ID,
				set: {
					startedAt: job.startedAt,
					completedAt: job.completedAt,
					status: job.status,
				},
			});
	};

	export type Queue = ReturnType<typeof createQueue>;
}
