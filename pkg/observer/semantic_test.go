package observer

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

type mockLLMClient struct {
	intentReturn    string
	embeddingReturn []float32
}

func (m *mockLLMClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return m.intentReturn, nil
}

func (m *mockLLMClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return m.embeddingReturn, nil
}

func (m *mockLLMClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *mockLLMClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "Mock completion", nil
}

func (m *mockLLMClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	return llm.StructuredCompletionResponse{Content: "mock structured"}, nil
}

func (m *mockLLMClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}
func (m *mockLLMClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}
func (m *mockLLMClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}
func (m *mockLLMClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0, nil
}

func TestSemanticEnhancer_Enhance(t *testing.T) {
	client := &mockLLMClient{
		intentReturn:    "Mocked intent",
		embeddingReturn: []float32{1.0, 2.0},
	}

	enhancer := NewSemanticEnhancer(context.Background(), client, nil)

	result := &ExtractResult{
		Entities: []Entity{
			{
				Kind:      "function",
				Name:      "TestFunc",
				File:      "test.go",
				Signature: "func TestFunc()",
			},
			{
				Kind: "file", // Should be ignored
				Name: "test.go",
				File: "test.go",
			},
		},
	}

	err := enhancer.Enhance(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify function was enhanced
	funcEntity := result.Entities[0]
	if funcEntity.Intent != "Mocked intent" {
		t.Errorf("expected intent 'Mocked intent', got '%s'", funcEntity.Intent)
	}
	if len(funcEntity.Embedding) != 2 || funcEntity.Embedding[0] != 1.0 {
		t.Errorf("expected embedding [1.0, 2.0], got %v", funcEntity.Embedding)
	}

	// Verify file was ignored
	fileEntity := result.Entities[1]
	if fileEntity.Intent != "" {
		t.Errorf("expected empty intent for file, got '%s'", fileEntity.Intent)
	}
}

func TestSemanticEnhancer_ExcludesTestFilesByDefault(t *testing.T) {
	client := &mockLLMClient{
		intentReturn:    "Mocked intent",
		embeddingReturn: []float32{1.0, 2.0},
	}

	enhancer := NewSemanticEnhancer(context.Background(), client, nil)

	result := &ExtractResult{
		Entities: []Entity{
			{
				Kind:      "function",
				Name:      "TestHelper",
				File:      "foo_test.go",
				Signature: "func TestHelper()",
			},
			{
				Kind:      "function",
				Name:      "RealFunc",
				File:      "foo.go",
				Signature: "func RealFunc()",
			},
		},
	}

	err := enhancer.Enhance(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// foo_test.go entity should NOT be enhanced
	if result.Entities[0].Intent != "" {
		t.Errorf("expected test file entity to be skipped, got intent %q", result.Entities[0].Intent)
	}

	// foo.go entity should be enhanced
	if result.Entities[1].Intent != "Mocked intent" {
		t.Errorf("expected production entity to be enhanced, got intent %q", result.Entities[1].Intent)
	}
}
