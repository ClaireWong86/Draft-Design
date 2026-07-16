// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package eino

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"

	"github.com/coze-dev/coze-loop/backend/modules/llm/domain/entity"
)

type joyBuildChatModel struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type joyBuildRequest struct {
	Model             string                    `json:"model"`
	Contents          []joyBuildContent         `json:"contents"`
	SystemInstruction *joyBuildSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *joyBuildGenerationConfig `json:"generationConfig,omitempty"`
}

type joyBuildSystemInstruction struct {
	Parts []joyBuildPart `json:"parts"`
}

type joyBuildContent struct {
	Role  string         `json:"role"`
	Parts []joyBuildPart `json:"parts"`
}

type joyBuildPart struct {
	Text       string              `json:"text,omitempty"`
	InlineData *joyBuildInlineData `json:"inlineData,omitempty"`
	FileData   *joyBuildFileData   `json:"fileData,omitempty"`
}

type joyBuildInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type joyBuildFileData struct {
	MIMEType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type joyBuildGenerationConfig struct {
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	Temperature     *float32 `json:"temperature,omitempty"`
	TopP            *float32 `json:"topP,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
	ResponseMIMEType string   `json:"responseMimeType,omitempty"`
}

type joyBuildErrorResponse struct {
	Error any `json:"error"`
}

type oneShotStreamReader struct {
	msg  *entity.Message
	done bool
}

func newJoyBuildChatModel(model *entity.Model) (*joyBuildChatModel, error) {
	if err := checkModelBeforeBuild(model); err != nil {
		return nil, err
	}
	p := model.ProtocolConfig
	if p.BaseURL == "" {
		return nil, errors.New("joybuild base_url is empty")
	}
	if p.APIKey == "" {
		return nil, errors.New("joybuild api_key is empty")
	}
	if p.Model == "" {
		return nil, errors.New("joybuild model is empty")
	}
	timeout := 180 * time.Second
	if p.TimeoutMs != nil && *p.TimeoutMs > 0 {
		timeout = time.Duration(*p.TimeoutMs) * time.Millisecond
	}
	return &joyBuildChatModel{
		baseURL: strings.TrimRight(p.BaseURL, "/"),
		apiKey:  p.APIKey,
		model:   p.Model,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (m *joyBuildChatModel) Generate(ctx context.Context, input []*entity.Message, opts ...entity.Option) (*entity.Message, error) {
	reqBody, err := m.buildRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	payload, err := sonic.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.responsesURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Trace-Id", "coze-loop-joybuild")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("joybuild request failed: status=%d body=%s", resp.StatusCode, truncateForError(body))
	}
	content, finishReason, err := parseJoyBuildContent(body)
	if err != nil {
		return nil, err
	}
	return &entity.Message{
		Role:    entity.RoleAssistant,
		Content: content,
		ResponseMeta: &entity.ResponseMeta{
			FinishReason: finishReason,
		},
	}, nil
}

func (m *joyBuildChatModel) Stream(ctx context.Context, input []*entity.Message, opts ...entity.Option) (entity.IStreamReader, error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return &oneShotStreamReader{msg: msg}, nil
}

func (m *joyBuildChatModel) responsesURL() string {
	if strings.HasSuffix(m.baseURL, "/v1") {
		return m.baseURL + "/responses"
	}
	return m.baseURL + "/v1/responses"
}

func (m *joyBuildChatModel) buildRequest(input []*entity.Message, opts ...entity.Option) (*joyBuildRequest, error) {
	options := entity.ApplyOptions(nil, opts...)
	req := &joyBuildRequest{
		Model:            m.model,
		Contents:         make([]joyBuildContent, 0, len(input)),
		GenerationConfig: buildJoyBuildGenerationConfig(options),
	}
	for _, msg := range input {
		if msg == nil {
			continue
		}
		parts, err := joyBuildPartsFromMessage(msg)
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		if msg.Role == entity.RoleSystem {
			if req.SystemInstruction == nil {
				req.SystemInstruction = &joyBuildSystemInstruction{}
			}
			req.SystemInstruction.Parts = append(req.SystemInstruction.Parts, parts...)
			continue
		}
		req.Contents = append(req.Contents, joyBuildContent{
			Role:  joyBuildRole(msg.Role),
			Parts: parts,
		})
	}
	if len(req.Contents) == 0 && req.SystemInstruction != nil {
		req.Contents = append(req.Contents, joyBuildContent{
			Role: "user",
			Parts: []joyBuildPart{{
				Text: "Please follow the system instruction.",
			}},
		})
	}
	if len(req.Contents) == 0 {
		return nil, errors.New("joybuild request has no contents")
	}
	return req, nil
}

func buildJoyBuildGenerationConfig(options *entity.Options) *joyBuildGenerationConfig {
	if options == nil {
		return nil
	}
	cfg := &joyBuildGenerationConfig{
		MaxOutputTokens: options.MaxTokens,
		Temperature:     options.Temperature,
		TopP:            options.TopP,
		StopSequences:   options.Stop,
	}
	if options.ResponseFormat != nil && options.ResponseFormat.Type == entity.ResponseFormatTypeJSON {
		cfg.ResponseMIMEType = "application/json"
	}
	if cfg.MaxOutputTokens == nil && cfg.Temperature == nil && cfg.TopP == nil && len(cfg.StopSequences) == 0 && cfg.ResponseMIMEType == "" {
		return nil
	}
	return cfg
}

func joyBuildPartsFromMessage(msg *entity.Message) ([]joyBuildPart, error) {
	if len(msg.MultiModalContent) == 0 {
		if msg.Content == "" {
			return nil, nil
		}
		return []joyBuildPart{{Text: msg.Content}}, nil
	}
	parts := make([]joyBuildPart, 0, len(msg.MultiModalContent))
	for _, part := range msg.MultiModalContent {
		if part == nil {
			continue
		}
		switch part.Type {
		case entity.ChatMessagePartTypeText:
			if part.Text != "" {
				parts = append(parts, joyBuildPart{Text: part.Text})
			}
		case entity.ChatMessagePartTypeImageURL:
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}
			if part.ImageURL.MIMEType != "" {
				parts = append(parts, joyBuildPart{
					InlineData: &joyBuildInlineData{
						MIMEType: part.ImageURL.MIMEType,
						Data:     stripDataURLPrefix(part.ImageURL.URL),
					},
				})
			} else {
				parts = append(parts, joyBuildPart{
					FileData: &joyBuildFileData{
						FileURI: part.ImageURL.URL,
					},
				})
			}
		default:
			return nil, fmt.Errorf("joybuild unsupported message part type: %s", part.Type)
		}
	}
	return parts, nil
}

func joyBuildRole(role entity.Role) string {
	switch role {
	case entity.RoleAssistant:
		return "model"
	case entity.RoleUser:
		return "user"
	case entity.RoleTool:
		return "user"
	default:
		return "user"
	}
}

func stripDataURLPrefix(value string) string {
	if idx := strings.Index(value, ","); idx >= 0 && strings.HasPrefix(value[:idx], "data:") {
		return value[idx+1:]
	}
	return value
}

func parseJoyBuildContent(body []byte) (string, string, error) {
	var errResp joyBuildErrorResponse
	if err := sonic.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return "", "", fmt.Errorf("joybuild error: %v", errResp.Error)
	}
	var raw map[string]any
	if err := sonic.Unmarshal(body, &raw); err != nil {
		return "", "", err
	}
	if text := firstString(raw, "output_text", "text", "content", "result", "response"); text != "" {
		return text, firstString(raw, "finishReason", "finish_reason"), nil
	}
	if text := extractJoyBuildCandidatesText(raw); text != "" {
		return text, firstString(raw, "finishReason", "finish_reason"), nil
	}
	if text := extractJoyBuildOutputText(raw); text != "" {
		return text, firstString(raw, "finishReason", "finish_reason"), nil
	}
	return "", "", fmt.Errorf("joybuild response has no text content: %s", truncateForError(body))
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func extractJoyBuildCandidatesText(raw map[string]any) string {
	candidates, ok := raw["candidates"].([]any)
	if !ok {
		return ""
	}
	var builder strings.Builder
	for _, item := range candidates {
		candidate, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content, ok := candidate["content"].(map[string]any)
		if !ok {
			continue
		}
		appendPartsText(&builder, content["parts"])
	}
	return builder.String()
}

func extractJoyBuildOutputText(raw map[string]any) string {
	var builder strings.Builder
	appendPartsText(&builder, raw["output"])
	return builder.String()
}

func appendPartsText(builder *strings.Builder, value any) {
	items, ok := value.([]any)
	if !ok {
		switch typed := value.(type) {
		case string:
			builder.WriteString(typed)
		case map[string]any:
			appendMapText(builder, typed)
		}
		return
	}
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			builder.WriteString(typed)
		case map[string]any:
			appendMapText(builder, typed)
		}
	}
}

func appendMapText(builder *strings.Builder, value map[string]any) {
	if text, ok := value["text"].(string); ok {
		builder.WriteString(text)
		return
	}
	if content, ok := value["content"]; ok {
		appendPartsText(builder, content)
		return
	}
	if parts, ok := value["parts"]; ok {
		appendPartsText(builder, parts)
	}
}

func truncateForError(body []byte) string {
	const maxLen = 512
	value := string(body)
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func (r *oneShotStreamReader) Recv() (*entity.Message, error) {
	if r.done {
		return nil, io.EOF
	}
	r.done = true
	return r.msg, nil
}
