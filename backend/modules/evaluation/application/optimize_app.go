// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-loop/backend/infra/idgen"
	"github.com/coze-dev/coze-loop/backend/infra/middleware/session"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/base"
	evaluationapi "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation"
	evaluatorcommon "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/common"
	evalsetdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/eval_set"
	evaluatordto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/evaluator"
	domainexpt "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/expt"
	evalsetservice "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/eval_set"
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
	evalSets    evaluationapi.EvaluationSetService
	queue       chan int64
}

func NewOptimizeApplication(idGenerator idgen.IIDGenerator, taskRepo repo.IOptimizeTaskRepo, llm rpc.ILLMProvider, experiments IExperimentApplication, evaluators evaluationapi.EvaluatorService, evalSets evaluationapi.EvaluationSetService) optimize.OptimizeService {
	app := &OptimizeApplication{
		idgen:       idGenerator,
		repo:        taskRepo,
		llm:         llm,
		experiments: experiments,
		evaluators:  evaluators,
		evalSets:    evalSets,
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
	sourceJSON, err := json.Marshal(req.Source)
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
		SourceType: string(req.Source.Type), SourceID: sourceID, SourceJSON: string(sourceJSON),
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
	if req.Source.GetType() == optimize.OptimizeSourceTypeExperiment && req.Source.GetExperimentID() <= 0 {
		return errors.New("source.experiment_id is required")
	}
	if req.Source.GetType() == optimize.OptimizeSourceTypeEvalSet && (req.Source.GetEvalSetID() <= 0 || req.Source.GetEvalSetVersionID() <= 0) {
		return errors.New("source.eval_set_id and source.eval_set_version_id are required")
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
	// The worker runs outside the request goroutine, so its background context
	// does not contain the authenticated session injected by HTTP middleware.
	// Restore the task creator before calling evaluation-set / experiment APIs;
	// those APIs correctly enforce workspace permission from the session context.
	ctx = optimizeWorkerContext(ctx, record.CreatedBy)
	if cancelled, _ := a.repo.IsCancelRequested(ctx, taskID); cancelled {
		_ = a.repo.MarkCancelled(ctx, taskID)
		return
	}
	_ = a.repo.UpdateProgress(ctx, taskID, leaseToken, 20)
	out, caseResults, status, err := a.runOptimizationLoop(ctx, record, leaseToken)
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
	_ = a.repo.Complete(ctx, taskID, leaseToken, resultJSON, status)
}

func optimizeWorkerContext(ctx context.Context, createdBy string) context.Context {
	if createdBy == "" {
		return ctx
	}
	return session.WithCtxUser(ctx, &session.User{ID: createdBy})
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

const (
	goodcaseJudgeBatchMin             = 4
	goodcaseJudgeBatchMax             = 40
	goodcaseJudgeOutputTokenBudget    = 4096
	goodcaseJudgeOutputBaseTokens     = 512
	goodcaseJudgeOutputTokensPerCase  = 160
	goodcaseJudgeInputCharBudget      = 120000
)

func policyForMode(modeScore float64) optimizePolicy {
	policy := optimizePolicy{rounds: 2, candidates: 2, validationRatio: 0.25, minGain: 0.001, maxModelCalls: 48}
	if modeScore >= 0.67 {
		policy.rounds, policy.candidates, policy.maxModelCalls = 3, 3, 96
	} else if modeScore <= 0.33 {
		policy.rounds, policy.candidates, policy.maxModelCalls = 1, 2, 24
	}
	return policy
}

func (a *OptimizeApplication) runOptimizationLoop(ctx context.Context, task *entity.OptimizeTaskRecord, leaseToken string) (*optimizerOutput, []candidateCaseResult, string, error) {
	policy := policyForMode(task.ModeScore)
	if task.SourceType == string(optimize.OptimizeSourceTypeEvalSet) {
		var caseIDs []string
		_ = json.Unmarshal([]byte(task.CaseItemIDsJSON), &caseIDs)
		// The baseline runs once per case. Each candidate then needs one
		// generation call, one inference call per case, and one judge call per
		// bounded batch comparing baseline and candidate together.
		// Reserve a second judge call per batch. It is consumed only when the
		// first pass returns suspicious all-perfect scores.
		required := len(caseIDs) + policy.rounds*policy.candidates*(len(caseIDs)+2*goodcaseJudgeBatchCount(len(caseIDs))+1)
		if required > policy.maxModelCalls {
			policy.maxModelCalls = required
		}
	}
	ctx = context.WithValue(ctx, optimizeBudgetContextKey{}, &optimizeCallBudget{max: policy.maxModelCalls})
	baselineOutputs, err := a.executeGoodcaseBaseline(ctx, task)
	if err != nil {
		return nil, nil, "", err
	}
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
			return nil, nil, "", fmt.Errorf("renew optimize lease: %w", err)
		}
		previousBestScore := bestScore
		roundBestScore := -1e100
		var roundBestOut *optimizerOutput
		for candidate := 0; candidate < policy.candidates; candidate++ {
			out, err := a.generateCandidate(ctx, &current, feedback)
			if err != nil {
				return nil, nil, "", err
			}
			cases, err := a.executeCandidateCases(ctx, task, out, baselineOutputs)
			if err != nil {
				return nil, nil, "", err
			}
			score, scored := validationScore(cases, validationIDs)
			if task.SourceType == string(optimize.OptimizeSourceTypeEvalSet) {
				score, scored = validationGain(cases, validationIDs)
			}
			if !scored {
				return nil, nil, "", errors.New("candidate evaluation produced no comparable scores")
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
			return nil, nil, "", err
		}
		setOptimizedInstruction(&snapshot, roundBestOut.OptimizedPrompt)
		encoded, _ := json.Marshal(&snapshot)
		current.BaselinePromptJSON = string(encoded)
		_ = a.repo.UpdateProgress(ctx, task.ID, leaseToken, int32(25+(round+1)*50/policy.rounds))
	}
	if bestOut == nil {
		return nil, nil, "", errors.New("optimizer produced no candidate")
	}
	if task.SourceType == string(optimize.OptimizeSourceTypeEvalSet) && bestScore <= policy.minGain {
		bestOut.FailureModes = append([]string{
			fmt.Sprintf("no candidate improved the Goodcase baseline: best gain %.4f, required > %.4f", bestScore, policy.minGain),
		}, bestOut.FailureModes...)
		return bestOut, bestCases, entity.OptimizeTaskStatusNoGain, nil
	}
	return bestOut, bestCases, entity.OptimizeTaskStatusSucceeded, nil
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

func validationGain(cases []candidateCaseResult, validation map[string]struct{}) (float64, bool) {
	var values []float64
	for _, item := range cases {
		if len(validation) > 0 {
			if _, ok := validation[item.caseID]; !ok {
				continue
			}
		}
		before, hasBefore := averageScores(item.beforeScores)
		after, hasAfter := averageScores(item.afterScores)
		if hasBefore && hasAfter {
			values = append(values, after-before)
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

func (a *OptimizeApplication) executeCandidateCases(ctx context.Context, task *entity.OptimizeTaskRecord, out *optimizerOutput, baselineOutputs map[string]string) ([]candidateCaseResult, error) {
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
	var evaluationSetItems []*evalsetdto.EvaluationSetItem
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
	} else if response, ok := evidence.(*evalsetservice.BatchGetEvaluationSetItemsResponse); ok && response != nil {
		if len(response.Items) == 0 {
			return nil, errors.New("no evaluation set items matched the selected IDs")
		}
		evaluationSetItems = response.Items
		itemEvidence = make([]json.RawMessage, 0, len(response.Items))
		for _, item := range response.Items {
			encoded, marshalErr := json.Marshal(normalizeEvaluationSetItem(item))
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal evaluation set item: %w", marshalErr)
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
		if task.SourceType == string(optimize.OptimizeSourceTypeEvalSet) {
			baseline, ok := baselineOutputs[caseID]
			if !ok || strings.TrimSpace(baseline) == "" {
				return nil, fmt.Errorf("Goodcase baseline output is missing for case %s", caseID)
			}
			mapped["actual_output"] = baseline
		}
		caseMessages, convertErr := buildOptimizeCaseMessages(&snapshot, mapped, caseID)
		if convertErr != nil {
			return nil, convertErr
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
	if len(evaluationSetItems) > 0 {
		if err := a.judgeGoodcaseBatches(ctx, task, results); err != nil {
			return nil, err
		}
	} else if len(experimentItems) > 0 {
		a.populateEvaluatorNames(ctx, task.WorkspaceID, results)
	}
	return results, nil
}

func (a *OptimizeApplication) executeGoodcaseBaseline(ctx context.Context, task *entity.OptimizeTaskRecord) (map[string]string, error) {
	if task.SourceType != string(optimize.OptimizeSourceTypeEvalSet) {
		return nil, nil
	}
	var snapshot optimize.OptimizePromptSnapshot
	if err := json.Unmarshal([]byte(task.BaselinePromptJSON), &snapshot); err != nil {
		return nil, fmt.Errorf("decode Goodcase baseline snapshot: %w", err)
	}
	evidence, err := a.loadEvidence(ctx, task)
	if err != nil {
		return nil, err
	}
	response, ok := evidence.(*evalsetservice.BatchGetEvaluationSetItemsResponse)
	if !ok || response == nil || len(response.Items) == 0 {
		return nil, errors.New("no evaluation set items available for Goodcase baseline")
	}
	outputs := make(map[string]string, len(response.Items))
	for index, item := range response.Items {
		if cancelled, _ := a.repo.IsCancelRequested(ctx, task.ID); cancelled {
			return nil, errors.New("Goodcase baseline execution cancelled")
		}
		encoded, err := json.Marshal(normalizeEvaluationSetItem(item))
		if err != nil {
			return nil, fmt.Errorf("marshal Goodcase baseline item: %w", err)
		}
		mapped, caseID := mapCaseEvidence(task, encoded, index)
		messages, err := buildOptimizeCaseMessages(&snapshot, mapped, caseID)
		if err != nil {
			return nil, err
		}
		output, err := a.callBaseline(ctx, task, messages)
		if err != nil {
			return nil, fmt.Errorf("execute Goodcase baseline case %s: %w", caseID, err)
		}
		outputs[caseID] = output
	}
	return outputs, nil
}

func buildOptimizeCaseMessages(snapshot *optimize.OptimizePromptSnapshot, mapped map[string]any, caseID string) ([]*entity.Message, error) {
	rendered, err := renderCandidateMessages(snapshot.Messages, mapped)
	if err != nil {
		return nil, fmt.Errorf("render candidate case %s: %w", caseID, err)
	}
	messages, err := convertCandidateMessages(rendered)
	if err != nil {
		return nil, fmt.Errorf("convert candidate case %s: %w", caseID, err)
	}
	if len(messages) == 0 {
		return nil, errors.New("candidate prompt has no messages")
	}
	messages, err = appendMappedMultimodalEvidence(messages, mapped)
	if err != nil {
		return nil, fmt.Errorf("append candidate evidence for case %s: %w", caseID, err)
	}
	return messages, nil
}

const goodcaseJudgeEvaluatorID int64 = 0

func goodcaseJudgeBatchCount(caseCount int) int {
	if caseCount <= 0 {
		return 0
	}
	// Reserve against the smallest dynamic batch so call-budget remains safe.
	return (caseCount + goodcaseJudgeBatchMin - 1) / goodcaseJudgeBatchMin
}

type goodcaseJudgeInput struct {
	CaseID          string         `json:"case_id"`
	Input           map[string]any `json:"input"`
	ReferenceOutput string         `json:"reference_output"`
	BaselineActual  *string        `json:"baseline_actual,omitempty"`
	CandidateActual string         `json:"candidate_actual"`
}

type goodcaseJudgeScore struct {
	CaseID      string   `json:"case_id"`
	BeforeScore *float64 `json:"before_score,omitempty"`
	AfterScore  *float64 `json:"after_score"`
	Reason      string   `json:"reason,omitempty"`
}

type goodcaseJudgeOutput struct {
	Scores []goodcaseJudgeScore `json:"scores"`
}

func (a *OptimizeApplication) judgeGoodcaseBatches(ctx context.Context, task *entity.OptimizeTaskRecord, results []candidateCaseResult) error {
	inputs := make([]goodcaseJudgeInput, 0, len(results))
	for index := range results {
		item := &results[index]
		reference, ok := item.input["reference_output"]
		if !ok || strings.TrimSpace(fmt.Sprint(reference)) == "" {
			return fmt.Errorf("goodcase %s reference_output mapping did not resolve to a value", item.caseID)
		}
		var baseline *string
		if actual, exists := item.input["actual_output"]; exists && strings.TrimSpace(fmt.Sprint(actual)) != "" {
			value := fmt.Sprint(actual)
			baseline = &value
		}
		inputs = append(inputs, goodcaseJudgeInput{
			CaseID: item.caseID, Input: goodcaseJudgeVariables(item.input), ReferenceOutput: fmt.Sprint(reference),
			BaselineActual: baseline, CandidateActual: item.output,
		})
	}

	offset := 0
	batchNo := 0
	for offset < len(inputs) {
		batchNo++
		remaining := inputs[offset:]
		size := estimateGoodcaseJudgeBatchSize(remaining)
		if size > len(remaining) {
			size = len(remaining)
		}
		window := remaining[:size]
		size = estimateGoodcaseJudgeBatchSize(window)
		if size > len(window) {
			size = len(window)
		}
		window = remaining[:size]
		judged, err := a.callGoodcaseJudgeBatchWithSplit(ctx, task, window)
		if err != nil {
			return fmt.Errorf("judge goodcase batch %d: %w", batchNo, err)
		}
		if err := applyGoodcaseJudgeOutput(results[offset:offset+len(window)], judged); err != nil {
			return fmt.Errorf("apply goodcase batch %d: %w", batchNo, err)
		}
		offset += len(window)
	}
	return nil
}

func estimateGoodcaseJudgeBatchSize(inputs []goodcaseJudgeInput) int {
	if len(inputs) == 0 {
		return goodcaseJudgeBatchMin
	}
	maxByOutput := (goodcaseJudgeOutputTokenBudget - goodcaseJudgeOutputBaseTokens) / goodcaseJudgeOutputTokensPerCase
	if maxByOutput < goodcaseJudgeBatchMin {
		maxByOutput = goodcaseJudgeBatchMin
	}
	if maxByOutput > goodcaseJudgeBatchMax {
		maxByOutput = goodcaseJudgeBatchMax
	}

	totalChars := 0
	for _, input := range inputs {
		totalChars += len(input.ReferenceOutput) + len(input.CandidateActual)
		if input.BaselineActual != nil {
			totalChars += len(*input.BaselineActual)
		}
		encoded, err := json.Marshal(input.Input)
		if err != nil {
			totalChars += 256
			continue
		}
		totalChars += len(encoded)
	}
	avgChars := totalChars / len(inputs)
	if avgChars < 1 {
		avgChars = 1
	}
	maxByInput := goodcaseJudgeInputCharBudget / avgChars
	if maxByInput < goodcaseJudgeBatchMin {
		maxByInput = goodcaseJudgeBatchMin
	}

	size := maxByOutput
	if maxByInput < size {
		size = maxByInput
	}
	if size > goodcaseJudgeBatchMax {
		size = goodcaseJudgeBatchMax
	}
	if size < goodcaseJudgeBatchMin {
		size = goodcaseJudgeBatchMin
	}
	if size > len(inputs) {
		size = len(inputs)
	}
	return size
}

func (a *OptimizeApplication) callGoodcaseJudgeBatchWithSplit(ctx context.Context, task *entity.OptimizeTaskRecord, inputs []goodcaseJudgeInput) (*goodcaseJudgeOutput, error) {
	judged, err := a.callGoodcaseJudgeBatch(ctx, task, inputs)
	if err == nil {
		coverErr := validateGoodcaseJudgeCoverage(inputs, judged)
		if coverErr == nil {
			return judged, nil
		}
		err = coverErr
	}
	if !isRetriableGoodcaseJudgeError(err) || len(inputs) <= 1 {
		return nil, err
	}
	mid := len(inputs) / 2
	left, leftErr := a.callGoodcaseJudgeBatchWithSplit(ctx, task, inputs[:mid])
	if leftErr != nil {
		return nil, leftErr
	}
	right, rightErr := a.callGoodcaseJudgeBatchWithSplit(ctx, task, inputs[mid:])
	if rightErr != nil {
		return nil, rightErr
	}
	return mergeGoodcaseJudgeOutputs(left, right), nil
}

func validateGoodcaseJudgeCoverage(inputs []goodcaseJudgeInput, judged *goodcaseJudgeOutput) error {
	if judged == nil {
		return errors.New("goodcase judge output is nil")
	}
	expected := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		expected[input.CaseID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(judged.Scores))
	for _, score := range judged.Scores {
		if _, ok := expected[score.CaseID]; !ok {
			return fmt.Errorf("unknown case_id %q", score.CaseID)
		}
		if _, dup := seen[score.CaseID]; dup {
			return fmt.Errorf("duplicate case_id %q", score.CaseID)
		}
		if score.AfterScore == nil {
			return fmt.Errorf("case %s missing after_score", score.CaseID)
		}
		seen[score.CaseID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("judge returned %d of %d cases", len(seen), len(expected))
	}
	return nil
}

func mergeGoodcaseJudgeOutputs(parts ...*goodcaseJudgeOutput) *goodcaseJudgeOutput {
	merged := &goodcaseJudgeOutput{}
	for _, part := range parts {
		if part == nil {
			continue
		}
		merged.Scores = append(merged.Scores, part.Scores...)
	}
	return merged
}

func isRetriableGoodcaseJudgeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"parse goodcase", "missing case", "unknown case_id", "duplicate case_id",
		"judge returned", "context", "token", "truncated", "too long", "maximum context",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func goodcaseJudgeVariables(input map[string]any) map[string]any {
	variables := make(map[string]any, len(input))
	for key, value := range input {
		if key == "actual_output" || key == "reference_output" {
			continue
		}
		variables[key] = value
	}
	return variables
}

func (a *OptimizeApplication) callGoodcaseJudgeBatch(ctx context.Context, task *entity.OptimizeTaskRecord, inputs []goodcaseJudgeInput) (*goodcaseJudgeOutput, error) {
	judged, err := a.callGoodcaseJudgeBatchOnce(ctx, task, inputs, false)
	if err != nil {
		return nil, err
	}
	guardGoodcaseJudgeOutput(inputs, judged)
	if !goodcaseJudgeOutputNeedsReview(judged) {
		return judged, nil
	}
	reviewed, err := a.callGoodcaseJudgeBatchOnce(ctx, task, inputs, true)
	if err != nil {
		return nil, fmt.Errorf("review suspicious all-perfect goodcase scores: %w", err)
	}
	guardGoodcaseJudgeOutput(inputs, reviewed)
	if err := mergeGoodcaseJudgeReview(judged, reviewed); err != nil {
		return nil, fmt.Errorf("merge suspicious goodcase score review: %w", err)
	}
	return judged, nil
}

func (a *OptimizeApplication) callGoodcaseJudgeBatchOnce(ctx context.Context, task *entity.OptimizeTaskRecord, inputs []goodcaseJudgeInput, review bool) (*goodcaseJudgeOutput, error) {
	payload, err := json.Marshal(map[string]any{"cases": inputs})
	if err != nil {
		return nil, err
	}
	systemText := "你是严格的批量评测裁判。逐条比较模型回答与优质参考答案在事实、约束和输出格式上的一致性，不得遗漏、合并或改写 case_id。只返回 JSON：{\"scores\":[{\"case_id\":\"原值\",\"before_score\":0到1之间的数字或省略,\"after_score\":0到1之间的数字,\"reason\":\"简短原因\"}]}。仅当输入含 baseline_actual 时返回 before_score。"
	if review {
		systemText += " 这是对可疑全满分结果的独立复核。不得沿用先前结论；重点检查拒答、空输出、JSON/字段缺失、类型变化和格式协议破坏。只有与参考答案在事实和协议上均完全一致时才可给 1 分。"
	}
	userText := string(payload)
	modelID, temperature := task.OptimizerModelID, 0.0
	maxTokens := int32(512 + len(inputs)*160)
	if maxTokens > 4096 {
		maxTokens = 4096
	}
	reply, err := a.callLLMWithRetry(ctx, &entity.LLMCallParam{
		SpaceID: task.WorkspaceID, EvaluatorID: fmt.Sprintf("optimize_goodcase:%d", task.ID), Scenario: entity.ScenarioEvaluator,
		Messages:    []*entity.Message{{Role: entity.RoleSystem, Content: &entity.Content{Text: &systemText}}, {Role: entity.RoleUser, Content: &entity.Content{Text: &userText}}},
		ModelConfig: &entity.ModelConfig{ModelID: &modelID, MaxTokens: &maxTokens, Temperature: &temperature},
	})
	if err != nil {
		return nil, err
	}
	if reply == nil || reply.Content == nil {
		return nil, errors.New("goodcase judge returned empty output")
	}
	var judged goodcaseJudgeOutput
	if err := json.Unmarshal([]byte(stripJSONFence(*reply.Content)), &judged); err != nil {
		return nil, fmt.Errorf("parse goodcase judge output: %w", err)
	}
	return &judged, nil
}

func guardGoodcaseJudgeOutput(inputs []goodcaseJudgeInput, judged *goodcaseJudgeOutput) {
	if judged == nil {
		return
	}
	byID := make(map[string]goodcaseJudgeInput, len(inputs))
	for _, input := range inputs {
		byID[input.CaseID] = input
	}
	for index := range judged.Scores {
		score := &judged.Scores[index]
		input, ok := byID[score.CaseID]
		if !ok {
			continue
		}
		if input.BaselineActual != nil {
			if err := validateGoodcaseAnswer(input.ReferenceOutput, *input.BaselineActual); err != nil {
				zero := 0.0
				score.BeforeScore = &zero
				score.Reason = appendJudgeReason(score.Reason, "baseline protocol guard: "+err.Error())
			}
		}
		if err := validateGoodcaseAnswer(input.ReferenceOutput, input.CandidateActual); err != nil {
			zero := 0.0
			score.AfterScore = &zero
			score.Reason = appendJudgeReason(score.Reason, "candidate protocol guard: "+err.Error())
		}
	}
}

func validateGoodcaseAnswer(reference, actual string) error {
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return errors.New("empty output")
	}
	if isObviousRefusal(actual) && !isObviousRefusal(reference) {
		return errors.New("obvious refusal")
	}
	var expected any
	if err := json.Unmarshal([]byte(strings.TrimSpace(reference)), &expected); err != nil {
		return nil
	}
	switch expected.(type) {
	case map[string]any, []any:
	default:
		return nil
	}
	var candidate any
	if err := json.Unmarshal([]byte(actual), &candidate); err != nil {
		return errors.New("expected structured JSON output")
	}
	return validateReferenceJSONShape(expected, candidate, "$")
}

func validateReferenceJSONShape(expected, candidate any, path string) error {
	switch value := expected.(type) {
	case map[string]any:
		got, ok := candidate.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for key, child := range value {
			actual, exists := got[key]
			if !exists {
				return fmt.Errorf("%s.%s is required", path, key)
			}
			if err := validateReferenceJSONShape(child, actual, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		got, ok := candidate.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if len(value) > 0 {
			for index, child := range got {
				if err := validateReferenceJSONShape(value[0], child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	case string:
		if candidate != nil {
			if _, ok := candidate.(string); !ok {
				return fmt.Errorf("%s must be a string", path)
			}
		}
	case float64:
		if candidate != nil {
			if _, ok := candidate.(float64); !ok {
				return fmt.Errorf("%s must be a number", path)
			}
		}
	case bool:
		if candidate != nil {
			if _, ok := candidate.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", path)
			}
		}
	}
	return nil
}

func isObviousRefusal(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	normalized = strings.TrimPrefix(normalized, "as an ai, ")
	for _, phrase := range []string{
		"i cannot assist", "i can't assist", "i am unable to", "i'm unable to",
		"i’m unable to", "i'm sorry, but", "i’m sorry, but",
		"cannot comply", "can't comply", "unable to comply",
		"抱歉，我无法", "抱歉，无法", "抱歉，我不能", "我无法帮助", "我不能帮助", "无法协助此请求",
	} {
		if strings.HasPrefix(normalized, phrase) {
			return true
		}
	}
	return false
}

func goodcaseJudgeOutputNeedsReview(judged *goodcaseJudgeOutput) bool {
	if judged == nil || len(judged.Scores) == 0 {
		return false
	}
	for _, score := range judged.Scores {
		if score.AfterScore == nil || *score.AfterScore < 0.999 {
			return false
		}
	}
	return true
}

func mergeGoodcaseJudgeReview(first, review *goodcaseJudgeOutput) error {
	if first == nil || review == nil {
		return errors.New("goodcase judge review output is nil")
	}
	byID := make(map[string]goodcaseJudgeScore, len(review.Scores))
	for _, score := range review.Scores {
		if _, exists := byID[score.CaseID]; exists {
			return fmt.Errorf("duplicate review case_id %q", score.CaseID)
		}
		byID[score.CaseID] = score
	}
	for index := range first.Scores {
		score := &first.Scores[index]
		rechecked, ok := byID[score.CaseID]
		if !ok || rechecked.AfterScore == nil {
			return fmt.Errorf("review missing case_id %q", score.CaseID)
		}
		score.AfterScore = lowerScore(score.AfterScore, rechecked.AfterScore)
		if score.BeforeScore != nil || rechecked.BeforeScore != nil {
			if score.BeforeScore == nil || rechecked.BeforeScore == nil {
				return fmt.Errorf("review before_score mismatch for case_id %q", score.CaseID)
			}
			score.BeforeScore = lowerScore(score.BeforeScore, rechecked.BeforeScore)
		}
		score.Reason = appendJudgeReason(score.Reason, "review: "+rechecked.Reason)
	}
	if len(byID) != len(first.Scores) {
		return fmt.Errorf("review returned %d cases, expected %d", len(byID), len(first.Scores))
	}
	return nil
}

func lowerScore(left, right *float64) *float64 {
	value := *left
	if *right < value {
		value = *right
	}
	return &value
}

func appendJudgeReason(current, addition string) string {
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return addition
	}
	return current + "; " + addition
}

func applyGoodcaseJudgeOutput(results []candidateCaseResult, judged *goodcaseJudgeOutput) error {
	if judged == nil {
		return errors.New("goodcase judge output is nil")
	}
	byID := make(map[string]*candidateCaseResult, len(results))
	for index := range results {
		byID[results[index].caseID] = &results[index]
	}
	seen := make(map[string]struct{}, len(judged.Scores))
	for _, score := range judged.Scores {
		item, ok := byID[score.CaseID]
		if !ok {
			return fmt.Errorf("unknown case_id %q", score.CaseID)
		}
		if _, duplicated := seen[score.CaseID]; duplicated {
			return fmt.Errorf("duplicate case_id %q", score.CaseID)
		}
		if score.AfterScore == nil {
			return fmt.Errorf("case %s missing after_score", score.CaseID)
		}
		if *score.AfterScore < 0 || *score.AfterScore > 1 {
			return fmt.Errorf("case %s after_score out of range: %v", score.CaseID, *score.AfterScore)
		}
		item.beforeScores = map[int64]float64{}
		item.afterScores = map[int64]float64{goodcaseJudgeEvaluatorID: *score.AfterScore}
		item.evaluatorNames = map[int64]string{goodcaseJudgeEvaluatorID: "Goodcase Judge"}
		baseline, hasBaseline := item.input["actual_output"]
		hasBaseline = hasBaseline && strings.TrimSpace(fmt.Sprint(baseline)) != ""
		if hasBaseline && score.BeforeScore == nil {
			return fmt.Errorf("case %s missing before_score", score.CaseID)
		}
		if !hasBaseline && score.BeforeScore != nil {
			return fmt.Errorf("case %s returned before_score without baseline_actual", score.CaseID)
		}
		if score.BeforeScore != nil {
			if *score.BeforeScore < 0 || *score.BeforeScore > 1 {
				return fmt.Errorf("case %s before_score out of range: %v", score.CaseID, *score.BeforeScore)
			}
			item.beforeScores[goodcaseJudgeEvaluatorID] = *score.BeforeScore
		}
		seen[score.CaseID] = struct{}{}
	}
	if len(seen) != len(results) {
		return fmt.Errorf("judge returned %d of %d cases", len(seen), len(results))
	}
	return nil
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

func (a *OptimizeApplication) populateEvaluatorNames(ctx context.Context, workspaceID int64, results []candidateCaseResult) {
	ids := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, result := range results {
		for evaluatorID := range result.beforeScores {
			if _, ok := seen[evaluatorID]; !ok {
				seen[evaluatorID] = struct{}{}
				ids = append(ids, evaluatorID)
			}
		}
		for evaluatorID := range result.afterScores {
			if _, ok := seen[evaluatorID]; !ok {
				seen[evaluatorID] = struct{}{}
				ids = append(ids, evaluatorID)
			}
		}
	}
	var evaluators []*evaluatordto.Evaluator
	if a.evaluators != nil && len(ids) > 0 {
		resp, err := a.evaluators.BatchGetEvaluatorVersions(ctx, &evaluatorservice.BatchGetEvaluatorVersionsRequest{
			WorkspaceID: workspaceID, EvaluatorVersionIds: ids,
		})
		if err == nil && resp != nil {
			evaluators = resp.GetEvaluators()
		}
	}
	fillEvaluatorNames(results, evaluators)
}

func fillEvaluatorNames(results []candidateCaseResult, evaluators []*evaluatordto.Evaluator) {
	names := make(map[int64]string, len(evaluators))
	for _, evaluator := range evaluators {
		if evaluator == nil || evaluator.GetCurrentVersion() == nil || evaluator.GetCurrentVersion().GetID() <= 0 || strings.TrimSpace(evaluator.GetName()) == "" {
			continue
		}
		names[evaluator.GetCurrentVersion().GetID()] = evaluator.GetName()
	}
	for index := range results {
		if results[index].evaluatorNames == nil {
			results[index].evaluatorNames = make(map[int64]string)
		}
		for evaluatorID := range results[index].beforeScores {
			name := names[evaluatorID]
			if name == "" {
				name = fmt.Sprintf("Evaluator %d", evaluatorID)
			}
			results[index].evaluatorNames[evaluatorID] = name
		}
		for evaluatorID := range results[index].afterScores {
			name := names[evaluatorID]
			if name == "" {
				name = fmt.Sprintf("Evaluator %d", evaluatorID)
			}
			results[index].evaluatorNames[evaluatorID] = name
		}
	}
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
	return a.callOptimizePrompt(ctx, task, messages, "optimize_candidate")
}

func (a *OptimizeApplication) callBaseline(ctx context.Context, task *entity.OptimizeTaskRecord, messages []*entity.Message) (string, error) {
	return a.callOptimizePrompt(ctx, task, messages, "optimize_baseline")
}

func (a *OptimizeApplication) callOptimizePrompt(ctx context.Context, task *entity.OptimizeTaskRecord, messages []*entity.Message, scenario string) (string, error) {
	modelID := task.OptimizerModelID
	maxTokens := int32(4096)
	temperature := 0.2
	reply, err := a.callLLMWithRetry(ctx, &entity.LLMCallParam{
		SpaceID: task.WorkspaceID, EvaluatorID: fmt.Sprintf("%s:%d", scenario, task.ID),
		Scenario: entity.ScenarioEvaluator, Messages: messages,
		ModelConfig: &entity.ModelConfig{ModelID: &modelID, MaxTokens: &maxTokens, Temperature: &temperature},
	})
	if err != nil {
		return "", fmt.Errorf("execute %s: %w", scenario, err)
	}
	if reply == nil || reply.Content == nil || strings.TrimSpace(*reply.Content) == "" {
		return "", fmt.Errorf("%s execution returned empty output", scenario)
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
	normalizedPrompt, unknownVariables, err := normalizeOptimizerVariables(task.BaselinePromptJSON, out.OptimizedPrompt)
	if err != nil {
		return nil, err
	}
	out.OptimizedPrompt = normalizedPrompt
	if len(unknownVariables) > 0 {
		out.FailureModes = append(out.FailureModes, fmt.Sprintf("优化模型生成了未定义变量，已按普通文本处理：%s", strings.Join(unknownVariables, ", ")))
	}
	return &out, nil
}

func normalizeOptimizerVariables(snapshotJSON, optimizedPrompt string) (string, []string, error) {
	var snapshot optimize.OptimizePromptSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", nil, fmt.Errorf("decode prompt snapshot for variable validation: %w", err)
	}
	allowed := make(map[string]struct{}, len(snapshot.VariableDefs))
	for _, definition := range snapshot.VariableDefs {
		if definition != nil && definition.GetKey() != "" {
			allowed[definition.GetKey()] = struct{}{}
		}
	}
	for _, message := range snapshot.Messages {
		if message == nil {
			continue
		}
		for _, match := range optimizeVariablePattern.FindAllStringSubmatch(message.GetContent(), -1) {
			allowed[match[1]] = struct{}{}
		}
		for _, part := range message.GetParts() {
			if part == nil {
				continue
			}
			for _, match := range optimizeVariablePattern.FindAllStringSubmatch(part.GetText(), -1) {
				allowed[match[1]] = struct{}{}
			}
		}
	}
	unknownSet := make(map[string]struct{})
	normalized := optimizeVariablePattern.ReplaceAllStringFunc(optimizedPrompt, func(match string) string {
		parts := optimizeVariablePattern.FindStringSubmatch(match)
		if _, ok := allowed[parts[1]]; ok {
			return match
		}
		unknownSet[parts[1]] = struct{}{}
		return parts[1]
	})
	unknown := make([]string, 0, len(unknownSet))
	for variable := range unknownSet {
		unknown = append(unknown, variable)
	}
	slices.Sort(unknown)
	return normalized, unknown, nil
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
	if task.SourceType == string(optimize.OptimizeSourceTypeEvalSet) {
		if a.evalSets == nil {
			return nil, errors.New("evaluation set service is unavailable")
		}
		var source optimize.OptimizeSource
		if task.SourceJSON == "" || json.Unmarshal([]byte(task.SourceJSON), &source) != nil {
			return nil, errors.New("evaluation set source snapshot is missing")
		}
		var rawIDs []string
		if err := json.Unmarshal([]byte(task.CaseItemIDsJSON), &rawIDs); err != nil {
			return nil, fmt.Errorf("decode evaluation set item IDs: %w", err)
		}
		itemIDs := make([]int64, 0, len(rawIDs))
		for _, rawID := range rawIDs {
			id, err := strconv.ParseInt(rawID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid evaluation set item ID %q", rawID)
			}
			itemIDs = append(itemIDs, id)
		}
		versionID := source.GetEvalSetVersionID()
		resp, err := a.evalSets.BatchGetEvaluationSetItems(ctx, &evalsetservice.BatchGetEvaluationSetItemsRequest{
			WorkspaceID: task.WorkspaceID, EvaluationSetID: source.GetEvalSetID(), VersionID: &versionID, ItemIds: itemIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("load evaluation set evidence: %w", err)
		}
		return resp, nil
	}
	if task.SourceType != string(optimize.OptimizeSourceTypeExperiment) || a.experiments == nil {
		return nil, fmt.Errorf("unsupported optimize source type %q", task.SourceType)
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

func normalizeEvaluationSetItem(item *evalsetdto.EvaluationSetItem) map[string]any {
	result := map[string]any{}
	if item == nil {
		return result
	}
	result["id"] = item.GetID()
	result["item_id"] = item.GetItemID()
	result["item_key"] = item.GetItemKey()
	for turnIndex, turn := range item.GetTurns() {
		if turn == nil {
			continue
		}
		for _, field := range turn.GetFieldDataList() {
			if field == nil {
				continue
			}
			key := field.GetKey()
			if key == "" {
				key = field.GetName()
			}
			if key == "" {
				continue
			}
			value := normalizeEvaluationContent(field.GetContent())
			if turnIndex == 0 {
				result[key] = value
				if name := field.GetName(); name != "" {
					result[name] = value
				}
			}
			result[fmt.Sprintf("turns.%d.%s", turnIndex, key)] = value
		}
	}
	return result
}

func normalizeEvaluationContent(content *evaluatorcommon.Content) any {
	if content == nil {
		return nil
	}
	if len(content.GetMultiPart()) > 0 {
		parts := make([]any, 0, len(content.GetMultiPart()))
		for _, part := range content.GetMultiPart() {
			parts = append(parts, normalizeEvaluationContent(part))
		}
		return parts
	}
	// EvaluationSet image/video parts may also carry an empty, non-nil Text
	// pointer. Prefer the declared content type and concrete media fields so
	// those parts are not silently normalized to an empty string.
	switch content.GetContentType() {
	case evaluatorcommon.ContentTypeImage:
		if content.Image != nil {
			return map[string]any{"content_type": string(entity.ContentTypeImage), "image": content.Image}
		}
	case evaluatorcommon.ContentTypeVideo:
		if content.Video != nil {
			return map[string]any{"content_type": string(entity.ContentTypeVideo), "video": content.Video}
		}
	case evaluatorcommon.ContentTypeAudio:
		if content.Audio != nil {
			return content.Audio
		}
	case evaluatorcommon.ContentTypeText:
		return content.GetText()
	}
	if content.Image != nil {
		return map[string]any{"content_type": string(entity.ContentTypeImage), "image": content.Image}
	}
	if content.Video != nil {
		return map[string]any{"content_type": string(entity.ContentTypeVideo), "video": content.Video}
	}
	if content.Audio != nil {
		return content.Audio
	}
	if content.Text != nil {
		return content.GetText()
	}
	return content
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&source)
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
		if record.SourceJSON != "" {
			source = &optimize.OptimizeSource{}
			if err := json.Unmarshal([]byte(record.SourceJSON), source); err != nil {
				return nil, err
			}
		} else {
			source = &optimize.OptimizeSource{Type: optimize.OptimizeSourceType(record.SourceType)}
			if source.Type == optimize.OptimizeSourceTypeExperiment {
				source.ExperimentID = &record.SourceID
			} else {
				source.EvalSetVersionID = &record.SourceID
			}
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
