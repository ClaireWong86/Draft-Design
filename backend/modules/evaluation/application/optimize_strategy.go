// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"fmt"
	"strings"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

// Text rewrite strategies routed from probe failure stages.
// See docs/prompt-loop/vlm-prompt-optimization-research.md §3.
const (
	textStrategyGeneral     = "general"
	textStrategyProtocol    = "protocol"
	textStrategyReasoning   = "reasoning"
	textStrategyPerception  = "perception"
	textStrategyInput       = "input"
)

const (
	visualNone               = "none"
	visualSpatialScan        = "spatial_scan"
	visualPerceiveThenReason = "perceive_then_reason"
	visualRegionFocusHint    = "region_focus_hint"
)

// VisualTransformPlan describes a reproducible visual guidance change.
// MVP applies prompt-side instructions only (no derived media storage yet).
type VisualTransformPlan struct {
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params,omitempty"`
}

// SolutionPlan is one searchable candidate unit: text strategy + visual transform.
type SolutionPlan struct {
	TextStrategy string              `json:"text_strategy"`
	Visual       VisualTransformPlan `json:"visual"`
	Rationale    string              `json:"rationale,omitempty"`
}

func (p SolutionPlan) summary() string {
	return fmt.Sprintf("solution_plan:text=%s;visual=%s;%s", p.TextStrategy, p.Visual.Kind, strings.TrimSpace(p.Rationale))
}

func dominantFailureStages(probes []ProbeResult) map[string]int {
	counts := map[string]int{}
	for _, probe := range probes {
		if probe.Status == probeStatusPass || probe.FailureStage == "" {
			continue
		}
		counts[probe.FailureStage]++
	}
	return counts
}

// buildCandidateSolutionPlans creates a bounded search space of SolutionPlans.
// Candidates diversify across failure-stage-driven text strategies and visual transforms.
func buildCandidateSolutionPlans(probes []ProbeResult, n int) []SolutionPlan {
	if n <= 0 {
		n = 1
	}
	stages := dominantFailureStages(probes)
	templates := make([]SolutionPlan, 0, 6)

	add := func(strategy, visual, rationale string) {
		templates = append(templates, SolutionPlan{
			TextStrategy: strategy,
			Visual:       VisualTransformPlan{Kind: visual},
			Rationale:    rationale,
		})
	}

	if stages[probeStageProtocol] > 0 {
		add(textStrategyProtocol, visualNone, "protocol probe failures dominate")
	}
	if stages[probeStageInput] > 0 {
		add(textStrategyInput, visualNone, "input completeness failures dominate; prefer adapter/mapping fixes over rewrite")
	}
	if stages[probeStageProtocol] == 0 && stages[probeStageInput] == 0 {
		add(textStrategyPerception, visualSpatialScan, "no hard protocol/input failure; try perception + spatial scan")
		add(textStrategyReasoning, visualPerceiveThenReason, "decouple perception then reasoning")
		add(textStrategyPerception, visualRegionFocusHint, "focus on critical visual regions before concluding")
	} else {
		add(textStrategyReasoning, visualPerceiveThenReason, "secondary: decouple perception and reasoning")
		add(textStrategyPerception, visualSpatialScan, "secondary: spatial scan for missed visual evidence")
	}
	add(textStrategyGeneral, visualNone, "general rewrite baseline")

	plans := make([]SolutionPlan, 0, n)
	for i := 0; i < n; i++ {
		plans = append(plans, templates[i%len(templates)])
	}
	return plans
}

// refineSolutionPlans re-ranks search space using low-scoring cases after a round.
func refineSolutionPlans(cases []candidateCaseResult, previous []ProbeResult, n int) []SolutionPlan {
	lowScoreHints := 0
	for _, item := range cases {
		if after, ok := averageScores(item.afterScores); ok && after < 0.6 {
			lowScoreHints++
		}
	}
	probes := append([]ProbeResult(nil), previous...)
	if lowScoreHints > 0 {
		probes = append(probes, ProbeResult{
			CaseID:       "*",
			ProbeType:    "score_heuristic",
			Status:       probeStatusFail,
			FailureStage: probeStageReasoning,
			Evidence:     []string{fmt.Sprintf("%d cases scored below 0.6", lowScoreHints)},
			Confidence:   0.5,
		})
		if lowScoreHints >= len(cases)/2 && len(cases) > 0 {
			probes = append(probes, ProbeResult{
				CaseID:       "*",
				ProbeType:    "score_heuristic",
				Status:       probeStatusFail,
				FailureStage: probeStagePerception,
				Evidence:     []string{"majority of cases remain weak; prefer perception channel"},
				Confidence:   0.55,
			})
		}
	}
	return buildCandidateSolutionPlans(probes, n)
}

