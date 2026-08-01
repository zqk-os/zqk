package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

func (c *OpenAIClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	if c.shouldMock() {
		return StructuredCompletionResponse{}, errfmt.Errorf("APIKey missing, cannot generate structured completion")
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

	// Map our messages to OpenAI format
	openAIMessages := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		msg := map[string]any{
			objects.FieldKeyRole: m.Role,
		}
		if m.Content != "" {
			msg[objects.FieldKeyContent] = m.Content
		}
		if m.Name != "" {
			msg[objects.FieldKeyName] = m.Name
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			openAIToolCalls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				openAIToolCalls = append(openAIToolCalls, map[string]any{
					objects.FieldKeyID:   tc.ID,
					objects.FieldKeyType: "function",
					"function": map[string]any{
						objects.FieldKeyName: tc.Name,
						"arguments":          tc.Arguments,
					},
				})
			}
			msg["tool_calls"] = openAIToolCalls
		}
		openAIMessages = append(openAIMessages, msg)
	}

	payload := map[string]any{
		"model":      c.config.ChatModel,
		"messages":   openAIMessages,
		"max_tokens": 4096,
	}

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	if len(tools) > 0 {
		openAITools := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			openAITools = append(openAITools, map[string]any{
				objects.FieldKeyType: "function",
				"function": map[string]any{
					objects.FieldKeyName:        t.Name,
					objects.FieldKeyDescription: t.Description,
					objects.FieldKeyParameters:  t.Parameters,
				},
			})
		}
		payload["tools"] = openAITools
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return StructuredCompletionResponse{}, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return StructuredCompletionResponse{}, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return StructuredCompletionResponse{}, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return StructuredCompletionResponse{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StructuredCompletionResponse{}, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return StructuredCompletionResponse{}, errfmt.Errorf("no response choices returned")
	}

	msg := result.Choices[0].Message
	response := StructuredCompletionResponse{
		Content: msg.Content,
	}

	for _, tc := range msg.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return response, nil
}
