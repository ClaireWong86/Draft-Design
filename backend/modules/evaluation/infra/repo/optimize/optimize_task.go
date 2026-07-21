// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package optimize

import (
	"context"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-loop/backend/infra/db"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/repo"
)

type optimizeTaskPO struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	WorkspaceID        int64      `gorm:"column:workspace_id"`
	PromptID           int64      `gorm:"column:prompt_id"`
	PromptVersion      string     `gorm:"column:prompt_version"`
	Name               string     `gorm:"column:name"`
	SourceType         string     `gorm:"column:source_type"`
	SourceID           int64      `gorm:"column:source_id"`
	SourceJSON         *string    `gorm:"column:source"`
	CaseItemIDsJSON    string     `gorm:"column:case_item_ids"`
	MappingJSON        string     `gorm:"column:mapping"`
	BaselinePromptJSON string     `gorm:"column:baseline_prompt"`
	OptimizerModelID   int64      `gorm:"column:optimizer_model_id"`
	ModeScore          float64    `gorm:"column:mode_score"`
	Status             string     `gorm:"column:status"`
	Progress           int32      `gorm:"column:progress"`
	ResultJSON         *string    `gorm:"column:result"`
	ErrorMsg           string     `gorm:"column:error_msg"`
	CancelRequested    bool       `gorm:"column:cancel_requested"`
	LeaseToken         string     `gorm:"column:lease_token"`
	LeaseExpiresAt     *time.Time `gorm:"column:lease_expires_at"`
	AttemptCount       int32      `gorm:"column:attempt_count"`
	CreatedBy          string     `gorm:"column:created_by"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (optimizeTaskPO) TableName() string { return "optimize_task" }

type OptimizeTaskRepo struct{ db db.Provider }

func NewOptimizeTaskRepo(provider db.Provider) repo.IOptimizeTaskRepo {
	return &OptimizeTaskRepo{db: provider}
}

func (r *OptimizeTaskRepo) Create(ctx context.Context, task *entity.OptimizeTaskRecord) error {
	return r.db.NewSession(ctx).Create(toPO(task)).Error
}

func (r *OptimizeTaskRepo) Get(ctx context.Context, workspaceID, taskID int64) (*entity.OptimizeTaskRecord, error) {
	var po optimizeTaskPO
	err := r.db.NewSession(ctx).Where("workspace_id = ? AND id = ?", workspaceID, taskID).First(&po).Error
	if err != nil {
		return nil, err
	}
	return toDO(&po), nil
}

func (r *OptimizeTaskRepo) GetByID(ctx context.Context, taskID int64) (*entity.OptimizeTaskRecord, error) {
	var po optimizeTaskPO
	if err := r.db.NewSession(ctx).Where("id = ?", taskID).First(&po).Error; err != nil {
		return nil, err
	}
	return toDO(&po), nil
}

func (r *OptimizeTaskRepo) List(ctx context.Context, f *entity.OptimizeTaskListFilter) ([]*entity.OptimizeTaskRecord, int64, error) {
	q := r.db.NewSession(ctx).Model(&optimizeTaskPO{}).
		Where("workspace_id = ? AND prompt_id = ?", f.WorkspaceID, f.PromptID)
	if f.Keyword != "" {
		q = q.Where("name LIKE ?", "%"+f.Keyword+"%")
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*optimizeTaskPO
	offset := int((f.PageNumber - 1) * f.PageSize)
	if err := q.Order("created_at DESC").Offset(offset).Limit(int(f.PageSize)).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]*entity.OptimizeTaskRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, toDO(row))
	}
	return result, total, nil
}

func (r *OptimizeTaskRepo) ListQueued(ctx context.Context, limit int) ([]*entity.OptimizeTaskRecord, error) {
	var rows []*optimizeTaskPO
	err := r.db.NewSession(ctx).Where("status = ?", entity.OptimizeTaskStatusQueued).
		Order("created_at ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]*entity.OptimizeTaskRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, toDO(row))
	}
	return result, nil
}

func (r *OptimizeTaskRepo) RequeueStale(ctx context.Context, expiredBefore time.Time) error {
	return r.db.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&optimizeTaskPO{}).
			Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ? AND attempt_count >= ?", entity.OptimizeTaskStatusRunning, expiredBefore, entity.OptimizeTaskMaxAttempts).
			Updates(map[string]any{"status": entity.OptimizeTaskStatusFailed, "error_msg": "worker retry limit exceeded", "lease_token": "", "lease_expires_at": nil}).Error; err != nil {
			return err
		}
		return tx.Model(&optimizeTaskPO{}).
			Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", entity.OptimizeTaskStatusRunning, expiredBefore).
			Updates(map[string]any{
				"status": entity.OptimizeTaskStatusQueued, "progress": 0, "error_msg": "",
				"lease_token": "", "lease_expires_at": nil,
			}).Error
	})
}

func (r *OptimizeTaskRepo) MarkRunning(ctx context.Context, taskID int64, leaseToken string, leaseUntil time.Time, maxAttempts int32) (bool, error) {
	result := r.db.NewSession(ctx).Model(&optimizeTaskPO{}).
		Where("id = ? AND status = ? AND cancel_requested = 0 AND attempt_count < ?", taskID, entity.OptimizeTaskStatusQueued, maxAttempts).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusRunning, "progress": 5, "lease_token": leaseToken,
			"lease_expires_at": leaseUntil, "attempt_count": gorm.Expr("attempt_count + 1")})
	return result.RowsAffected == 1, result.Error
}

func (r *OptimizeTaskRepo) RenewLease(ctx context.Context, taskID int64, leaseToken string, leaseUntil time.Time) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).
		Where("id = ? AND status = ? AND lease_token = ?", taskID, entity.OptimizeTaskStatusRunning, leaseToken).
		Update("lease_expires_at", leaseUntil).Error
}

func (r *OptimizeTaskRepo) UpdateProgress(ctx context.Context, taskID int64, leaseToken string, progress int32) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ? AND status = ? AND lease_token = ?", taskID, entity.OptimizeTaskStatusRunning, leaseToken).
		Update("progress", progress).Error
}

func (r *OptimizeTaskRepo) Complete(ctx context.Context, taskID int64, leaseToken, resultJSON string) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ? AND status = ? AND lease_token = ?", taskID, entity.OptimizeTaskStatusRunning, leaseToken).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusSucceeded, "progress": 100, "result": resultJSON, "error_msg": "", "lease_token": "", "lease_expires_at": nil}).Error
}

func (r *OptimizeTaskRepo) Fail(ctx context.Context, taskID int64, leaseToken, errMsg string) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ? AND status = ? AND lease_token = ?", taskID, entity.OptimizeTaskStatusRunning, leaseToken).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusFailed, "error_msg": truncateOptimizeErrorMessage(errMsg), "lease_token": "", "lease_expires_at": nil}).Error
}

const optimizeErrorMessageMaxRunes = 2048

// error_msg is VARCHAR(2048). Some wrapped service errors include a complete
// stack trace; truncating them here guarantees that recording the failure does
// not itself fail and leave a task permanently in the running state.
func truncateOptimizeErrorMessage(message string) string {
	if utf8.RuneCountInString(message) <= optimizeErrorMessageMaxRunes {
		return message
	}
	runes := []rune(message)
	return string(runes[:optimizeErrorMessageMaxRunes])
}

func (r *OptimizeTaskRepo) RequestCancel(ctx context.Context, workspaceID, taskID int64) error {
	var affected int64
	err := r.db.Transaction(ctx, func(tx *gorm.DB) error {
		queued := tx.Model(&optimizeTaskPO{}).
			Where("workspace_id = ? AND id = ? AND status = ?", workspaceID, taskID, entity.OptimizeTaskStatusQueued).
			Updates(map[string]any{
				"cancel_requested": true,
				"status":           entity.OptimizeTaskStatusCancelled,
				"progress":         100,
			})
		if queued.Error != nil {
			return queued.Error
		}
		running := tx.Model(&optimizeTaskPO{}).
			Where("workspace_id = ? AND id = ? AND status = ?", workspaceID, taskID, entity.OptimizeTaskStatusRunning).
			Update("cancel_requested", true)
		if running.Error != nil {
			return running.Error
		}
		affected = queued.RowsAffected + running.RowsAffected
		return nil
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *OptimizeTaskRepo) IsCancelRequested(ctx context.Context, taskID int64) (bool, error) {
	var po optimizeTaskPO
	err := r.db.NewSession(ctx).Select("cancel_requested").Where("id = ?", taskID).First(&po).Error
	return po.CancelRequested, err
}

func (r *OptimizeTaskRepo) MarkCancelled(ctx context.Context, taskID int64) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ?", taskID).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusCancelled, "progress": 100}).Error
}

func toPO(task *entity.OptimizeTaskRecord) *optimizeTaskPO {
	var result *string
	if task.ResultJSON != "" {
		result = &task.ResultJSON
	}
	var source *string
	if task.SourceJSON != "" {
		source = &task.SourceJSON
	}
	return &optimizeTaskPO{
		ID: task.ID, WorkspaceID: task.WorkspaceID, PromptID: task.PromptID,
		PromptVersion: task.PromptVersion, Name: task.Name, SourceType: task.SourceType,
		SourceID: task.SourceID, SourceJSON: source, CaseItemIDsJSON: task.CaseItemIDsJSON,
		MappingJSON: task.MappingJSON, BaselinePromptJSON: task.BaselinePromptJSON,
		OptimizerModelID: task.OptimizerModelID, ModeScore: task.ModeScore, Status: task.Status,
		Progress: task.Progress, ResultJSON: result, ErrorMsg: task.ErrorMsg,
		CancelRequested: task.CancelRequested, CreatedBy: task.CreatedBy,
		LeaseToken: task.LeaseToken, LeaseExpiresAt: task.LeaseExpiresAt, AttemptCount: task.AttemptCount,
	}
}

func toDO(po *optimizeTaskPO) *entity.OptimizeTaskRecord {
	if po == nil {
		return nil
	}
	result := ""
	if po.ResultJSON != nil {
		result = *po.ResultJSON
	}
	source := ""
	if po.SourceJSON != nil {
		source = *po.SourceJSON
	}
	return &entity.OptimizeTaskRecord{
		ID: po.ID, WorkspaceID: po.WorkspaceID, PromptID: po.PromptID,
		PromptVersion: po.PromptVersion, Name: po.Name, SourceType: po.SourceType,
		SourceID: po.SourceID, SourceJSON: source, CaseItemIDsJSON: po.CaseItemIDsJSON,
		MappingJSON: po.MappingJSON, BaselinePromptJSON: po.BaselinePromptJSON,
		OptimizerModelID: po.OptimizerModelID, ModeScore: po.ModeScore, Status: po.Status,
		Progress: po.Progress, ResultJSON: result, ErrorMsg: po.ErrorMsg,
		CancelRequested: po.CancelRequested, CreatedBy: po.CreatedBy,
		LeaseToken: po.LeaseToken, LeaseExpiresAt: po.LeaseExpiresAt, AttemptCount: po.AttemptCount,
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
	}
}
