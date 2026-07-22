// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	promptdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/prompt/domain/prompt"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

var optimizeVariablePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// renderCandidateMessages clones and renders a prompt snapshot for one case.
// Rendering happens before conversion to runtime messages, so text in both
// ordinary messages and multimodal text parts follows the same semantics.
func renderCandidateMessages(messages []*promptdto.Message, variables map[string]any) ([]*promptdto.Message, error) {
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("clone prompt messages: %w", err)
	}
	var cloned []*promptdto.Message
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("clone prompt messages: %w", err)
	}
	missing := make(map[string]struct{})
	for _, message := range cloned {
		if message == nil || message.GetSkipRender() {
			continue
		}
		if message.Content != nil {
			value := renderTemplateText(*message.Content, variables, missing)
			message.Content = &value
		}
		for _, part := range message.Parts {
			if part == nil || part.Text == nil {
				continue
			}
			value := renderTemplateText(*part.Text, variables, missing)
			part.Text = &value
		}
	}
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		return nil, fmt.Errorf("prompt variables are not mapped: %s", strings.Join(keys, ", "))
	}
	return cloned, nil
}

func renderTemplateText(text string, variables map[string]any, missing map[string]struct{}) string {
	return optimizeVariablePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := optimizeVariablePattern.FindStringSubmatch(match)
		value, ok := variables[parts[1]]
		if !ok {
			missing[parts[1]] = struct{}{}
			return match
		}
		switch typed := value.(type) {
		case string:
			return typed
		case nil:
			return ""
		default:
			if containsMappedMedia(typed) {
				return "[多模态输入见后续用户消息]"
			}
			encoded, err := json.Marshal(typed)
			if err != nil {
				return fmt.Sprint(typed)
			}
			return string(encoded)
		}
	})
}

func containsMappedMedia(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, part := range typed {
			if containsMappedMedia(part) {
				return true
			}
		}
	case map[string]any:
		kind, _ := typed["content_type"].(string)
		return strings.EqualFold(kind, string(entity.ContentTypeImage)) || strings.EqualFold(kind, string(entity.ContentTypeVideo))
	}
	return false
}

// lookupMappedValue accepts explicit JSON paths (a.b.c) and, for backwards
// compatibility, an unambiguous leaf key. It replaces the old top-level-only
// lookup which silently dropped experiment payload fields.
func lookupMappedValue(source any, field string) (any, bool) {
	if field == "" {
		return nil, false
	}
	current := source
	for _, segment := range strings.Split(field, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			current = nil
			break
		}
		current, ok = object[segment]
		if !ok {
			current = nil
			break
		}
	}
	if current != nil {
		return current, true
	}
	var found any
	count := 0
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if key == field {
					found = nested
					count++
				}
				visit(nested)
			}
		case []any:
			for _, nested := range typed {
				visit(nested)
			}
		}
	}
	visit(source)
	return found, count == 1
}
