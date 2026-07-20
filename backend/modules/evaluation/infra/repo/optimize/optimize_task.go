// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package optimize

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-loop/backend/infra/db"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/repo"
)

type optimizeTaskPO struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	WorkspaceID        int64     `gorm:"column:workspace_id"`
	PromptID           int64     `gorm:"column:prompt_id"`
	PromptVersion      string    `gorm:"column:prompt_version"`
	Name               string    `gorm:"column:name"`
	SourceType         string    `gorm:"column:source_type"`
	SourceID           int64     `gorm:"column:source_id"`
	CaseItemIDsJSON    string    `gorm:"column:case_item_ids"`
	MappingJSON        string    `gorm:"column:mapping"`
	BaselinePromptJSON string    `gorm:"column:baseline_prompt"`
	OptimizerModelID   int64     `gorm:"column:optimizer_model_id"`
	ModeScore          float64   `gorm:"column:mode_score"`
	Status             string    `gorm:"column:status"`
	Progress           int32     `gorm:"column:progress"`
	ResultJSON         *string   `gorm:"column:result"`
	ErrorMsg           string    `gorm:"column:error_msg"`
	CancelRequested    bool      `gorm:"column:cancel_requested"`
	CreatedBy          string    `gorm:"column:created_by"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
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

func (r *OptimizeTaskRepo) RequeueStale(ctx context.Context, updatedBefore time.Time) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).
		Where("status = ? AND updated_at < ?", entity.OptimizeTaskStatusRunning, updatedBefore).
		Updates(map[string]any{
			"status":    entity.OptimizeTaskStatusQueued,
			"progress":  0,
			"error_msg": "",
		}).Error
}

func (r *OptimizeTaskRepo) MarkRunning(ctx context.Context, taskID int64) (bool, error) {
	result := r.db.NewSession(ctx).Model(&optimizeTaskPO{}).
		Where("id = ? AND status = ? AND cancel_requested = 0", taskID, entity.OptimizeTaskStatusQueued).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusRunning, "progress": 5})
	return result.RowsAffected == 1, result.Error
}

func (r *OptimizeTaskRepo) UpdateProgress(ctx context.Context, taskID int64, progress int32) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ?", taskID).
		Update("progress", progress).Error
}

func (r *OptimizeTaskRepo) Complete(ctx context.Context, taskID int64, resultJSON string) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ?", taskID).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusSucceeded, "progress": 100, "result": resultJSON, "error_msg": ""}).Error
}

func (r *OptimizeTaskRepo) Fail(ctx context.Context, taskID int64, errMsg string) error {
	return r.db.NewSession(ctx).Model(&optimizeTaskPO{}).Where("id = ?", taskID).
		Updates(map[string]any{"status": entity.OptimizeTaskStatusFailed, "error_msg": errMsg}).Error
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
	return &optimizeTaskPO{
		ID: task.ID, WorkspaceID: task.WorkspaceID, PromptID: task.PromptID,
		PromptVersion: task.PromptVersion, Name: task.Name, SourceType: task.SourceType,
		SourceID: task.SourceID, CaseItemIDsJSON: task.CaseItemIDsJSON,
		MappingJSON: task.MappingJSON, BaselinePromptJSON: task.BaselinePromptJSON,
		OptimizerModelID: task.OptimizerModelID, ModeScore: task.ModeScore, Status: task.Status,
		Progress: task.Progress, ResultJSON: result, ErrorMsg: task.ErrorMsg,
		CancelRequested: task.CancelRequested, CreatedBy: task.CreatedBy,
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
	return &entity.OptimizeTaskRecord{
		ID: po.ID, WorkspaceID: po.WorkspaceID, PromptID: po.PromptID,
		PromptVersion: po.PromptVersion, Name: po.Name, SourceType: po.SourceType,
		SourceID: po.SourceID, CaseItemIDsJSON: po.CaseItemIDsJSON,
		MappingJSON: po.MappingJSON, BaselinePromptJSON: po.BaselinePromptJSON,
		OptimizerModelID: po.OptimizerModelID, ModeScore: po.ModeScore, Status: po.Status,
		Progress: po.Progress, ResultJSON: result, ErrorMsg: po.ErrorMsg,
		CancelRequested: po.CancelRequested, CreatedBy: po.CreatedBy,
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
	}
}
