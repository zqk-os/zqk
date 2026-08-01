package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestOpenAIClient_GenerateStructuredCompletion_MissingAPIKey(t *testing.T) {
	client := NewOpenAIClient(&Config{}) // Empty config, no API key

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}
	tools := []ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
		},
	}

	_, err := client.GenerateStructuredCompletion(context.Background(), messages, tools)
	if err == nil {
		t.Errorf("Expected error due to missing API key, got nil")
	}

	expectedErr := "APIKey missing, cannot generate structured completion"
	if err.Error() != expectedErr {
		t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestOpenAIClient_GenerateStructuredCompletion_Success(t *testing.T) {
	// Ensure ZQK_API_KEY="account:system" is injected for tests
	os.Setenv(zqkenv.APIKey(), "account:system")
	defer os.Unsetenv(zqkenv.APIKey())

	// Create a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account:system" {
			t.Errorf("Expected Authorization header to be Bearer account:system")
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
			function, ok := toolObj["function"].(map[string]any)
			if !ok {
				t.Errorf("Expected function in tool definition")
			} else if function[objects.FieldKeyName] != "get_weather" {
				t.Errorf("Expected tool name 'get_weather', got %v", function[objects.FieldKeyName])
			}
		}

		// Mock a response that invokes the tool
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								objects.FieldKeyID:   "call_abc123",
								objects.FieldKeyType: "function",
								"function": map[string]any{
									objects.FieldKeyName: "get_weather",
									"arguments":          `{"location":"San Francisco"}`,
								},
							},
						},
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &Config{
		APIKey:    "account:system",
		BaseURL:   ts.URL,
		ChatModel: "gpt-test",
	}

	client := NewOpenAIClient(config)

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

func TestOpenAIClient_ShouldMock(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		expectMock bool
	}{
		{
			name: "Default OpenAI BaseURL with Empty APIKey",
			config: Config{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "",
			},
			expectMock: true,
		},
		{
			name: "Empty BaseURL with Empty APIKey",
			config: Config{
				BaseURL: "",
				APIKey:  "",
			},
			expectMock: true,
		},
		{
			name: "Default OpenAI BaseURL with APIKey",
			config: Config{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "some-key",
			},
			expectMock: false,
		},
		{
			name: "Custom BaseURL (Ollama) with Empty APIKey",
			config: Config{
				BaseURL: "http://localhost:11434/v1",
				APIKey:  "",
			},
			expectMock: false,
		},
		{
			name: "Custom BaseURL (Localhost) with APIKey",
			config: Config{
				BaseURL: "http://127.0.0.1:8080/v1",
				APIKey:  "local-key",
			},
			expectMock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewOpenAIClient(&tt.config)
			got := client.shouldMock()
			if got != tt.expectMock {
				t.Errorf("shouldMock() = %v, expected %v", got, tt.expectMock)
			}
		})
	}
}
