package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestQwenContextWindowConfig(t *testing.T) {
	// 1. Initialize a QwenClient with a Config that specifies ContextWindowSize = 32768
	// 2. Mock the underlying HTTP transport to intercept the payload sent to Ollama
	// 3. Verify that the JSON payload includes {"options": {"num_ctx": 32768}}

	var receivedPayload map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)

		// Return a dummy valid response
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						objects.FieldKeyContent: "ok",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &Config{
		Provider: "qwen",
		BaseURL:  ts.URL,
	}

	client := NewQwenClient(config)

	_, err := client.GenerateStructuredCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	options, ok := receivedPayload["options"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'options' in payload, got %v", receivedPayload)
	}

	numCtx, ok := options["num_ctx"].(float64)
	if !ok || numCtx != 32768 {
		t.Fatalf("expected num_ctx to be 32768, got %v", numCtx)
	}
}
