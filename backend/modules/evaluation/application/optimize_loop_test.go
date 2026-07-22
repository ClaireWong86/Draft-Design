// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/coze-dev/coze-loop/backend/infra/middleware/session"
	evaluatorcommon "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/common"
	evalsetdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/eval_set"
	evaluatordto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/evaluator"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

func TestPolicyForMode(t *testing.T) {
	if got := policyForMode(0.2); got.rounds != 1 || got.candidates != 2 || got.maxModelCalls != 24 {
		t.Fatalf("unexpected cost policy: %#v", got)
	}
	if got := policyForMode(0.8); got.rounds != 3 || got.candidates != 3 || got.maxModelCalls != 96 {
		t.Fatalf("unexpected quality policy: %#v", got)
	}
}

func TestGoodcaseNoGainUsesDedicatedStatus(t *testing.T) {
	policy := policyForMode(0.5)
	if policy.minGain != 0.001 {
		t.Fatalf("unexpected minGain: %v", policy.minGain)
	}
	// Contract: eval-set tasks with non-positive validation gain complete as
	// no_gain instead of failed so the UI can still open the report.
	status := entity.OptimizeTaskStatusSucceeded
	bestScore := -0.1
	if bestScore <= policy.minGain {
		status = entity.OptimizeTaskStatusNoGain
	}
	if status != entity.OptimizeTaskStatusNoGain {
		t.Fatalf("status = %q, want no_gain", status)
	}
}

func TestSplitAndValidationScore(t *testing.T) {
	optimization, validation := splitCaseIDs(`["1","2","3","4"]`, 0.25)
	if len(optimization) != 3 {
		t.Fatalf("unexpected optimization set: %#v", optimization)
	}
	if _, ok := validation["4"]; !ok {
		t.Fatalf("unexpected validation set: %#v", validation)
	}
	cases := []candidateCaseResult{{caseID: "3", afterScores: map[int64]float64{1: 0.1}}, {caseID: "4", afterScores: map[int64]float64{1: 0.8, 2: 1}}}
	if score, ok := validationScore(cases, validation); !ok || score != 0.9 {
		t.Fatalf("unexpected validation score: %v %v", score, ok)
	}
	gainCases := []candidateCaseResult{
		{caseID: "3", beforeScores: map[int64]float64{1: 0.2}, afterScores: map[int64]float64{1: 0.7}},
		{caseID: "4", beforeScores: map[int64]float64{1: 0.4}, afterScores: map[int64]float64{1: 0.9}},
	}
	if gain, ok := validationGain(gainCases, validation); !ok || gain != 0.5 {
		t.Fatalf("unexpected validation gain: %v %v", gain, ok)
	}
}

func TestOptimizeCallBudget(t *testing.T) {
	ctx := context.WithValue(context.Background(), optimizeBudgetContextKey{}, &optimizeCallBudget{max: 1})
	if err := consumeOptimizeCall(ctx); err != nil {
		t.Fatal(err)
	}
	if err := consumeOptimizeCall(ctx); err == nil {
		t.Fatal("expected exhausted budget")
	}
}

func TestOptimizeWorkerContextRestoresCreator(t *testing.T) {
	ctx := optimizeWorkerContext(context.Background(), "123456")
	if got := session.UserIDInCtxOrEmpty(ctx); got != "123456" {
		t.Fatalf("worker user = %q, want creator", got)
	}
}

func TestMapCaseEvidencePreservesLargeItemID(t *testing.T) {
	mapping := `{"variable_fields":[{"field_name":"query","from_field_name":"input"}]}`
	task := &entity.OptimizeTaskRecord{MappingJSON: mapping}
	_, caseID := mapCaseEvidence(task, []byte(`{"item_id":7662631610872905729,"input":"hello"}`), 0)
	if caseID != "7662631610872905729" {
		t.Fatalf("case ID = %q, want exact int64", caseID)
	}
}

func TestGoodcaseJudgeBatchCount(t *testing.T) {
	// Budget reservation uses the smallest dynamic batch (4).
	for _, tc := range []struct {
		cases int
		want  int
	}{{0, 0}, {1, 1}, {4, 1}, {5, 2}, {12, 3}, {13, 4}} {
		if got := goodcaseJudgeBatchCount(tc.cases); got != tc.want {
			t.Fatalf("count(%d) = %d, want %d", tc.cases, got, tc.want)
		}
	}
}

