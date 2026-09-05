package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

func (c *OpenAIClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	return c.GenerateStructuredCompletionWithOptions(ctx, messages, tools, StructuredCompletionOptions{})
}

func (c *OpenAIClient) GenerateStructuredCompletionWithOptions(ctx context.Context, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error) {
	return c.doStructuredCompletion(ctx, messages, tools, opts, true)
}

func (c *OpenAIClient) doStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions, retryWithoutToolChoice bool) (StructuredCompletionResponse, error) {
	if c.shouldMock() {
		return StructuredCompletionResponse{}, errfmt.Errorf("APIKey missing, cannot generate structured completion")
	}

	for i, m := range messages {
		if m.Role == "user" {
			sanitized, err := SanitizeUntrustedText(m.Content)
			if err != nil {
				return StructuredCompletionResponse{}, errfmt.Newf("sanitizing user prompt").Wrap(err)
			}
			messages[i].Content = sanitized
		}
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

	openAIMessages := openAIChatMessages(messages)

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
	if c.config.Temperature != nil {
		payload["temperature"] = *c.config.Temperature
	}
	if c.config.TopP != nil {
		payload["top_p"] = *c.config.TopP
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
		if encoded := encodeToolChoice(opts); encoded != nil {
			payload["tool_choice"] = encoded
		}
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
		bodyText := string(respBody)
		if retryWithoutToolChoice && toolChoiceUnsupported(resp.StatusCode, bodyText) && strings.TrimSpace(opts.ToolChoice) != "" {
			return c.doStructuredCompletion(ctx, messages, tools, StructuredCompletionOptions{}, false)
		}
		return StructuredCompletionResponse{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, bodyText)
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

	rawBytes, _ := io.ReadAll(resp.Body)
	_ = os.WriteFile("/tmp/zqk-raw-llm-response.json", rawBytes, 0644)

	if err := json.Unmarshal(rawBytes, &result); err != nil {
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

func toolChoiceUnsupported(status int, body string) bool {
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		return false
	}
	b := strings.ToLower(body)
	return strings.Contains(b, "tool_choice") ||
		strings.Contains(b, "unknown field") ||
		strings.Contains(b, "unexpected keyword") ||
		strings.Contains(b, "unrecognized")
}

// encodeToolChoice prefers a pinned function object. Local 7B servers often
// accept the string "required" and then return toolCalls=0; naming the owed
// write tool is the next constraint they honor.
func encodeToolChoice(opts StructuredCompletionOptions) any {
	if fn := strings.TrimSpace(opts.ToolChoiceFunction); fn != "" {
		return map[string]any{
			objects.FieldKeyType: "function",
			"function": map[string]any{
				objects.FieldKeyName: fn,
			},
		}
	}
	if choice := strings.TrimSpace(opts.ToolChoice); choice != "" {
		return choice
	}
	return nil
}

// openAIChatMessages maps kernel messages onto the OpenAI chat schema.
//
// Content is always a JSON string, including "". Recovered markdown tool_calls
// clear assistant Content; omitting the field then serializes as missing/null
// and local servers (vLLM / Ollama) reject with
// "invalid message content type: <nil>", after which ResilientClient used to
// fall through to static_mock and the seat-worker parked for missing writes.
// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001 — remove when: providers accept
// omitted content on assistant+tool_calls and tool-role messages.
func openAIChatMessages(messages []Message) []map[string]any {
	openAIMessages := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		msg := map[string]any{
			objects.FieldKeyRole:    m.Role,
			objects.FieldKeyContent: m.Content,
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
	return openAIMessages
}
