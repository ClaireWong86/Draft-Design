ALTER TABLE `optimize_task`
  ADD COLUMN `source` json DEFAULT NULL COMMENT 'Immutable optimization source snapshot' AFTER `source_id`;
