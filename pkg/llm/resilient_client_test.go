package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/mesh"
	"github.com/lanceman/zqk/pkg/objects"
)

type MockLLMClient struct {
	Client
	GenerateStructuredCompletionFunc func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error)
	GenerateCompletionFunc           func(ctx context.Context, prompt string, system string) (string, error)
}

func (m *MockLLMClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	if m.GenerateStructuredCompletionFunc != nil {
		return m.GenerateStructuredCompletionFunc(ctx, messages, tools)
	}
	return StructuredCompletionResponse{}, nil
}

func (m *MockLLMClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	if m.GenerateCompletionFunc != nil {
		return m.GenerateCompletionFunc(ctx, prompt, system)
	}
	return "", nil
}

func (m *MockLLMClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (m *MockLLMClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (mesh.AnalysisResult, error) {
	return mesh.AnalysisResult{}, nil
}

func (m *MockLLMClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (mesh.AnalysisResult, error) {
	return mesh.AnalysisResult{}, nil
}

func (m *MockLLMClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}

func (m *MockLLMClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}

func (m *MockLLMClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0, nil
}

func TestResilientClient_FallbackOnTimeoutOrError(t *testing.T) {
	primaryCalled := false
	secondaryCalled := false

	primary := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
			primaryCalled = true
			return StructuredCompletionResponse{}, errors.New("timeout or connection failure")
		},
	}

	secondary := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
			secondaryCalled = true
			return StructuredCompletionResponse{Content: "success from secondary"}, nil
		},
	}

	client := NewResilientClient(primary, secondary, 50*time.Millisecond)
	client.retryDelay = time.Millisecond
	client.maxRetries = 2

	resp, err := client.GenerateStructuredCompletion(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !primaryCalled {
		t.Error("Expected primary client to be called")
	}
	if !secondaryCalled {
		t.Error("Expected secondary client to be called as fallback")
	}
	if resp.Content != "success from secondary" {
		t.Errorf("Expected content 'success from secondary', got %q", resp.Content)
	}
}

func TestResilientClient_RequestTimeout(t *testing.T) {
	primary := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
			select {
			case <-ctx.Done():
				return StructuredCompletionResponse{}, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return StructuredCompletionResponse{Content: "primary finished too late"}, nil
			}
		},
	}

	secondary := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
			return StructuredCompletionResponse{Content: "success from secondary after timeout"}, nil
		},
	}

	// Set short timeout to force deadline exceeded
	client := NewResilientClient(primary, secondary, 10*time.Millisecond)
	client.retryDelay = time.Millisecond
	client.maxRetries = 2

	resp, err := client.GenerateStructuredCompletion(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.Content != "success from secondary after timeout" {
		t.Errorf("Expected content 'success from secondary after timeout', got %q", resp.Content)
	}
}

func TestResilientClient_NoMockFallbackOnInvalidRequest(t *testing.T) {
	t.Parallel()

	calls := 0
	primary := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
			calls++
			return StructuredCompletionResponse{}, errors.New(`API error: status=400 body={"error":{"message":"invalid message content type: <nil>","` + objects.FieldKeyType + `":"invalid_request_error"}}`)
		},
	}
	client := NewResilientClient(primary, nil, 20*time.Millisecond)
	client.retryDelay = time.Millisecond
	client.maxRetries = 3

	_, err := client.GenerateStructuredCompletion(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected 400 to fail closed, not mock-fallback")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("error = %v, want the original 400", err)
	}
	if calls != 1 {
		t.Fatalf("primary calls = %d, want 1 (no retries on invalid_request)", calls)
	}
}

func TestIsNonRetryableLLMError(t *testing.T) {
	t.Parallel()
	if isNonRetryableLLMError(nil) {
		t.Fatal("nil is retryable")
	}
	if !isNonRetryableLLMError(errors.New("API error: status=400 body=x")) {
		t.Fatal("400 should be non-retryable")
	}
	if isNonRetryableLLMError(errors.New("connection refused")) {
		t.Fatal("transport errors stay retryable")
	}
}

func TestResilientClient_MockFallbackWhenNoSecondary(t *testing.T) {
	primary := &MockLLMClient{
		GenerateCompletionFunc: func(ctx context.Context, prompt string, system string) (string, error) {
			return "", errors.New("connection refused")
		},
	}
	client := NewResilientClient(primary, nil, 20*time.Millisecond)
	client.retryDelay = time.Millisecond
	client.maxRetries = 2

	out, err := client.GenerateCompletion(context.Background(), "hi", "")
	if err != nil {
		t.Fatalf("expected mock fallback success, got %v", err)
	}
	if !strings.Contains(out, mockUnavailablePrefix) {
		t.Fatalf("expected mock prefix in %q", out)
	}
}
