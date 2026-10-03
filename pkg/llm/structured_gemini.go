package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func removeAdditionalProperties(val any) any {
	switch v := val.(type) {
	case map[string]any:
		newMap := make(map[string]any)
		for key, value := range v {
			if key == "additionalProperties" {
				continue
			}
			newMap[key] = removeAdditionalProperties(value)
		}
		return newMap
	case []any:
		var newSlice []any
		for _, item := range v {
			newSlice = append(newSlice, removeAdditionalProperties(item))
		}
		return newSlice
	default:
		return v
	}
}

func (c *GeminiClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	return c.GenerateStructuredCompletionWithOptions(ctx, messages, tools, StructuredCompletionOptions{})
}

func (c *GeminiClient) GenerateStructuredCompletionWithOptions(ctx context.Context, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error) {
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

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimSuffix(c.config.BaseURL, "/"), c.config.ChatModel)

	// Map tools to Gemini format
	var geminiTools []map[string]any
	if len(tools) > 0 {
		var funcDecls []map[string]any
		for _, t := range tools {
			params := t.Parameters
			if params != nil {
				params = removeAdditionalProperties(params).(map[string]any)
			}
			funcDecls = append(funcDecls, map[string]any{
				objects.FieldKeyName:        t.Name,
				objects.FieldKeyDescription: t.Description,
				objects.FieldKeyParameters:  params,
			})
		}
		geminiTools = append(geminiTools, map[string]any{
			"functionDeclarations": funcDecls,
		})
	}

	// Map messages to Gemini format
	var geminiContents []map[string]any
	var systemInstruction *map[string]any

	for _, m := range messages {
		if m.Role == "system" {
			systemInstruction = &map[string]any{
				"parts": []map[string]any{
					{"text": m.Content},
				},
			}
			continue
		}

		role := "user"
		if m.Role == "assistant" {
			role = "model"
		} else if m.Role == "tool" || m.Role == "function" {
			role = "user" // Gemini expects function responses from 'user' or 'function'
		}

		var parts []map[string]any

		if m.Content != "" {
			parts = append(parts, map[string]any{
				"text": m.Content,
			})
		}

		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					args = map[string]any{}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						objects.FieldKeyName: tc.Name,
						"args":               args,
					},
				})
			}
		}

		if m.Role == "tool" || m.Role == "function" {
			var resp map[string]any
			if err := json.Unmarshal([]byte(m.Content), &resp); err != nil {
				resp = map[string]any{"result": m.Content}
			}
			parts = append(parts, map[string]any{
				"functionResponse": map[string]any{
					objects.FieldKeyName: m.Name,
					"response":           resp,
				},
			})
		}

		geminiContents = append(geminiContents, map[string]any{
			objects.FieldKeyRole: role,
			"parts":              parts,
		})
	}

	payload := map[string]any{
		"contents": geminiContents,
	}

	if systemInstruction != nil {
		payload["systemInstruction"] = *systemInstruction
	}

	if len(geminiTools) > 0 {
		payload["tools"] = geminiTools
		if cfg := geminiFunctionCallingConfig(opts); cfg != nil {
			payload["toolConfig"] = map[string]any{
				"functionCallingConfig": cfg,
			}
		}
	}

	req, err := createJSONPostRequest(ctx, url, payload)
	if err != nil {
		return StructuredCompletionResponse{}, err
	}
	if c.config.APIKey != "" {
		req.Header.Set("x-goog-api-key", c.config.APIKey)
	}

	resp, err := c.executeStructuredRequest(req)
	if err != nil {
		return StructuredCompletionResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return StructuredCompletionResponse{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text,omitempty"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StructuredCompletionResponse{}, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return StructuredCompletionResponse{}, errfmt.Errorf("no response candidates returned")
	}

	var response StructuredCompletionResponse
	parts := result.Candidates[0].Content.Parts

	for _, p := range parts {
		if p.Text != "" {
			response.Content += p.Text
		}
		if p.FunctionCall != nil {
			argsBytes, _ := json.Marshal(p.FunctionCall.Args)
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				Name:      p.FunctionCall.Name,
				Arguments: string(argsBytes),
			})
		}
	}

	if response.Content == "" && len(response.ToolCalls) == 0 {
		rawResp, _ := json.Marshal(result)
		logging.FluentEvent(logging.GetLogger()).Error("Gemini returned empty content and empty tool calls!", errfmt.Errorf("Raw response: %s", string(rawResp))).Log()
	}

	return response, nil
}

func geminiFunctionCallingMode(choice string) string {
	switch strings.TrimSpace(choice) {
	case ToolChoiceRequired:
		return "ANY"
	case ToolChoiceNone:
		return "NONE"
	default:
		return ""
	}
}

func geminiFunctionCallingConfig(opts StructuredCompletionOptions) map[string]any {
	fn := strings.TrimSpace(opts.ToolChoiceFunction)
	mode := geminiFunctionCallingMode(opts.ToolChoice)
	if fn != "" && mode == "" {
		mode = "ANY"
	}
	if mode == "" {
		return nil
	}
	cfg := map[string]any{"mode": mode}
	if fn != "" {
		cfg["allowedFunctionNames"] = []string{fn}
	}
	return cfg
}

func (c *GeminiClient) executeStructuredRequest(req *http.Request) (*http.Response, error) {
	return executeHTTPRequest(c.httpClient, req)
}
