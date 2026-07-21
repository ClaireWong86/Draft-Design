// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-loop/backend/infra/idgen"
	"github.com/coze-dev/coze-loop/backend/infra/middleware/session"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/base"
	evaluationapi "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation"
	evaluatorcommon "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/common"
	evaluatordto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/evaluator"
	domainexpt "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/expt"
	evaluatorservice "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/evaluator"
	expt "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/expt"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/optimize"
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
	experiments IExperimentApplication
	evaluators  evaluationapi.EvaluatorService
	queue       chan int64
}

func NewOptimizeApplication(idGenerator idgen.IIDGenerator, taskRepo repo.IOptimizeTaskRepo, llm rpc.ILLMProvider, experiments IExperimentApplication, evaluators evaluationapi.EvaluatorService) optimize.OptimizeService {
	app := &OptimizeApplication{
		idgen:       idGenerator,
		repo:        taskRepo,
		llm:         llm,
		experiments: experiments,
		evaluators:  evaluators,
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
	leaseToken := newOptimizeLeaseToken()
	claimed, err := a.repo.MarkRunning(ctx, taskID, leaseToken, time.Now().Add(optimizeStaleTaskTimeout), entity.OptimizeTaskMaxAttempts)
	if err != nil || !claimed {
		return
	}
	record, err := a.repo.GetByID(ctx, taskID)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, leaseToken, err.Error())
		return
	}
	if cancelled, _ := a.repo.IsCancelRequested(ctx, taskID); cancelled {
		_ = a.repo.MarkCancelled(ctx, taskID)
		return
	}
	_ = a.repo.UpdateProgress(ctx, taskID, leaseToken, 20)
	out, caseResults, err := a.runOptimizationLoop(ctx, record, leaseToken)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, leaseToken, err.Error())
		return
	}
	_ = a.repo.UpdateProgress(ctx, taskID, leaseToken, 80)
	resultJSON, err := buildOptimizeResult(record, out, caseResults)
	if err != nil {
		_ = a.repo.Fail(ctx, taskID, leaseToken, err.Error())
		return
	}
	_ = a.repo.Complete(ctx, taskID, leaseToken, resultJSON)
}

func newOptimizeLeaseToken() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("lease-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", value[:])
}

type optimizePolicy struct {
	rounds          int
	candidates      int
	validationRatio float64
	minGain         float64
	maxModelCalls   int
}

func policyForMode(modeScore float64) optimizePolicy {
	policy := optimizePolicy{rounds: 2, candidates: 2, validationRatio: 0.25, minGain: 0.001, maxModelCalls: 48}
	if modeScore >= 0.67 {
		policy.rounds, policy.candidates, policy.maxModelCalls = 3, 3, 96
	} else if modeScore <= 0.33 {
		policy.rounds, policy.candidates, policy.maxModelCalls = 1, 2, 24
	}
	return policy
}

