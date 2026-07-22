// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"strings"
	"testing"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

func TestBuildCandidateSolutionPlansRoutesProtocol(t *testing.T) {
	probes := []ProbeResult{{
		CaseID: "1", ProbeType: probeTypeOutputProtocol, Status: probeStatusFail, FailureStage: probeStageProtocol,
	}}
	plans := buildCandidateSolutionPlans(probes, 3)
	if plans[0].TextStrategy != textStrategyProtocol {
		t.Fatalf("expected protocol-first plan, got %#v", plans[0])
	}
	if plans[0].Visual.Kind != visualNone {
		t.Fatalf("protocol plan should not change visual transform first: %#v", plans[0])
	}
}

func TestBuildCandidateSolutionPlansPerceptionWhenClean(t *testing.T) {
	plans := buildCandidateSolutionPlans(nil, 2)
	if plans[0].TextStrategy != textStrategyPerception || plans[0].Visual.Kind != visualSpatialScan {
		t.Fatalf("expected perception+spatial_scan, got %#v", plans[0])
	}
}

func TestRefineSolutionPlansUsesLowScores(t *testing.T) {
	cases := []candidateCaseResult{
		{caseID: "a", afterScores: map[int64]float64{1: 0.2}},
		{caseID: "b", afterScores: map[int64]float64{1: 0.3}},
	}
	plans := refineSolutionPlans(cases, nil, 2)
	foundPerception := false
	for _, plan := range plans {
		if plan.TextStrategy == textStrategyPerception || plan.Visual.Kind == visualSpatialScan {
			foundPerception = true
		}
	}
	if !foundPerception {
		t.Fatalf("expected perception channel after low scores, got %#v", plans)
	}
}

func TestApplyVisualTransformToMessages(t *testing.T) {
	user := "answer the question"
	messages := []*entity.Message{{
		Role:    entity.RoleUser,
		Content: &entity.Content{Text: &user},
	}}
	got := applyVisualTransformToMessages(messages, VisualTransformPlan{Kind: visualPerceiveThenReason})
	if got[0].Content == nil || !strings.Contains(*got[0].Content.Text, "感知后推理") {
		t.Fatalf("expected overlay on user message, got %#v", got[0])
	}
	if user != "answer the question" {
		t.Fatal("original message text must stay immutable")
	}
}

func TestOptimizerSystemPromptStrategyFocus(t *testing.T) {
	prompt := optimizerSystemPrompt(SolutionPlan{TextStrategy: textStrategyProtocol, Visual: VisualTransformPlan{Kind: visualNone}})
	if !strings.Contains(prompt, "输出协议") {
		t.Fatalf("protocol focus missing: %s", prompt)
	}
}
