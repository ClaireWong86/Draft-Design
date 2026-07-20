// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-loop/backend/infra/idgen"
	"github.com/coze-dev/coze-loop/backend/infra/middleware/session"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/base"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/optimize"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/experimentservice"
	promptdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/prompt/domain/prompt"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/component/rpc"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/repo"
)

const optimizeWorkerQueueSize = 128

const (
	optimizeQueueScanInterval = 30 * time.Second
	optimizeStaleTaskTimeout  = 10 * time.Minute
)

type OptimizeApplication struct {
	idgen       idgen.IIDGenerator
	repo        repo.IOptimizeTaskRepo
	llm         rpc.ILLMProvider
	experiments experimentservice.ExperimentService
	queue       chan int64
}

func NewOptimizeApplication(idGenerator idgen.IIDGenerator, taskRepo repo.IOptimizeTaskRepo, llm rpc.ILLMProvider, experiments experimentservice.ExperimentService) optimize.OptimizeService {
	app := &OptimizeApplication{
		idgen:       idGenerator,
		repo:        taskRepo,
		llm:         llm,
		experiments: experiments,
		queue:       make(chan int64, optimizeWorkerQueueSize),
	}
	go app.runWorker()
	go app.resumeQueuedTasks()
	return app
}

func (a *OptimizeApplication) CreateOptimizeTask(ctx context.Context, req *optimize.CreateOptimizeTaskRequest) (*optimize.CreateOptimizeTaskResponse, error) {
	if err := validateCreateOptimizeTask(req); err != nil {
		return nil, err
	}
	id, err := a.idgen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	mappingJSON, err := json.Marshal(req.Mapping)
	if err != nil {
		return nil, err
	}
	caseIDsJSON, err := json.Marshal(req.CaseItemIds)
	if err != nil {
		return nil, err
	}
	baselineJSON, err := json.Marshal(req.BaselinePrompt)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		name = fmt.Sprintf("智能优化-%s", time.Now().Format("20060102-150405"))
	}
	sourceID := req.Source.GetExperimentID()
	if req.Source.GetType() == optimize.OptimizeSourceTypeEvalSet {
		sourceID = req.Source.GetEvalSetVersionID()
	}
	now := time.Now()
	record := &entity.OptimizeTaskRecord{
		ID: id, WorkspaceID: req.WorkspaceID, PromptID: req.PromptID,
		PromptVersion: req.GetPromptVersion(), Name: name,
		SourceType: string(req.Source.Type), SourceID: sourceID,
		CaseItemIDsJSON: string(caseIDsJSON), MappingJSON: string(mappingJSON),
		BaselinePromptJSON: string(baselineJSON), OptimizerModelID: req.OptimizerModelID,
		ModeScore: req.ModeScore, Status: entity.OptimizeTaskStatusQueued,
		CreatedBy: session.UserIDInCtxOrEmpty(ctx), CreatedAt: now, UpdatedAt: now,
	}
	if err := a.repo.Create(ctx, record); err != nil {
		return nil, err
	}
	a.enqueue(id)
	task, err := a.recordToDTO(record, req.Source)
	if err != nil {
		return nil, err
	}
	return &optimize.CreateOptimizeTaskResponse{Task: task, BaseResp: base.NewBaseResp()}, nil
}