func (a *OptimizeApplication) runOptimizationLoop(ctx context.Context, task *entity.OptimizeTaskRecord, leaseToken string) (*optimizerOutput, []candidateCaseResult, error) {
	policy := policyForMode(task.ModeScore)
	ctx = context.WithValue(ctx, optimizeBudgetContextKey{}, &optimizeCallBudget{max: policy.maxModelCalls})
	optimizationIDs, validationIDs := splitCaseIDs(task.CaseItemIDsJSON, policy.validationRatio)
	current := *task
	if len(optimizationIDs) > 0 {
		encoded, _ := json.Marshal(optimizationIDs)
		current.CaseItemIDsJSON = string(encoded)
	}
	var bestOut *optimizerOutput
	var bestCases []candidateCaseResult
	bestScore := -1e100
	feedback := ""
	for round := 0; round < policy.rounds; round++ {
		if err := a.repo.RenewLease(ctx, task.ID, leaseToken, time.Now().Add(optimizeStaleTaskTimeout)); err != nil {
			return nil, nil, fmt.Errorf("renew optimize lease: %w", err)
		}
		previousBestScore := bestScore
		roundBestScore := -1e100
		var roundBestOut *optimizerOutput
		for candidate := 0; candidate < policy.candidates; candidate++ {
			out, err := a.generateCandidate(ctx, &current, feedback)
			if err != nil {
				return nil, nil, err
			}
			cases, err := a.executeCandidateCases(ctx, task, out)
			if err != nil {
				return nil, nil, err
			}
			score, scored := validationScore(cases, validationIDs)
			if !scored {
				score = -float64(candidate)
			}
			if bestOut == nil || score > bestScore {
				bestOut, bestCases, bestScore = out, cases, score
			}
			if roundBestOut == nil || score > roundBestScore {
				roundBestOut, roundBestScore = out, score
			}
		}
		if roundBestOut == nil || (round > 0 && roundBestScore-previousBestScore <= policy.minGain) {
			break
		}
		feedback = strings.Join(roundBestOut.SuggestedChanges, "; ")
		var snapshot optimize.OptimizePromptSnapshot
		if err := json.Unmarshal([]byte(current.BaselinePromptJSON), &snapshot); err != nil {
			return nil, nil, err
		}
		setOptimizedInstruction(&snapshot, roundBestOut.OptimizedPrompt)
		encoded, _ := json.Marshal(&snapshot)
		current.BaselinePromptJSON = string(encoded)
		_ = a.repo.UpdateProgress(ctx, task.ID, leaseToken, int32(25+(round+1)*50/policy.rounds))
	}
	if bestOut == nil {
		return nil, nil, errors.New("optimizer produced no candidate")
	}
	return bestOut, bestCases, nil
}

func splitCaseIDs(encoded string, validationRatio float64) ([]string, map[string]struct{}) {
	var ids []string
	_ = json.Unmarshal([]byte(encoded), &ids)
	validation := make(map[string]struct{})
	if len(ids) < 2 {
		return ids, validation
	}
	validationCount := int(float64(len(ids)) * validationRatio)
	if validationCount < 1 {
		validationCount = 1
	}
	optimization := append([]string(nil), ids[:len(ids)-validationCount]...)
	for _, id := range ids[len(ids)-validationCount:] {
		validation[id] = struct{}{}
	}
	return optimization, validation
}

func validationScore(cases []candidateCaseResult, validation map[string]struct{}) (float64, bool) {
	var values []float64
	for _, item := range cases {
		if len(validation) > 0 {
			if _, ok := validation[item.caseID]; !ok {
				continue
			}
		}
		if score, ok := averageScores(item.afterScores); ok {
			values = append(values, score)
		}
	}
	return averageSlice(values)
}

// executeCandidateCases performs one temporary execution per selected case.
// It deliberately does not create a Prompt version. Each case is sent with a
// structured variables/evidence payload; outputs are not treated as scores.
type candidateCaseResult struct {
	caseID         string
	output         string
	input          map[string]any
	beforeScores   map[int64]float64
	afterScores    map[int64]float64
	evaluatorNames map[int64]string
}

