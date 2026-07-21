// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

func TestAppendMappedMultimodalEvidence(t *testing.T) {
	messages := []*entity.Message{{Role: entity.RoleSystem}}
	got, err := appendMappedMultimodalEvidence(messages, map[string]any{
		"IMAGE_TIRE": []any{
			map[string]any{"content_type": "Image", "image": map[string]any{"url": "https://example.test/tire.jpg", "uri": "test/tire.jpg"}},
			"compare every visible region",
		},
		"reference_output": "normal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Role != entity.RoleUser || got[1].Content == nil || len(got[1].Content.MultiPart) != 3 {
		t.Fatalf("unexpected messages: %#v", got)
	}
	image := got[1].Content.MultiPart[1].Image
	if image == nil || image.URL == nil || *image.URL != "https://example.test/tire.jpg" {
		t.Fatalf("unexpected image evidence: %#v", image)
	}
}

func TestAppendMappedMultimodalEvidenceSkipsTextOnly(t *testing.T) {
	messages := []*entity.Message{{Role: entity.RoleSystem}}
	got, err := appendMappedMultimodalEvidence(messages, map[string]any{"query": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected appended message: %#v", got)
	}
}
