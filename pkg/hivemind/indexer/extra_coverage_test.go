// BLI-STARTER-COMMUNITY-062 / PRI-STARTER-COMMUNITY-062 coverage elevation
package indexer

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/llm"
)

type extraLLM struct{}

func (extraLLM) GenerateIntent(context.Context, string) (string, error) { return "", nil }
func (extraLLM) GenerateEmbedding(_ context.Context, text string) ([]float32, error) {
	return []float32{float32(len(text))}, nil
}
func (extraLLM) AnalyzeVideoFrames(context.Context, [][]byte, string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}
func (extraLLM) GenerateCompletion(context.Context, string, string) (string, error) { return "", nil }
func (extraLLM) GenerateStructuredCompletion(context.Context, []llm.Message, []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	return llm.StructuredCompletionResponse{}, nil
}
func (extraLLM) VerifyImage(context.Context, []byte, []byte, string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}
func (extraLLM) DescribeImage(context.Context, []byte) (string, error) { return "", nil }
func (extraLLM) DescribeScene(context.Context, [][]byte) (string, error) {
	return "", nil
}
func (extraLLM) SemanticCompare(context.Context, string, string) (float64, error) { return 0, nil }

type errEmbed struct{}

func (errEmbed) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed fail")
}

type errStore struct{}

func (errStore) Upsert(context.Context, string, []float32) error {
	return errors.New("upsert fail")
}

func TestExtraEmbedAndIndexErrors(t *testing.T) {
	ctx := context.Background()
	svc := NewLLMEmbeddingService(extraLLM{})
	vec, err := svc.Embed(ctx, "hi")
	if err != nil || len(vec) != 1 {
		t.Fatalf("embed %#v %v", vec, err)
	}
	w := NewIndexerWorker(errEmbed{}, &MockMemoryStore{Vectors: map[string][]float32{}})
	if err := w.Index(ctx, "id", "text"); err == nil {
		t.Fatal("expected embed error")
	}
	w = NewIndexerWorker(&MockEmbeddingService{}, errStore{})
	if err := w.Index(ctx, "id", "text"); err == nil {
		t.Fatal("expected upsert error")
	}

	wal, err := lifecycle.NewLifecycleEventWAL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = wal.Append(&lifecycle.LifecycleEvent{EventType: "other", Kind: "requirement", ID: "x"})
	_ = wal.Append(&lifecycle.LifecycleEvent{EventType: lifecycle.EventTypeStatusTransition, Kind: "backlog", ID: "b"})
	_ = wal.Append(&lifecycle.LifecycleEvent{EventType: lifecycle.EventTypeStatusTransition, Kind: "specification", ID: "spec-1"})
	_ = wal.Sync()
	listener := NewIndexerListener(wal, NewIndexerWorker(errEmbed{}, errStore{}))
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := listener.Run(runCtx); err == nil {
		t.Fatal("expected canceled")
	}
}