func (a *OptimizeApplication) ListOptimizeTasks(ctx context.Context, req *optimize.ListOptimizeTasksRequest) (*optimize.ListOptimizeTasksResponse, error) {
	if req == nil || req.WorkspaceID <= 0 || req.PromptID <= 0 {
		return nil, errors.New("workspace_id and prompt_id are required")
	}
	pageNumber, pageSize := req.GetPageNumber(), req.GetPageSize()
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	statuses := make([]string, 0, len(req.Statuses))
	for _, status := range req.Statuses {
		statuses = append(statuses, string(status))
	}
	rows, total, err := a.repo.List(ctx, &entity.OptimizeTaskListFilter{
		WorkspaceID: req.WorkspaceID, PromptID: req.PromptID, Keyword: req.GetKeyword(),
		Statuses: statuses, PageNumber: pageNumber, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	tasks := make([]*optimize.OptimizeTask, 0, len(rows))
	for _, row := range rows {
		task, convertErr := a.recordToDTO(row, nil)
		if convertErr != nil {
			return nil, convertErr
		}
		tasks = append(tasks, task)
	}
	return &optimize.ListOptimizeTasksResponse{Tasks: tasks, Total: &total, BaseResp: base.NewBaseResp()}, nil
}

func (a *OptimizeApplication) GetOptimizeTask(ctx context.Context, req *optimize.GetOptimizeTaskRequest) (*optimize.GetOptimizeTaskResponse, error) {
	if req == nil || req.WorkspaceID <= 0 || req.TaskID <= 0 {
		return nil, errors.New("workspace_id and task_id are required")
	}
	record, err := a.repo.Get(ctx, req.WorkspaceID, req.TaskID)
	if err != nil {
		return nil, err
	}
	task, err := a.recordToDTO(record, nil)
	if err != nil {
		return nil, err
	}
	return &optimize.GetOptimizeTaskResponse{Task: task, BaseResp: base.NewBaseResp()}, nil
}

func (a *OptimizeApplication) CancelOptimizeTask(ctx context.Context, req *optimize.CancelOptimizeTaskRequest) (*optimize.CancelOptimizeTaskResponse, error) {
	if req == nil || req.WorkspaceID <= 0 || req.TaskID <= 0 {
		return nil, errors.New("workspace_id and task_id are required")
	}
	if err := a.repo.RequestCancel(ctx, req.WorkspaceID, req.TaskID); err != nil {
		return nil, err
	}
	return &optimize.CancelOptimizeTaskResponse{BaseResp: base.NewBaseResp()}, nil
}

func validateCreateOptimizeTask(req *optimize.CreateOptimizeTaskRequest) error {
	if req == nil || req.WorkspaceID <= 0 || req.PromptID <= 0 {
		return errors.New("workspace_id and prompt_id are required")
	}
	if req.OptimizerModelID <= 0 {
		return errors.New("optimizer_model_id is required")
	}
	if req.ModeScore < 0 || req.ModeScore > 1 {
		return errors.New("mode_score must be between 0 and 1")
	}
	if req.Source == nil || req.Mapping == nil || req.BaselinePrompt == nil {
		return errors.New("source, mapping and baseline_prompt are required")
	}
	if req.BaselinePrompt.PromptID != req.PromptID {
		return errors.New("baseline_prompt.prompt_id must equal prompt_id")
	}
	if len(req.CaseItemIds) == 0 {
		return errors.New("at least one case is required")
	}
	return nil
}

func (a *OptimizeApplication) enqueue(taskID int64) {
	select {
	case a.queue <- taskID:
	default:
		// The task is durable. The resume scan below and the next process restart
		// safely recover queue saturation without losing the request.
	}
}

func (a *OptimizeApplication) resumeQueuedTasks() {
	_ = a.repo.RequeueStale(context.Background(), time.Now().Add(-optimizeStaleTaskTimeout))
	rows, err := a.repo.ListQueued(context.Background(), optimizeWorkerQueueSize)
	if err != nil {
		return
	}
	for _, row := range rows {
		a.enqueue(row.ID)
	}
}

func (a *OptimizeApplication) runWorker() {
	ticker := time.NewTicker(optimizeQueueScanInterval)
	defer ticker.Stop()
	for {
		select {
		case taskID := <-a.queue:
			a.processTask(context.Background(), taskID)
		case <-ticker.C:
			a.resumeQueuedTasks()
		}
	}
}

type optimizerOutput struct {
	OptimizedPrompt  string   `json:"optimized_prompt"`
	FailureModes     []string `json:"failure_modes"`
	SuggestedChanges []string `json:"suggested_instruction_changes"`
}

func (a *OptimizeApplication) processTask(ctx context.Context, taskID int64) {
	claimed, err := a.repo.MarkRunning(ctx, taskID)
	if err != nil || !claimed {
		return
	}
	record, err := a.repo.GetByID(ctx, taskID)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, err.Error())
		return
	}
	if cancelled, _ := a.repo.IsCancelRequested(ctx, taskID); cancelled {
		_ = a.repo.MarkCancelled(ctx, taskID)
		return
	}
	_ = a.repo.UpdateProgress(ctx, taskID, 20)
	out, err := a.generateCandidate(ctx, record)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, err.Error())
		return
	}
	if cancelled, _ := a.repo.IsCancelRequested(ctx, taskID); cancelled {
		_ = a.repo.MarkCancelled(ctx, taskID)
		return
	}
	_ = a.repo.UpdateProgress(ctx, taskID, 80)
	resultJSON, err := buildOptimizeResult(record, out)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, err.Error())
		return
	}
	_ = a.repo.Complete(ctx, taskID, resultJSON)
}