func optimizerSystemPrompt(plan SolutionPlan) string {
	base := "你是多模态 Prompt 优化器。保持变量名、输出协议和多模态占位符不变。"
	focus := ""
	switch plan.TextStrategy {
	case textStrategyProtocol:
		focus = "本轮只强化输出协议：JSON/Schema、枚举、必填字段、拒答与空输出处理；不要改任务目标或视觉扫描方式。"
	case textStrategyReasoning:
		focus = "本轮只强化推理：先列出可观察证据再下结论，补充判别规则与反例；不要改输出 Schema。"
	case textStrategyPerception:
		focus = "本轮只强化感知：要求按空间顺序扫描、引用可见区域与属性，避免语言先验压过视觉证据。"
	case textStrategyInput:
		focus = "检测到输入完整性风险。优先给出不依赖缺失媒体的稳健指令，并在 failure_modes 标明应修复映射/适配器，而不是用文案掩盖缺图。"
	default:
		focus = "给出更清晰、可执行、感知与推理解耦的指令。"
	}
	visual := ""
	switch plan.Visual.Kind {
	case visualSpatialScan:
		visual = "请在优化指令中加入从左到右、从上到下的空间扫描步骤。"
	case visualPerceiveThenReason:
		visual = "请在优化指令中强制两阶段：先描述可见证据，再仅基于描述判断。"
	case visualRegionFocusHint:
		visual = "请在优化指令中要求先定位关键区域（方位/相对位置）再放大观察该区域细节。"
	}
	return base + focus + visual + "只返回 JSON：{\"optimized_prompt\":\"...\",\"failure_modes\":[\"...\"],\"suggested_instruction_changes\":[\"...\"]}。"
}

func visualInstructionOverlay(plan VisualTransformPlan) string {
	switch plan.Kind {
	case visualSpatialScan:
		return "【视觉变换·空间扫描】先从左到右、从上到下扫描整图，再对每个可见关键区域做简要描述，最后才下结论。"
	case visualPerceiveThenReason:
		return "【视觉变换·感知后推理】第一步只描述可观察证据（形状、纹理、位置、异常），第二步仅根据上述描述作答，禁止跳过证据直接下结论。"
	case visualRegionFocusHint:
		return "【视觉变换·区域聚焦】先指出最可疑区域的方位与边界，再仅基于该区域细节给出最终判断。"
	default:
		return ""
	}
}

func applyVisualTransformToMessages(messages []*entity.Message, plan VisualTransformPlan) []*entity.Message {
	overlay := strings.TrimSpace(visualInstructionOverlay(plan))
	if overlay == "" || len(messages) == 0 {
		return messages
	}
	out := append([]*entity.Message(nil), messages...)
	for i := len(out) - 1; i >= 0; i-- {
		msg := out[i]
		if msg == nil || msg.Role != entity.RoleUser {
			continue
		}
		cloned := *msg
		if cloned.Content == nil {
			cloned.Content = &entity.Content{}
		}
		content := *cloned.Content
		text := ""
		if content.Text != nil {
			text = *content.Text
		}
		merged := overlay + "\n" + text
		content.Text = &merged
		cloned.Content = &content
		out[i] = &cloned
		return out
	}
	system := overlay
	return append([]*entity.Message{{
		Role:    entity.RoleSystem,
		Content: &entity.Content{Text: &system},
	}}, out...)
}

func attachSolutionPlan(out *optimizerOutput, plan SolutionPlan) {
	if out == nil {
		return
	}
	copied := plan
	out.SolutionPlan = &copied
	out.SuggestedChanges = appendUniqueStrings(out.SuggestedChanges, plan.summary())
	if plan.TextStrategy == textStrategyInput {
		out.FailureModes = appendUniqueStrings(out.FailureModes, "strategy:input: prefer fixing media adapters/mapping before prompt rewrite")
	}
}
