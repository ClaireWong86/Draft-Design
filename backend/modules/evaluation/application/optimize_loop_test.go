// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"

	"github.com/coze-dev/coze-loop/backend/infra/middleware/session"
	evaluatorcommon "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/common"
	evalsetdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/evaluation/domain/eval_set"
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
	for _, tc := range []struct {
		cases int
		want  int
	}{{0, 0}, {1, 1}, {12, 1}, {13, 2}, {24, 2}, {25, 3}} {
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

func testPtr[T any](value T) *T { return &value }
