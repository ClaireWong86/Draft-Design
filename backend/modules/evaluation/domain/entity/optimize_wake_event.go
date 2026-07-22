// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

// OptimizeTaskWakeEvent asks workers to enqueue a durable OptimizeTask.
// MySQL lease (MarkRunning) remains the source of truth for claim ownership.
type OptimizeTaskWakeEvent struct {
	TaskID int64 `json:"task_id"`
}
