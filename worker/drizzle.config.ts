import { defineConfig } from 'drizzle-kit';

export default defineConfig({
	out: './src/migrations',
	schema: './src/**/schema.sql.ts',
	dialect: 'sqlite',
	driver: 'durable-sqlite',
});
