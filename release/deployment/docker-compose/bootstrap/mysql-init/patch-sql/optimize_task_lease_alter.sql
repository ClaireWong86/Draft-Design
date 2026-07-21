ALTER TABLE `optimize_task`
  ADD COLUMN `lease_token` varchar(64) NOT NULL DEFAULT '' COMMENT 'Current worker lease token' AFTER `cancel_requested`,
  ADD COLUMN `lease_expires_at` timestamp(3) NULL DEFAULT NULL COMMENT 'Current worker lease expiry' AFTER `lease_token`,
  ADD COLUMN `attempt_count` int unsigned NOT NULL DEFAULT 0 COMMENT 'Worker claim attempts' AFTER `lease_expires_at`;
