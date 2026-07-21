// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"
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