func (a *OptimizeApplication) generateCandidate(ctx context.Context, task *entity.OptimizeTaskRecord) (*optimizerOutput, error) {
	evidence, err := a.loadEvidence(ctx, task)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"baseline_prompt":   json.RawMessage(task.BaselinePromptJSON),
		"field_mapping":     json.RawMessage(task.MappingJSON),
		"selected_case_ids": json.RawMessage(task.CaseItemIDsJSON),
		"source_type":       task.SourceType,
		"source_id":         task.SourceID,
		"mode_score":        task.ModeScore,
		"evidence":          evidence,
	}
	userJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	systemText := "你是多模态 Prompt 优化器。分析基线 Prompt 与评测来源，保持变量名、输出协议和多模态占位符不变，给出更清晰、可执行、感知与推理解耦的指令。只返回 JSON：{\"optimized_prompt\":\"...\",\"failure_modes\":[\"...\"],\"suggested_instruction_changes\":[\"...\"]}。"
	userText := string(userJSON)
	modelID := task.OptimizerModelID
	maxTokens := int32(4096)
	temperature := 0.2
	reply, err := a.llm.Call(ctx, &entity.LLMCallParam{
		SpaceID: task.WorkspaceID, EvaluatorID: fmt.Sprintf("optimize_task:%d", task.ID),
		Scenario: entity.ScenarioEvaluator,
		Messages: []*entity.Message{
			{Role: entity.RoleSystem, Content: &entity.Content{Text: &systemText}},
			{Role: entity.RoleUser, Content: &entity.Content{Text: &userText}},
		},
		ModelConfig: &entity.ModelConfig{ModelID: &modelID, MaxTokens: &maxTokens, Temperature: &temperature},
	})
	if err != nil {
		return nil, err
	}
	if reply == nil || reply.Content == nil || strings.TrimSpace(*reply.Content) == "" {
		return nil, errors.New("optimizer model returned empty content")
	}
	var out optimizerOutput
	if err := json.Unmarshal([]byte(stripJSONFence(*reply.Content)), &out); err != nil {
		return nil, fmt.Errorf("parse optimizer output: %w", err)
	}
	if strings.TrimSpace(out.OptimizedPrompt) == "" {
		return nil, errors.New("optimizer output missing optimized_prompt")
	}
	return &out, nil
}

// loadEvidence snapshots the selected experiment results before the optimizer
// is called. This keeps the worker deterministic even if the experiment report
// changes while a task is running, and gives the meta-model actual/reference
// outputs and evaluator scores instead of only case IDs.
func (a *OptimizeApplication) loadEvidence(ctx context.Context, task *entity.OptimizeTaskRecord) (any, error) {
	if task.SourceType != string(optimize.OptimizeSourceTypeExperiment) || a.experiments == nil {
		return map[string]any{"source_type": task.SourceType, "case_item_ids": json.RawMessage(task.CaseItemIDsJSON)}, nil
	}
	pageSize := int32(500)
	pageNumber := int32(1)
	useAccelerator := false
	fullTrajectory := false
	resp, err := a.experiments.BatchGetExperimentResult_(ctx, &experimentservice.BatchGetExperimentResultRequest{
		WorkspaceID:     task.WorkspaceID,
		ExperimentIds:   []int64{task.SourceID},
		PageNumber:      &pageNumber,
		PageSize:        &pageSize,
		UseAccelerator:  &useAccelerator,
		FullTrajectory:  &fullTrajectory,
	})
	if err != nil {
		return nil, fmt.Errorf("load experiment evidence: %w", err)
	}
	selected := make(map[int64]struct{})
	var caseIDs []string
	if err := json.Unmarshal([]byte(task.CaseItemIDsJSON), &caseIDs); err == nil {
		for _, rawID := range caseIDs {
			if id, parseErr := strconv.ParseInt(rawID, 10, 64); parseErr == nil {
				selected[id] = struct{}{}
			}
		}
	}
	if len(selected) == 0 || resp == nil {
		return resp, nil
	}
	filtered := resp.ItemResults[:0]
	for _, item := range resp.ItemResults {
		if item != nil {
			if _, ok := selected[item.GetItemID()]; ok {
				filtered = append(filtered, item)
			}
		}
	}
	resp.ItemResults = filtered
	return resp, nil
}

