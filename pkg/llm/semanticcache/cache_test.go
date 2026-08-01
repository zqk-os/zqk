package semanticcache

import (
	"context"
	"reflect"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/storage"
)

type mockLLMClient struct {
	intentCalled    bool
	embeddingCalled bool
	intentReturn    string
	embeddingReturn []float32
}

func (m *mockLLMClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	m.intentCalled = true
	return m.intentReturn, nil
}

func (m *mockLLMClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	m.embeddingCalled = true
	return m.embeddingReturn, nil
}

func (m *mockLLMClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
}

func (m *mockLLMClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "mock completion", nil
}

func (m *mockLLMClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	return llm.StructuredCompletionResponse{Content: "mock structured"}, nil
}

func (m *mockLLMClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
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

type mockStorage struct {
	storage.NoopObjectStorage
	queryReturn *storage.QueryResult
}

func (m *mockStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	return m.queryReturn, nil
}

func TestSemanticCache_Hit(t *testing.T) {
	mockClient := &mockLLMClient{
		intentReturn:    "Generated intent",
		embeddingReturn: []float32{0.1, 0.2, 0.3},
	}

	mockStore := &mockStorage{
		queryReturn: &storage.QueryResult{
			Objects: []map[string]any{
				{"intent": "Cached intent"},
			},
		},
	}

	cache := NewSemanticCache(mockClient, mockStore)

	intent, err := cache.GenerateIntent(context.Background(), "func main() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if intent != "Cached intent" {
		t.Errorf("expected cached intent, got %s", intent)
	}

	if mockClient.intentCalled {
		t.Error("LLM GenerateIntent should not have been called on cache hit")
	}
	if !mockClient.embeddingCalled {
		t.Error("LLM GenerateEmbedding should have been called to perform lookup")
	}
}

func TestSemanticCache_Miss(t *testing.T) {
	mockClient := &mockLLMClient{
		intentReturn:    "Generated intent",
		embeddingReturn: []float32{0.1, 0.2, 0.3},
	}

	mockStore := &mockStorage{
		queryReturn: &storage.QueryResult{
			Objects: []map[string]any{}, // Empty result
		},
	}

	cache := NewSemanticCache(mockClient, mockStore)

	intent, err := cache.GenerateIntent(context.Background(), "func main() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if intent != "Generated intent" {
		t.Errorf("expected generated intent, got %s", intent)
	}

	if !mockClient.intentCalled {
		t.Error("LLM GenerateIntent should have been called on cache miss")
	}
}

func TestSemanticCache_GenerateEmbedding(t *testing.T) {
	mockClient := &mockLLMClient{
		embeddingReturn: []float32{0.5, 0.5},
	}
	cache := NewSemanticCache(mockClient, nil)

	emb, err := cache.GenerateEmbedding(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []float32{0.5, 0.5}
	if !reflect.DeepEqual(emb, expected) {
		t.Errorf("expected %v, got %v", expected, emb)
	}
	if !mockClient.embeddingCalled {
		t.Error("LLM GenerateEmbedding should have been called")
	}
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }

func TestSemanticCache_GenerateCompletion(t *testing.T) {
	mockClient := &mockLLMClient{}
	cache := NewSemanticCache(mockClient, nil)

	res, err := cache.GenerateCompletion(context.Background(), "prompt", "system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res != "mock completion" {
		t.Errorf("expected 'mock completion', got %s", res)
	}
}