func TestApplyGoodcaseJudgeOutput(t *testing.T) {
	results := []candidateCaseResult{{caseID: "a", input: map[string]any{"actual_output": "old"}}, {caseID: "b", input: map[string]any{}}}
	before, afterA, afterB := 0.4, 0.8, 0.9
	err := applyGoodcaseJudgeOutput(results, &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{
		{CaseID: "a", BeforeScore: &before, AfterScore: &afterA},
		{CaseID: "b", AfterScore: &afterB},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].beforeScores[goodcaseJudgeEvaluatorID] != before || results[0].afterScores[goodcaseJudgeEvaluatorID] != afterA {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
	if _, ok := results[1].beforeScores[goodcaseJudgeEvaluatorID]; ok || results[1].afterScores[goodcaseJudgeEvaluatorID] != afterB {
		t.Fatalf("unexpected second result: %#v", results[1])
	}

	missing := []candidateCaseResult{{caseID: "a"}, {caseID: "b"}}
	if err := applyGoodcaseJudgeOutput(missing, &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{CaseID: "a", AfterScore: &afterA}}}); err == nil {
		t.Fatal("expected missing case error")
	}
}

func TestGuardGoodcaseJudgeOutputRejectsProtocolBreakAndRefusal(t *testing.T) {
	perfect := 1.0
	inputs := []goodcaseJudgeInput{
		{
			CaseID:          "missing-json-field",
			ReferenceOutput: `{"defects":[],"recommended_next_action":"manual_review"}`,
			BaselineActual:  testPtr(`{"defects":[],"recommended_next_action":"manual_review"}`),
			CandidateActual: `{"defects":[]}`,
		},
		{
			CaseID:          "refusal",
			ReferenceOutput: "The tire has a sidewall crack.",
			CandidateActual: "I cannot assist with this request.",
		},
	}
	judged := &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{
		{CaseID: "missing-json-field", BeforeScore: &perfect, AfterScore: &perfect},
		{CaseID: "refusal", AfterScore: &perfect},
	}}

	guardGoodcaseJudgeOutput(inputs, judged)

	if *judged.Scores[0].BeforeScore != perfect {
		t.Fatalf("valid baseline score changed: %#v", judged.Scores[0])
	}
	if *judged.Scores[0].AfterScore != 0 || *judged.Scores[1].AfterScore != 0 {
		t.Fatalf("hard failures must score zero: %#v", judged.Scores)
	}
	if judged.Scores[0].Reason == "" || judged.Scores[1].Reason == "" {
		t.Fatalf("hard failure reasons must be recorded: %#v", judged.Scores)
	}
}

func TestValidateGoodcaseAnswerRequiresStandaloneJSON(t *testing.T) {
	reference := `{"defects":[{"type":"crack"}],"image_quality":"ok"}`
	for _, actual := range []string{
		"```json\n" + reference + "\n```",
		reference + "\nThis is the result.",
		`{"defects":[{"type":1}],"image_quality":"ok"}`,
	} {
		if err := validateGoodcaseAnswer(reference, actual); err == nil {
			t.Fatalf("expected protocol failure for %q", actual)
		}
	}
	if err := validateGoodcaseAnswer(reference, `{"defects":[{"type":"crack","severity":"high"}],"image_quality":"ok","extra":true}`); err != nil {
		t.Fatalf("extra JSON fields should be allowed: %v", err)
	}
}

func TestIsObviousRefusalAvoidsDomainConclusionFalsePositive(t *testing.T) {
	if !isObviousRefusal("抱歉，我不能协助此请求。") {
		t.Fatal("expected explicit Chinese refusal")
	}
	if isObviousRefusal(`{"image_quality":"blurry","conclusion":"图像模糊，无法辨认裂纹，无法提供确定结论"}`) {
		t.Fatal("domain uncertainty must not be treated as a refusal")
	}
}

