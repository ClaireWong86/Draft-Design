// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

import "time"

const (
	OptimizeTaskStatusQueued    = "queued"
	OptimizeTaskStatusRunning   = "running"
	OptimizeTaskStatusSucceeded = "succeeded"
	OptimizeTaskStatusFailed    = "failed"
	OptimizeTaskStatusCancelled = "cancelled"
	OptimizeTaskMaxAttempts     = 3
)

// OptimizeTaskRecord is the durable representation of an intelligent prompt
// optimization task. JSON fields preserve the exact request/result snapshots
// so a worker never depends on a mutable prompt draft or evaluation source.
type OptimizeTaskRecord struct {
	ID                 int64
	WorkspaceID        int64
	PromptID           int64
	PromptVersion      string
	Name               string
	SourceType         string
	SourceID           int64
	CaseItemIDsJSON    string
	MappingJSON        string
	BaselinePromptJSON string
	OptimizerModelID   int64
	ModeScore          float64
	Status             string
	Progress           int32
	ResultJSON         string
	ErrorMsg           string
	CancelRequested    bool
	LeaseToken         string
	LeaseExpiresAt     *time.Time
	AttemptCount       int32
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type OptimizeTaskListFilter struct {
	WorkspaceID int64
	PromptID    int64
	Keyword     string
	Statuses    []string
	PageNumber  int32
	PageSize    int32
}
