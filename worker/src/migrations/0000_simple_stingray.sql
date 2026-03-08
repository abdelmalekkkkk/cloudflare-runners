CREATE TABLE `jobs` (
	`ID` integer PRIMARY KEY NOT NULL,
	`runID` integer NOT NULL,
	`repo` text NOT NULL,
	`labels` text NOT NULL,
	`installationID` integer NOT NULL,
	`createdAt` text NOT NULL,
	`startedAt` text,
	`completedAt` text,
	`status` text NOT NULL
);
--> statement-breakpoint
CREATE INDEX `status_created_at_idx` ON `jobs` (`status`,`createdAt`);