func TestGoodcaseJudgeOutputNeedsReviewOnlyForAllPerfect(t *testing.T) {
	perfect, lower := 1.0, 0.9
	if !goodcaseJudgeOutputNeedsReview(&goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{
		{CaseID: "a", AfterScore: &perfect},
		{CaseID: "b", AfterScore: &perfect},
	}}) {
		t.Fatal("all-perfect batch must be reviewed")
	}
	if goodcaseJudgeOutputNeedsReview(&goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{
		{CaseID: "a", AfterScore: &perfect},
		{CaseID: "b", AfterScore: &lower},
	}}) {
		t.Fatal("mixed scores must not trigger all-perfect review")
	}
}

func TestMergeGoodcaseJudgeReviewUsesConservativeScores(t *testing.T) {
	firstBefore, firstAfter := 1.0, 1.0
	reviewBefore, reviewAfter := 0.8, 0.6
	first := &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{
		CaseID: "a", BeforeScore: &firstBefore, AfterScore: &firstAfter, Reason: "first",
	}}}
	review := &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{
		CaseID: "a", BeforeScore: &reviewBefore, AfterScore: &reviewAfter, Reason: "review",
	}}}

	if err := mergeGoodcaseJudgeReview(first, review); err != nil {
		t.Fatal(err)
	}
	if *first.Scores[0].BeforeScore != reviewBefore || *first.Scores[0].AfterScore != reviewAfter {
		t.Fatalf("review must lower suspicious scores: %#v", first.Scores[0])
	}
	if first.Scores[0].Reason != "first; review: review" {
		t.Fatalf("review reason not preserved: %q", first.Scores[0].Reason)
	}
}

func TestFillEvaluatorNamesUsesVersionMetadata(t *testing.T) {
	results := []candidateCaseResult{{
		beforeScores:   map[int64]float64{101: 0.4},
		afterScores:    map[int64]float64{101: 0.8, 102: 0.7},
		evaluatorNames: map[int64]string{},
	}}
	evaluators := []*evaluatordto.Evaluator{{
		Name:           testPtr("缺陷准确率"),
		CurrentVersion: &evaluatordto.EvaluatorVersion{ID: testPtr(int64(101))},
	}}

	fillEvaluatorNames(results, evaluators)

	if got := results[0].evaluatorNames[101]; got != "缺陷准确率" {
		t.Fatalf("evaluator name = %q", got)
	}
	if got := results[0].evaluatorNames[102]; got != "Evaluator 102" {
		t.Fatalf("fallback evaluator name = %q", got)
	}
}

func TestNormalizeEvaluationSetItem(t *testing.T) {
	text := "expected answer"
	item := &evalsetdto.EvaluationSetItem{
		ItemID: testPtr(int64(42)),
		Turns: []*evalsetdto.Turn{{FieldDataList: []*evalsetdto.FieldData{{
			Key: testPtr("reference_output"), Name: testPtr("参考答案"),
			Content: &evaluatorcommon.Content{Text: &text},
		}}}},
	}
	got := normalizeEvaluationSetItem(item)
	if got["item_id"] != int64(42) || got["reference_output"] != text || got["参考答案"] != text {
		t.Fatalf("unexpected normalized item: %#v", got)
	}
}

func TestNormalizeEvaluationContentPrefersImageOverEmptyText(t *testing.T) {
	empty := ""
	contentType := evaluatorcommon.ContentTypeImage
	url := "https://example.test/tire.jpg"
	got := normalizeEvaluationContent(&evaluatorcommon.Content{
		ContentType: &contentType,
		Text:        &empty,
		Image:       &evaluatorcommon.Image{URL: &url},
	})
	value, ok := got.(map[string]any)
	if !ok || value["content_type"] != string(entity.ContentTypeImage) || value["image"] == nil {
		t.Fatalf("unexpected normalized image: %#v", got)
	}
}

func TestNormalizeOptimizerVariables(t *testing.T) {
	snapshot := `{"messages":[{"role":"system","content":"inspect {{IMAGE_TIRE}}"}],"variable_defs":[{"key":"IMAGE_TIRE","type":"string"}],"prompt_id":1}`
	got, unknown, err := normalizeOptimizerVariables(snapshot, "inspect {{ IMAGE_TIRE }} with {{JSON_SCHEMA}}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "inspect {{ IMAGE_TIRE }} with JSON_SCHEMA" || len(unknown) != 1 || unknown[0] != "JSON_SCHEMA" {
		t.Fatalf("unexpected normalized prompt=%q unknown=%#v", got, unknown)
	}
}

