package indexer

import (
	"context"

	"github.com/lanceman/zqk/pkg/llm"
)

// LLMEmbeddingService implements EmbeddingService using pkg/llm.
type LLMEmbeddingService struct {
	client llm.Client
}

// NewLLMEmbeddingService creates a new LLMEmbeddingService.
func NewLLMEmbeddingService(client llm.Client) *LLMEmbeddingService {
	return &LLMEmbeddingService{
		client: client,
	}
}

// Embed generates an embedding for the given text.
func (s *LLMEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	return s.client.GenerateEmbedding(ctx, text)
}
