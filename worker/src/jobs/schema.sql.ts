import { index, int, sqliteTable, text } from 'drizzle-orm/sqlite-core';

export const jobsTable = sqliteTable(
	'jobs',
	{
		ID: int().primaryKey(),
		runID: int().notNull(),
		repo: text().notNull(),
		labels: text({ mode: 'json' }).$type<string[]>().notNull(),
		installationID: int().notNull(),
		createdAt: text().notNull(),
		startedAt: text(),
		completedAt: text(),
		status: text({
			enum: ['completed', 'in_progress', 'queued', 'waiting'],
		}).notNull(),
	},
	(table) => [index('status_created_at_idx').on(table.status, table.createdAt)],
);
