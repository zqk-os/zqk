package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestGeminiClient_GenerateStructuredCompletion(t *testing.T) {
	// NewGeminiClient prefers ZQK_GEMINI_API_KEY over Config.APIKey — pin both.
	t.Setenv(zqkenv.APIKey().Name(), "ACC-1785920548450214012-68b850c0")
	t.Setenv(zqkenv.GeminiAPIKey().Name(), "ACC-1785920548450214012-68b850c0")

	// Create a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "ACC-1785920548450214012-68b850c0" {
			t.Errorf("Expected x-goog-api-key header to be ACC-1785920548450214012-68b850c0")
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("Failed to parse request JSON: %v", err)
		}

		// Verify tools were passed correctly
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Errorf("Expected tools in request")
		} else {
			toolObj := tools[0].(map[string]any)
			decls, ok := toolObj["functionDeclarations"].([]any)
			if !ok || len(decls) == 0 {
				t.Errorf("Expected functionDeclarations in tools")
			} else {
				decl := decls[0].(map[string]any)
				if decl[objects.FieldKeyName] != "get_weather" {
					t.Errorf("Expected tool name 'get_weather', got %v", decl[objects.FieldKeyName])
				}
			}
		}

		// Mock a response that invokes the tool
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					objects.FieldKeyContent: map[string]any{
						"parts": []map[string]any{
							{
								"functionCall": map[string]any{
									objects.FieldKeyName: "get_weather",
									"args": map[string]any{
										"location": "San Francisco",
									},
								},
							},
						},
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &Config{
		APIKey:    "test_api_key_12345678",
		BaseURL:   ts.URL,
		ChatModel: "gemini-test",
	}

	client := NewGeminiClient(context.Background(), config)

	messages := []Message{
		{Role: "user", Content: "What's the weather in San Francisco?"},
	}
	tools := []ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather in a given location",
			Parameters: map[string]any{
				objects.FieldKeyType: "object",
				"properties": map[string]any{
					"location": map[string]any{
						objects.FieldKeyType: "string",
					},
				},
			},
		},
	}

	res, err := client.GenerateStructuredCompletion(context.Background(), messages, tools)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(res.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(res.ToolCalls))
	}

	if res.ToolCalls[0].Name != "get_weather" {
		t.Errorf("Expected tool call name 'get_weather', got %s", res.ToolCalls[0].Name)
	}

	if res.ToolCalls[0].Arguments != `{"location":"San Francisco"}` {
		t.Errorf("Expected tool call arguments, got %s", res.ToolCalls[0].Arguments)
	}
}