func (a *OptimizeApplication) executeCandidateCases(ctx context.Context, task *entity.OptimizeTaskRecord, out *optimizerOutput) ([]candidateCaseResult, error) {
	var snapshot optimize.OptimizePromptSnapshot
	if err := json.Unmarshal([]byte(task.BaselinePromptJSON), &snapshot); err != nil {
		return nil, fmt.Errorf("decode candidate snapshot: %w", err)
	}
	setOptimizedInstruction(&snapshot, out.OptimizedPrompt)
	evidence, err := a.loadEvidence(ctx, task)
	if err != nil {
		return nil, err
	}
	itemEvidence := []json.RawMessage{nil}
	var experimentItems []*domainexpt.ItemResult_
	if response, ok := evidence.(*expt.BatchGetExperimentResultResponse); ok && response != nil {
		if len(response.ItemResults) == 0 {
			return nil, errors.New("no experiment cases matched the selected IDs and score filters")
		}
		experimentItems = response.ItemResults
		itemEvidence = make([]json.RawMessage, 0, len(response.ItemResults))
		for _, item := range response.ItemResults {
			encoded, marshalErr := json.Marshal(item)
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal case evidence: %w", marshalErr)
			}
			itemEvidence = append(itemEvidence, encoded)
		}
	}
	results := make([]candidateCaseResult, 0, len(itemEvidence))
	for index, evidenceJSON := range itemEvidence {
		if cancelled, _ := a.repo.IsCancelRequested(ctx, task.ID); cancelled {
			return nil, errors.New("candidate execution cancelled")
		}
		mapped, caseID := mapCaseEvidence(task, evidenceJSON, index)
		rendered, renderErr := renderCandidateMessages(snapshot.Messages, mapped)
		if renderErr != nil {
			return nil, fmt.Errorf("render candidate case %s: %w", caseID, renderErr)
		}
		caseMessages, convertErr := convertCandidateMessages(rendered)
		if convertErr != nil {
			return nil, fmt.Errorf("convert candidate case %s: %w", caseID, convertErr)
		}
		if len(caseMessages) == 0 {
			return nil, errors.New("candidate prompt has no messages")
		}
		output, err := a.callCandidate(ctx, task, caseMessages)
		if err != nil {
			return nil, fmt.Errorf("execute candidate case %d: %w", index, err)
		}
		result := candidateCaseResult{caseID: caseID, output: output, input: mapped}
		if index < len(experimentItems) {
			result.beforeScores, result.afterScores, result.evaluatorNames, err = a.rerunOriginalEvaluators(ctx, task, experimentItems[index], output)
			if err != nil {
				return nil, fmt.Errorf("rerun evaluators for case %s: %w", caseID, err)
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func (a *OptimizeApplication) rerunOriginalEvaluators(ctx context.Context, task *entity.OptimizeTaskRecord, item *domainexpt.ItemResult_, actual string) (map[int64]float64, map[int64]float64, map[int64]string, error) {
	before, after, names := map[int64]float64{}, map[int64]float64{}, map[int64]string{}
	if a.evaluators == nil || item == nil {
		return before, after, names, nil
	}
	for _, turn := range item.GetTurnResults() {
		for _, experimentResult := range turn.GetExperimentResults() {
			payload := experimentResult.GetPayload()
			if payload == nil || payload.GetEvaluatorOutput() == nil {
				continue
			}
			for evaluatorVersionID, baseline := range payload.GetEvaluatorOutput().GetEvaluatorRecords() {
				if baseline == nil || baseline.GetEvaluatorInputData() == nil {
					continue
				}
				if output := baseline.GetEvaluatorOutputData(); output != nil && output.GetEvaluatorResult_() != nil && output.GetEvaluatorResult_().Score != nil {
					before[evaluatorVersionID] = output.GetEvaluatorResult_().GetScore()
				}
				input, cloneErr := cloneEvaluatorInput(baseline.GetEvaluatorInputData())
				if cloneErr != nil {
					return nil, nil, nil, cloneErr
				}
				replaceEvaluatorActual(input, actual)
				if err := consumeOptimizeCall(ctx); err != nil {
					return nil, nil, nil, err
				}
				resp, runErr := a.evaluators.RunEvaluator(ctx, &evaluatorservice.RunEvaluatorRequest{
					WorkspaceID: task.WorkspaceID, EvaluatorVersionID: evaluatorVersionID, InputData: input,
				})
				if runErr != nil {
					return nil, nil, nil, runErr
				}
				if resp != nil && resp.Record != nil && resp.Record.GetEvaluatorOutputData() != nil && resp.Record.GetEvaluatorOutputData().GetEvaluatorResult_() != nil && resp.Record.GetEvaluatorOutputData().GetEvaluatorResult_().Score != nil {
					after[evaluatorVersionID] = resp.Record.GetEvaluatorOutputData().GetEvaluatorResult_().GetScore()
				}
			}
		}
	}
	return before, after, names, nil
}

func cloneEvaluatorInput(input *evaluatordto.EvaluatorInputData) (*evaluatordto.EvaluatorInputData, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cloned := &evaluatordto.EvaluatorInputData{}
	if err := json.Unmarshal(raw, cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func replaceEvaluatorActual(input *evaluatordto.EvaluatorInputData, actual string) {
	if input.EvaluateTargetOutputFields == nil {
		input.EvaluateTargetOutputFields = make(map[string]*evaluatorcommon.Content)
	}
	textType := evaluatorcommon.ContentTypeText
	content := &evaluatorcommon.Content{ContentType: &textType, Text: &actual}
	if len(input.EvaluateTargetOutputFields) == 0 {
		input.EvaluateTargetOutputFields["actual_output"] = content
		return
	}
	for key := range input.EvaluateTargetOutputFields {
		input.EvaluateTargetOutputFields[key] = content
	}
}

func (a *OptimizeApplication) callCandidate(ctx context.Context, task *entity.OptimizeTaskRecord, messages []*entity.Message) (string, error) {
	modelID := task.OptimizerModelID
	maxTokens := int32(4096)
	temperature := 0.2
	reply, err := a.callLLMWithRetry(ctx, &entity.LLMCallParam{
		SpaceID: task.WorkspaceID, EvaluatorID: fmt.Sprintf("optimize_candidate:%d", task.ID),
		Scenario: entity.ScenarioEvaluator, Messages: messages,
		ModelConfig: &entity.ModelConfig{ModelID: &modelID, MaxTokens: &maxTokens, Temperature: &temperature},
	})
	if err != nil {
		return "", fmt.Errorf("execute candidate: %w", err)
	}
	if reply == nil || reply.Content == nil || strings.TrimSpace(*reply.Content) == "" {
		return "", errors.New("candidate execution returned empty output")
	}
	return *reply.Content, nil
}

func (a *OptimizeApplication) generateCandidate(ctx context.Context, task *entity.OptimizeTaskRecord, feedback string) (*optimizerOutput, error) {
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
		"previous_feedback": feedback,
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
	reply, err := a.callLLMWithRetry(ctx, &entity.LLMCallParam{
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

func (a *OptimizeApplication) callLLMWithRetry(ctx context.Context, param *entity.LLMCallParam) (*entity.ReplyItem, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := consumeOptimizeCall(ctx); err != nil {
			return nil, err
		}
		reply, err := a.llm.Call(ctx, param)
		if err == nil {
			return reply, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return nil, lastErr
}

type optimizeBudgetContextKey struct{}

type optimizeCallBudget struct {
	used int
	max  int
}

func consumeOptimizeCall(ctx context.Context) error {
	budget, _ := ctx.Value(optimizeBudgetContextKey{}).(*optimizeCallBudget)
	if budget == nil {
		return nil
	}
	if budget.used >= budget.max {
		return fmt.Errorf("optimize model-call budget exhausted (%d)", budget.max)
	}
	budget.used++
	return nil
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
	resp, err := a.experiments.BatchGetExperimentResult_(ctx, &expt.BatchGetExperimentResultRequest{
		WorkspaceID:    task.WorkspaceID,
		ExperimentIds:  []int64{task.SourceID},
		PageNumber:     &pageNumber,
		PageSize:       &pageSize,
		UseAccelerator: &useAccelerator,
		FullTrajectory: &fullTrajectory,
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
	var mapping optimize.OptimizeFieldMapping
	_ = json.Unmarshal([]byte(task.MappingJSON), &mapping)
	for _, item := range resp.ItemResults {
		if item != nil {
			if _, ok := selected[item.GetItemID()]; ok {
				if matchesOptimizeScoreFilter(item, &mapping) {
					filtered = append(filtered, item)
				}
			}
		}
	}
	resp.ItemResults = filtered
	return resp, nil
}

func matchesOptimizeScoreFilter(item *domainexpt.ItemResult_, mapping *optimize.OptimizeFieldMapping) bool {
	if mapping == nil || (mapping.EvaluatorVersionID == nil && mapping.ScoreMin == nil && mapping.ScoreMax == nil && !mapping.GetOnlyFailed()) {
		return true
	}
	found := false
	for _, turn := range item.GetTurnResults() {
		for _, experimentResult := range turn.GetExperimentResults() {
			payload := experimentResult.GetPayload()
			if payload == nil || payload.GetEvaluatorOutput() == nil {
				continue
			}
			for evaluatorVersionID, record := range payload.GetEvaluatorOutput().GetEvaluatorRecords() {
				if mapping.EvaluatorVersionID != nil && evaluatorVersionID != mapping.GetEvaluatorVersionID() {
					continue
				}
				if record == nil || record.GetEvaluatorOutputData() == nil || record.GetEvaluatorOutputData().GetEvaluatorResult_() == nil || record.GetEvaluatorOutputData().GetEvaluatorResult_().Score == nil {
					continue
				}
				score := record.GetEvaluatorOutputData().GetEvaluatorResult_().GetScore()
				if mapping.ScoreMin != nil && score < mapping.GetScoreMin() {
					continue
				}
				if mapping.ScoreMax != nil && score > mapping.GetScoreMax() {
					continue
				}
				if mapping.GetOnlyFailed() && score >= 1 {
					continue
				}
				found = true
			}
		}
	}
	return found
}

func buildOptimizeResult(record *entity.OptimizeTaskRecord, out *optimizerOutput, caseResults []candidateCaseResult) (string, error) {
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
	var beforeDistribution, afterDistribution []float64
	for _, item := range caseResults {
		caseID := item.caseID
		actual := item.output
		detail := &optimize.OptimizeCaseDetail{CaseID: caseID, AfterActual: &actual}
		if score, ok := averageScores(item.beforeScores); ok {
			detail.BeforeScore = &score
			beforeDistribution = append(beforeDistribution, score)
		}
		if score, ok := averageScores(item.afterScores); ok {
			detail.AfterScore = &score
			afterDistribution = append(afterDistribution, score)
		}
		evaluatorIDs := make(map[int64]struct{}, len(item.beforeScores)+len(item.afterScores))
		for evaluatorID := range item.beforeScores {
			evaluatorIDs[evaluatorID] = struct{}{}
		}
		for evaluatorID := range item.afterScores {
			evaluatorIDs[evaluatorID] = struct{}{}
		}
		for evaluatorID := range evaluatorIDs {
			score := &optimize.OptimizeEvaluatorScore{EvaluatorVersionID: evaluatorID}
			if value, ok := item.beforeScores[evaluatorID]; ok {
				score.BeforeScore = &value
			}
			if value, ok := item.afterScores[evaluatorID]; ok {
				score.AfterScore = &value
			}
			if name := item.evaluatorNames[evaluatorID]; name != "" {
				score.EvaluatorName = &name
			}
			detail.EvaluatorScores = append(detail.EvaluatorScores, score)
		}
		if v, ok := item.input["actual_output"]; ok {
			s := fmt.Sprint(v)
			detail.BeforeActual = &s
		}
		if v, ok := item.input["reference_output"]; ok {
			s := fmt.Sprint(v)
			detail.Reference = &s
		}
		result.CaseDetails = append(result.CaseDetails, detail)
	}
	result.BeforeScoreDistribution = beforeDistribution
	result.AfterScoreDistribution = afterDistribution
	if score, ok := averageSlice(beforeDistribution); ok {
		result.BeforeScore = &score
	}
	if score, ok := averageSlice(afterDistribution); ok {
		result.AfterScore = &score
	}
	b, err := json.Marshal(result)
	return string(b), err
}

func averageScores(scores map[int64]float64) (float64, bool) {
	values := make([]float64, 0, len(scores))
	for _, score := range scores {
		values = append(values, score)
	}
	return averageSlice(values)
}

func averageSlice(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values)), true
}

func mapCaseEvidence(task *entity.OptimizeTaskRecord, raw json.RawMessage, index int) (map[string]any, string) {
	var source map[string]any
	_ = json.Unmarshal(raw, &source)
	var mapping optimize.OptimizeFieldMapping
	_ = json.Unmarshal([]byte(task.MappingJSON), &mapping)
	out := make(map[string]any)
	for _, field := range mapping.VariableFields {
		if field == nil {
			continue
		}
		if value, ok := lookupMappedValue(source, field.GetFromFieldName()); ok {
			out[field.GetFieldName()] = value
		}
	}
	if mapping.ActualOutputField != nil {
		if value, ok := lookupMappedValue(source, *mapping.ActualOutputField); ok {
			out["actual_output"] = value
		}
	}
	if mapping.ReferenceOutputField != nil {
		if value, ok := lookupMappedValue(source, *mapping.ReferenceOutputField); ok {
			out["reference_output"] = value
		}
	}
	caseID := strconv.Itoa(index)
	if value, ok := source["item_id"]; ok {
		caseID = fmt.Sprint(value)
	}
	if value, ok := source["case_id"]; ok {
		caseID = fmt.Sprint(value)
	}
	return out, caseID
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
