// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repo

import (
	"context"
	"time"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

type IOptimizeTaskRepo interface {
	Create(ctx context.Context, task *entity.OptimizeTaskRecord) error
	Get(ctx context.Context, workspaceID, taskID int64) (*entity.OptimizeTaskRecord, error)
	GetByID(ctx context.Context, taskID int64) (*entity.OptimizeTaskRecord, error)
	List(ctx context.Context, filter *entity.OptimizeTaskListFilter) ([]*entity.OptimizeTaskRecord, int64, error)
	ListQueued(ctx context.Context, limit int) ([]*entity.OptimizeTaskRecord, error)
	RequeueStale(ctx context.Context, expiredBefore time.Time) error
	MarkRunning(ctx context.Context, taskID int64, leaseToken string, leaseUntil time.Time, maxAttempts int32) (bool, error)
	RenewLease(ctx context.Context, taskID int64, leaseToken string, leaseUntil time.Time) error
	UpdateProgress(ctx context.Context, taskID int64, leaseToken string, progress int32) error
	Complete(ctx context.Context, taskID int64, leaseToken, resultJSON, status string) error
	Fail(ctx context.Context, taskID int64, leaseToken, errMsg string) error
	RequestCancel(ctx context.Context, workspaceID, taskID int64) error
	IsCancelRequested(ctx context.Context, taskID int64) (bool, error)
	MarkCancelled(ctx context.Context, taskID int64) error
}