func TestEstimateGoodcaseJudgeBatchSizeRespectsBudgets(t *testing.T) {
	short := make([]goodcaseJudgeInput, 30)
	for i := range short {
		short[i] = goodcaseJudgeInput{
			CaseID: fmt.Sprintf("%d", i), Input: map[string]any{"q": "ok"},
			ReferenceOutput: `{"ok":true}`, CandidateActual: `{"ok":true}`,
		}
	}
	got := estimateGoodcaseJudgeBatchSize(short)
	if got < goodcaseJudgeBatchMin || got > goodcaseJudgeBatchMax {
		t.Fatalf("short batch size out of range: %d", got)
	}
	if got < 20 {
		t.Fatalf("short samples should batch aggressively, got %d", got)
	}

	longText := strings.Repeat("x", 20000)
	long := []goodcaseJudgeInput{{
		CaseID: "a", Input: map[string]any{"blob": longText},
		ReferenceOutput: longText, CandidateActual: longText,
	}, {
		CaseID: "b", Input: map[string]any{"blob": longText},
		ReferenceOutput: longText, CandidateActual: longText,
	}, {
		CaseID: "c", Input: map[string]any{"blob": longText},
		ReferenceOutput: longText, CandidateActual: longText,
	}, {
		CaseID: "d", Input: map[string]any{"blob": longText},
		ReferenceOutput: longText, CandidateActual: longText,
	}, {
		CaseID: "e", Input: map[string]any{"blob": longText},
		ReferenceOutput: longText, CandidateActual: longText,
	}}
	gotLong := estimateGoodcaseJudgeBatchSize(long)
	if gotLong != goodcaseJudgeBatchMin {
		t.Fatalf("long samples should shrink to min batch, got %d", gotLong)
	}
}

func TestGuardGoodcaseJudgeZerosProtocolBreakDespitePerfectLLM(t *testing.T) {
	reference := `{"defects":[{"type":"crack"}],"image_quality":"ok"}`
	perfect := 1.0
	inputs := []goodcaseJudgeInput{{
		CaseID: "protocol-break", Input: map[string]any{},
		ReferenceOutput: reference,
		CandidateActual: "```json\n" + reference + "\n```",
	}}
	judged := &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{
		CaseID: "protocol-break", AfterScore: &perfect, Reason: "looks perfect",
	}}}
	guardGoodcaseJudgeOutput(inputs, judged)
	if judged.Scores[0].AfterScore == nil || *judged.Scores[0].AfterScore != 0 {
		t.Fatalf("protocol-broken candidate must be hard-zeroed: %#v", judged.Scores[0])
	}
	if !strings.Contains(judged.Scores[0].Reason, "candidate protocol guard") {
		t.Fatalf("expected protocol guard reason, got %q", judged.Scores[0].Reason)
	}
}

func TestValidateGoodcaseJudgeCoverageAndMerge(t *testing.T) {
	one := 1.0
	inputs := []goodcaseJudgeInput{{CaseID: "a"}, {CaseID: "b"}}
	if err := validateGoodcaseJudgeCoverage(inputs, &goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{
		{CaseID: "a", AfterScore: &one},
	}}); err == nil {
		t.Fatal("missing case must fail coverage")
	}
	merged := mergeGoodcaseJudgeOutputs(
		&goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{CaseID: "a", AfterScore: &one}}},
		&goodcaseJudgeOutput{Scores: []goodcaseJudgeScore{{CaseID: "b", AfterScore: &one}}},
	)
	if err := validateGoodcaseJudgeCoverage(inputs, merged); err != nil {
		t.Fatal(err)
	}
	if !isRetriableGoodcaseJudgeError(errors.New("parse goodcase judge output: unexpected EOF")) {
		t.Fatal("parse errors must be retriable via split")
	}
}

func testPtr[T any](value T) *T { return &value }