func buildOptimizeResult(record *entity.OptimizeTaskRecord, out *optimizerOutput) (string, error) {
	var before optimize.OptimizePromptSnapshot
	if err := json.Unmarshal([]byte(record.BaselinePromptJSON), &before); err != nil {
		return "", err
	}
	var after optimize.OptimizePromptSnapshot
	if err := json.Unmarshal([]byte(record.BaselinePromptJSON), &after); err != nil {
		return "", err
	}
	setOptimizedInstruction(&after, out.OptimizedPrompt)
	result := &optimize.OptimizeTaskResult_{
		BeforePrompt: &before,
		AfterPrompt:  &after,
		Diagnosis: &optimize.OptimizeDiagnosis{
			FailureModes:                out.FailureModes,
			SuggestedInstructionChanges: out.SuggestedChanges,
		},
	}
	b, err := json.Marshal(result)
	return string(b), err
}

func setOptimizedInstruction(snapshot *optimize.OptimizePromptSnapshot, text string) {
	for _, message := range snapshot.Messages {
		if message != nil && message.GetRole() == promptdto.RoleSystem {
			message.Content = &text
			return
		}
	}
	role := promptdto.RoleSystem
	snapshot.Messages = append([]*promptdto.Message{{Role: &role, Content: &text}}, snapshot.Messages...)
}

func stripJSONFence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		value = strings.TrimPrefix(value, "```json")
		value = strings.TrimPrefix(value, "```")
		value = strings.TrimSuffix(value, "```")
	}
	return strings.TrimSpace(value)
}

func (a *OptimizeApplication) recordToDTO(record *entity.OptimizeTaskRecord, source *optimize.OptimizeSource) (*optimize.OptimizeTask, error) {
	if source == nil {
		source = &optimize.OptimizeSource{Type: optimize.OptimizeSourceType(record.SourceType)}
		if source.Type == optimize.OptimizeSourceTypeExperiment {
			source.ExperimentID = &record.SourceID
		} else {
			source.EvalSetVersionID = &record.SourceID
		}
	}
	var caseIDs []string
	if err := json.Unmarshal([]byte(record.CaseItemIDsJSON), &caseIDs); err != nil {
		return nil, err
	}
	var mapping optimize.OptimizeFieldMapping
	if err := json.Unmarshal([]byte(record.MappingJSON), &mapping); err != nil {
		return nil, err
	}
	var result *optimize.OptimizeTaskResult_
	if record.ResultJSON != "" {
		result = &optimize.OptimizeTaskResult_{}
		if err := json.Unmarshal([]byte(record.ResultJSON), result); err != nil {
			return nil, err
		}
	}
	createdAt, updatedAt := record.CreatedAt.UnixMilli(), record.UpdatedAt.UnixMilli()
	promptVersion, createdBy := record.PromptVersion, record.CreatedBy
	var errorMsg *string
	if record.ErrorMsg != "" {
		errorMsg = &record.ErrorMsg
	}
	return &optimize.OptimizeTask{
		ID: record.ID, Name: record.Name, WorkspaceID: record.WorkspaceID, PromptID: record.PromptID,
		PromptVersion: &promptVersion, Source: source, CaseItemIds: caseIDs, Mapping: &mapping,
		ModeScore: record.ModeScore, OptimizerModelID: record.OptimizerModelID,
		Status: optimize.OptimizeTaskStatus(record.Status), Progress: record.Progress,
		ErrorMsg: errorMsg, Result_: result, CreatedAt: &createdAt, UpdatedAt: &updatedAt, CreatedBy: &createdBy,
	}, nil
}
