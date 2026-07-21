// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package eino

import "testing"

func TestJoyBuildImageStats(t *testing.T) {
	req := &joyBuildRequest{Contents: []joyBuildContent{
		{Parts: []joyBuildPart{
			{Text: "inspect this image"},
			{InlineData: &joyBuildInlineData{MIMEType: "image/jpeg", Data: "YWJj"}},
			{FileData: &joyBuildFileData{FileURI: "https://example.com/image.png"}},
		}},
	}}

	inlineImages, fileImages, inlineBytes := joyBuildImageStats(req)
	if inlineImages != 1 || fileImages != 1 || inlineBytes != 4 {
		t.Fatalf("unexpected image stats: inline=%d file=%d bytes=%d", inlineImages, fileImages, inlineBytes)
	}
}

func TestJoyBuildImageStatsNilRequest(t *testing.T) {
	inlineImages, fileImages, inlineBytes := joyBuildImageStats(nil)
	if inlineImages != 0 || fileImages != 0 || inlineBytes != 0 {
		t.Fatalf("unexpected image stats for nil request: inline=%d file=%d bytes=%d", inlineImages, fileImages, inlineBytes)
	}
}
