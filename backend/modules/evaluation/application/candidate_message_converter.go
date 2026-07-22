// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"encoding/json"
	"fmt"

	promptdto "github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/prompt/domain/prompt"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

// convertCandidateMessages converts the Prompt API representation into the
// evaluation runtime representation without dropping multimodal parts.
func convertCandidateMessages(messages []*promptdto.Message) ([]*entity.Message, error) {
	result := make([]*entity.Message, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			continue
		}
		content, err := convertCandidateContent(message)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", index, err)
		}
		result = append(result, &entity.Message{
			Role:    convertCandidateRole(message.GetRole()),
			Content: content,
			Ext:     message.GetMetadata(),
		})
	}
	return result, nil
}

// appendMappedMultimodalEvidence turns versioned EvaluationSet Content back
// into runtime message parts. This prevents image/video variables from being
// serialized as JSON text when the baseline Prompt uses a text placeholder.
func appendMappedMultimodalEvidence(messages []*entity.Message, variables map[string]any) ([]*entity.Message, error) {
	parts := make([]*entity.Content, 0)
	for key, value := range variables {
		if key == "actual_output" || key == "reference_output" || !containsMappedMedia(value) {
			continue
		}
		converted, err := mappedValueToRuntimeParts(value)
		if err != nil {
			return nil, fmt.Errorf("multimodal variable %s: %w", key, err)
		}
		parts = append(parts, converted...)
	}
	if len(parts) == 0 {
		return messages, nil
	}
	prefix := "请基于以下多模态输入完成任务。"
	parts = append([]*entity.Content{{ContentType: contentType(entity.ContentTypeText), Text: &prefix}}, parts...)
	return append(messages, &entity.Message{
		Role:    entity.RoleUser,
		Content: &entity.Content{ContentType: contentType(entity.ContentTypeMultipart), MultiPart: parts},
	}), nil
}

func mappedValueToRuntimeParts(value any) ([]*entity.Content, error) {
	switch typed := value.(type) {
	case []any:
		parts := make([]*entity.Content, 0, len(typed))
		for _, valuePart := range typed {
			converted, err := mappedValueToRuntimeParts(valuePart)
			if err != nil {
				return nil, err
			}
			parts = append(parts, converted...)
		}
		return parts, nil
	case string:
		return []*entity.Content{{ContentType: contentType(entity.ContentTypeText), Text: &typed}}, nil
	case map[string]any:
		kind, _ := typed["content_type"].(string)
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		switch entity.ContentType(kind) {
		case entity.ContentTypeImage:
			var wrapped struct {
				Image *entity.Image `json:"image"`
			}
			if err := json.Unmarshal(raw, &wrapped); err != nil || wrapped.Image == nil {
				return nil, errorsOr(err, "image content is missing")
			}
			return []*entity.Content{{ContentType: contentType(entity.ContentTypeImage), Image: wrapped.Image}}, nil
		case entity.ContentTypeVideo:
			var wrapped struct {
				Video *entity.Video `json:"video"`
			}
			if err := json.Unmarshal(raw, &wrapped); err != nil || wrapped.Video == nil {
				return nil, errorsOr(err, "video content is missing")
			}
			return []*entity.Content{{ContentType: contentType(entity.ContentTypeVideo), Video: wrapped.Video}}, nil
		}
	}
	return nil, nil
}

func errorsOr(err error, message string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", message)
}

func convertCandidateRole(role promptdto.Role) entity.Role {
	switch role {
	case promptdto.RoleSystem:
		return entity.RoleSystem
	case promptdto.RoleAssistant:
		return entity.RoleAssistant
	case promptdto.RoleTool:
		return entity.RoleTool
	default:
		return entity.RoleUser
	}
}

func convertCandidateContent(message *promptdto.Message) (*entity.Content, error) {
	parts := message.GetParts()
	if len(parts) == 0 {
		text := message.GetContent()
		return &entity.Content{ContentType: contentType(entity.ContentTypeText), Text: &text}, nil
	}
	content := &entity.Content{ContentType: contentType(entity.ContentTypeMultipart)}
	for index, part := range parts {
		converted, err := convertCandidatePart(part)
		if err != nil {
			return nil, fmt.Errorf("part[%d]: %w", index, err)
		}
		content.MultiPart = append(content.MultiPart, converted)
	}
	return content, nil
}

func convertCandidatePart(part *promptdto.ContentPart) (*entity.Content, error) {
	if part == nil {
		return nil, fmt.Errorf("content part is nil")
	}
	switch part.GetType() {
	case promptdto.ContentTypeText:
		text := part.GetText()
		return &entity.Content{ContentType: contentType(entity.ContentTypeText), Text: &text}, nil
	case promptdto.ContentTypeImageURL:
		imageURL := part.GetImageURL()
		if imageURL == nil {
			return nil, fmt.Errorf("image_url is missing")
		}
		return &entity.Content{
			ContentType: contentType(entity.ContentTypeImage),
			Image:       &entity.Image{URL: stringPtr(imageURL.GetURL()), URI: stringPtr(imageURL.GetURI())},
		}, nil
	case promptdto.ContentTypeVideoURL:
		videoURL := part.GetVideoURL()
		if videoURL == nil {
			return nil, fmt.Errorf("video_url is missing")
		}
		return &entity.Content{
			ContentType: contentType(entity.ContentTypeVideo),
			Video:       &entity.Video{URL: stringPtr(videoURL.GetURL()), URI: stringPtr(videoURL.GetURI())},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported content type %q", part.GetType())
	}
}

func contentType(value entity.ContentType) *entity.ContentType { return &value }

func stringPtr(value string) *string {
	return &value
}
