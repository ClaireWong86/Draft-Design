// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	promptdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/prompt/domain/prompt"
)

func TestRenderCandidateMessages(t *testing.T) {
	role := promptdto.RoleUser
	content := "inspect {{ image_name }}: {{payload}}"
	partText := "question={{ question }}"
	typeText := promptdto.ContentTypeText
	messages := []*promptdto.Message{{
		Role: &role, Content: &content,
		Parts: []*promptdto.ContentPart{{Type: &typeText, Text: &partText}},
	}}
	rendered, err := renderCandidateMessages(messages, map[string]any{
		"image_name": "tire", "payload": map[string]any{"ok": true}, "question": "damaged?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rendered[0].GetContent(); got != `inspect tire: {"ok":true}` {
		t.Fatalf("unexpected content: %s", got)
	}
	if got := rendered[0].Parts[0].GetText(); got != "question=damaged?" {
		t.Fatalf("unexpected part: %s", got)
	}
	if messages[0].GetContent() != content {
		t.Fatal("input messages were mutated")
	}
}

func TestRenderCandidateMessagesMissingVariable(t *testing.T) {
	content := "{{missing}}"
	_, err := renderCandidateMessages([]*promptdto.Message{{Content: &content}}, nil)
	if err == nil {
		t.Fatal("expected missing variable error")
	}
}

func TestRenderCandidateMessagesUsesMultimodalMarker(t *testing.T) {
	content := "inspect {{IMAGE_TIRE}}"
	rendered, err := renderCandidateMessages([]*promptdto.Message{{Content: &content}}, map[string]any{
		"IMAGE_TIRE": []any{map[string]any{"content_type": "Image", "image": map[string]any{"url": "https://example.test/tire.jpg"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rendered[0].GetContent(); got != "inspect [多模态输入见后续用户消息]" {
		t.Fatalf("unexpected content: %s", got)
	}
}

func TestLookupMappedValue(t *testing.T) {
	source := map[string]any{"payload": map[string]any{"input": map[string]any{"question": "q"}}}
	if got, ok := lookupMappedValue(source, "payload.input.question"); !ok || got != "q" {
		t.Fatalf("path lookup failed: %#v %v", got, ok)
	}
	if got, ok := lookupMappedValue(source, "question"); !ok || got != "q" {
		t.Fatalf("leaf lookup failed: %#v %v", got, ok)
	}
}
