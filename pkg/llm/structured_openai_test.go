package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestOpenAIChatMessages_emptyContentIsString(t *testing.T) {
	t.Parallel()

	got := openAIChatMessages([]Message{
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "c1", Name: "zqk_write_code", Arguments: `{}`}}},
		{Role: "tool", Content: "", ToolCallID: "c1", Name: "zqk_write_code"},
		{Role: "user", Content: "hello"},
	})
	if len(got) != 3 {
		t.Fatalf("messages = %d, want 3", len(got))
	}
	for i, msg := range got {
		raw, ok := msg[objects.FieldKeyContent]
		if !ok {
			t.Fatalf("message %d omitted content (vLLM treats that as <nil>)", i)
		}
		if _, ok := raw.(string); !ok {
			t.Fatalf("message %d content type %T, want string", i, raw)
		}
	}
	if got[0][objects.FieldKeyContent] != "" {
		t.Fatalf("assistant content = %#v, want empty string", got[0][objects.FieldKeyContent])
	}
	if _, ok := got[0]["tool_calls"]; !ok {
		t.Fatal("assistant tool_calls missing")
	}
	if got[2][objects.FieldKeyContent] != "hello" {
		t.Fatalf("user content = %#v", got[2][objects.FieldKeyContent])
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"`+objects.FieldKeyContent+`":null`) {
		t.Fatalf("serialized content was null: %s", body)
	}
}

func TestOpenAIClient_GenerateStructuredCompletion_MissingAPIKey(t *testing.T) {
	client := NewOpenAIClient(context.Background(), &Config{}) // Empty config, no API key

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
	// Ensure ZQK_API_KEY="test_api_key_12345678" is injected for tests
	_ = zqkenv.APIKey().Set("test_api_key_12345678")
	defer zqkenv.APIKey().Unset()

	// Create a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_api_key_12345678" {
			t.Errorf("Expected Authorization header to be Bearer test_api_key_12345678, got %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("Failed to parse request JSON: %v", err)
		}

		if req["parallel_tool_calls"] != false {
			t.Errorf("expected parallel_tool_calls=false, got %v", req["parallel_tool_calls"])
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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &Config{
		APIKey:    "test_api_key_12345678",
		BaseURL:   ts.URL,
		ChatModel: "gpt-test",
	}

	client := NewOpenAIClient(context.Background(), config)

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

func TestOpenAIClient_toolChoiceRequiredAndUnsupportedRetry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("request json: %v", err)
		}
		if attempts == 1 {
			if req["tool_choice"] != ToolChoiceRequired {
				t.Fatalf("first attempt tool_choice = %v, want required", req["tool_choice"])
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unknown field tool_choice"}`))
			return
		}
		if _, ok := req["tool_choice"]; ok {
			t.Fatal("retry must omit tool_choice")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{objects.FieldKeyContent: "ok"}},
			},
		})
	}))
	defer ts.Close()

	client := NewOpenAIClient(context.Background(), &Config{APIKey: "k", BaseURL: ts.URL, ChatModel: "qwen-test"})
	res, err := client.GenerateStructuredCompletionWithOptions(context.Background(),
		[]Message{{Role: "user", Content: "hi"}},
		[]ToolDefinition{{Name: "test_tool"}},
		StructuredCompletionOptions{ToolChoice: ToolChoiceRequired},
	)
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if res.Content != "ok" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestOpenAIClient_toolChoiceFunctionObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("request json: %v", err)
		}
		choice, ok := req["tool_choice"].(map[string]any)
		if !ok {
			t.Fatalf("tool_choice = %T %v, want function object", req["tool_choice"], req["tool_choice"])
		}
		if choice[objects.FieldKeyType] != "function" {
			t.Fatalf("type = %v", choice[objects.FieldKeyType])
		}
		fn, ok := choice["function"].(map[string]any)
		if !ok || fn[objects.FieldKeyName] != "zqk_write_code" {
			t.Fatalf("function = %v", choice["function"])
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{objects.FieldKeyContent: "ok"}},
			},
		})
	}))
	defer ts.Close()

	client := NewOpenAIClient(context.Background(), &Config{APIKey: "k", BaseURL: ts.URL, ChatModel: "qwen-test"})
	if _, err := client.GenerateStructuredCompletionWithOptions(context.Background(),
		[]Message{{Role: "user", Content: "hi"}},
		[]ToolDefinition{{Name: "zqk_write_code"}},
		StructuredCompletionOptions{ToolChoice: ToolChoiceRequired, ToolChoiceFunction: "zqk_write_code"},
	); err != nil {
		t.Fatalf("named tool_choice: %v", err)
	}
}

func TestOpenAIClient_sendsSamplingWhenSet(t *testing.T) {
	temp := 0.0
	topP := 0.8
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("request json: %v", err)
		}
		if req["temperature"] != 0.0 {
			t.Fatalf("temperature = %v, want 0", req["temperature"])
		}
		if req["top_p"] != 0.8 {
			t.Fatalf("top_p = %v, want 0.8", req["top_p"])
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{objects.FieldKeyContent: "ok"}},
			},
		})
	}))
	defer ts.Close()

	client := NewOpenAIClient(context.Background(), &Config{
		APIKey:      "k",
		BaseURL:     ts.URL,
		ChatModel:   "qwen-test",
		Temperature: &temp,
		TopP:        &topP,
	})
	if _, err := client.GenerateStructuredCompletion(context.Background(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
	); err != nil {
		t.Fatalf("structured: %v", err)
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
			client := NewOpenAIClient(context.Background(), &tt.config)
			got := client.shouldMock()
			if got != tt.expectMock {
				t.Errorf("shouldMock() = %v, expected %v", got, tt.expectMock)
			}
		})
	}
}
