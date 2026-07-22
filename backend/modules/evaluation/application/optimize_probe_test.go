// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"strings"
	"testing"
)

func TestProbeInputCompletenessDetectsMissingImage(t *testing.T) {
	input := map[string]any{
		"image": map[string]any{
			"content_type": "Image",
			"image":        map[string]any{},
		},
	}
	probes := runDeterministicProbes("case-1", input, `{"ok":true}`, `{"ok":true}`)
	found := false
	for _, probe := range probes {
		if probe.ProbeType == probeTypeInputCompleteness && probe.Status == probeStatusFail {
			found = true
			if !strings.Contains(strings.Join(probe.Evidence, " "), "image") {
				t.Fatalf("expected image evidence, got %#v", probe.Evidence)
			}
		}
	}
	if !found {
		t.Fatalf("expected input completeness failure, got %#v", probes)
	}
}

func TestProbeOutputProtocolDetectsBrokenJSON(t *testing.T) {
	probes := runDeterministicProbes("case-2", map[string]any{"text": "ok"}, `{"label":"A"}`, "not-json")
	found := false
	for _, probe := range probes {
		if probe.ProbeType == probeTypeOutputProtocol && probe.Status == probeStatusFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected protocol failure, got %#v", probes)
	}
}

func TestMergeProbeDiagnosisDedupes(t *testing.T) {
	out := &optimizerOutput{FailureModes: []string{"existing"}}
	probes := []ProbeResult{
		{CaseID: "1", ProbeType: probeTypeOutputProtocol, Status: probeStatusFail, Evidence: []string{"expected structured JSON output"}, RecommendedActions: []string{"strengthen structured output constraints, enums, and refusal handling"}},
		{CaseID: "1", ProbeType: probeTypeOutputProtocol, Status: probeStatusFail, Evidence: []string{"expected structured JSON output"}, RecommendedActions: []string{"strengthen structured output constraints, enums, and refusal handling"}},
	}
	mergeProbeDiagnosis(out, probes)
	if len(out.FailureModes) != 2 {
		t.Fatalf("expected one new failure mode, got %#v", out.FailureModes)
	}
	if len(out.SuggestedChanges) != 1 {
		t.Fatalf("expected one suggested change, got %#v", out.SuggestedChanges)
	}
}

func TestWakeOptimizeTaskEnqueues(t *testing.T) {
	app := &OptimizeApplication{queue: make(chan int64, 1)}
	RegisterOptimizeTaskWaker(app)
	t.Cleanup(func() { RegisterOptimizeTaskWaker(nil) })
	WakeOptimizeTask(42)
	select {
	case id := <-app.queue:
		if id != 42 {
			t.Fatalf("got %d", id)
		}
	default:
		t.Fatal("expected wake to enqueue")
	}
}
