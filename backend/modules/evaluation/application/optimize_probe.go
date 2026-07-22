// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"fmt"
	"strings"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

const (
	probeTypeInputCompleteness = "input_completeness"
	probeTypeOutputProtocol    = "output_protocol"

	probeStatusPass       = "pass"
	probeStatusFail       = "fail"
	probeStatusUncertain  = "uncertain"
	probeStageInput       = "input"
	probeStageProtocol    = "protocol"
)

// ProbeResult is a deterministic multimodal diagnosis probe outcome.
// See docs/prompt-loop/vlm-prompt-optimization-research.md §2.
type ProbeResult struct {
	CaseID              string   `json:"case_id"`
	ProbeType           string   `json:"probe_type"`
	Status              string   `json:"status"`
	FailureStage        string   `json:"failure_stage,omitempty"`
	Evidence            []string `json:"evidence,omitempty"`
	RecommendedActions  []string `json:"recommended_actions,omitempty"`
	Confidence          float64  `json:"confidence"`
}

// runDeterministicProbes executes low-cost input / protocol probes for one case.
// VLM-backed probes (bare description, localization) are intentionally out of MVP scope.
func runDeterministicProbes(caseID string, input map[string]any, reference, actual string) []ProbeResult {
	results := make([]ProbeResult, 0, 2)
	results = append(results, probeInputCompleteness(caseID, input))
	if strings.TrimSpace(reference) != "" || strings.TrimSpace(actual) != "" {
		results = append(results, probeOutputProtocol(caseID, reference, actual))
	}
	return results
}

func probeInputCompleteness(caseID string, input map[string]any) ProbeResult {
	result := ProbeResult{
		CaseID:       caseID,
		ProbeType:    probeTypeInputCompleteness,
		Status:       probeStatusPass,
		FailureStage: probeStageInput,
		Confidence:   0.9,
	}
	if len(input) == 0 {
		result.Status = probeStatusFail
		result.Evidence = []string{"mapped case input is empty"}
		result.RecommendedActions = []string{"fix field mapping and evaluation-set / experiment payload assembly"}
		return result
	}
	var evidence []string
	walkMappedInput(input, "", func(path string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			contentType, _ := typed["content_type"].(string)
			switch strings.ToLower(contentType) {
			case strings.ToLower(string(entity.ContentTypeImage)), "image":
				if !mediaPayloadPresent(typed["image"]) {
					evidence = append(evidence, fmt.Sprintf("%s: image content missing url/uri", pathOrRoot(path)))
				}
			case strings.ToLower(string(entity.ContentTypeVideo)), "video":
				if !mediaPayloadPresent(typed["video"]) {
					evidence = append(evidence, fmt.Sprintf("%s: video content missing url/uri", pathOrRoot(path)))
				}
			}
		}
	})
	if len(evidence) > 0 {
		result.Status = probeStatusFail
		result.Evidence = evidence
		result.RecommendedActions = []string{"repair media adapters / multipart assembly before rewriting prompt text"}
		return result
	}
	return result
}

func probeOutputProtocol(caseID, reference, actual string) ProbeResult {
	result := ProbeResult{
		CaseID:       caseID,
		ProbeType:    probeTypeOutputProtocol,
		Status:       probeStatusPass,
		FailureStage: probeStageProtocol,
		Confidence:   0.95,
	}
	if err := validateGoodcaseAnswer(reference, actual); err != nil {
		result.Status = probeStatusFail
		result.Evidence = []string{err.Error()}
		result.RecommendedActions = []string{"strengthen structured output constraints, enums, and refusal handling"}
		return result
	}
	if strings.TrimSpace(actual) == "" && strings.TrimSpace(reference) != "" {
		result.Status = probeStatusUncertain
		result.Evidence = []string{"empty actual with non-empty reference"}
		result.RecommendedActions = []string{"inspect target model empty-output rate before prompt rewrite"}
		result.Confidence = 0.6
	}
	return result
}

func collectProbeFailureModes(probes []ProbeResult) ([]string, []string) {
	modes := make([]string, 0)
	actions := make([]string, 0)
	seenMode := make(map[string]struct{})
	seenAction := make(map[string]struct{})
	for _, probe := range probes {
		if probe.Status == probeStatusPass {
			continue
		}
		mode := fmt.Sprintf("probe:%s:%s:case_%s", probe.ProbeType, probe.Status, probe.CaseID)
		if len(probe.Evidence) > 0 {
			mode += ":" + probe.Evidence[0]
		}
		if _, ok := seenMode[mode]; !ok {
			seenMode[mode] = struct{}{}
			modes = append(modes, mode)
		}
		for _, action := range probe.RecommendedActions {
			if _, ok := seenAction[action]; ok {
				continue
			}
			seenAction[action] = struct{}{}
			actions = append(actions, action)
		}
	}
	return modes, actions
}

func mergeProbeDiagnosis(out *optimizerOutput, probes []ProbeResult) {
	if out == nil || len(probes) == 0 {
		return
	}
	modes, actions := collectProbeFailureModes(probes)
	out.FailureModes = appendUniqueStrings(out.FailureModes, modes...)
	out.SuggestedChanges = appendUniqueStrings(out.SuggestedChanges, actions...)
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		dst = append(dst, value)
	}
	return dst
}

func walkMappedInput(value any, path string, visit func(path string, value any)) {
	visit(path, value)
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			walkMappedInput(child, childPath, visit)
		}
	case []any:
		for index, child := range typed {
			childPath := fmt.Sprintf("%s[%d]", pathOrRoot(path), index)
			walkMappedInput(child, childPath, visit)
		}
	}
}

func mediaPayloadPresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]any:
		for _, key := range []string{"url", "uri", "thumb_url", "storage_key", "name"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
		// Nested thrift-like shapes may use URL / URI fields.
		if nested, ok := typed["url"].(map[string]any); ok {
			return mediaPayloadPresent(nested)
		}
		return false
	default:
		return fmt.Sprint(typed) != "" && fmt.Sprint(typed) != "<nil>"
	}
}

func pathOrRoot(path string) string {
	if path == "" {
		return "$"
	}
	return path
}
