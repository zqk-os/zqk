package indexer

import (
	"context"
)

type EmbeddingService interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type MemoryStore interface {
	Upsert(ctx context.Context, id string, vector []float32) error
}

type IndexerWorker struct {
	embeddingService EmbeddingService
	memoryStore      MemoryStore
}

func NewIndexerWorker(e EmbeddingService, m MemoryStore) *IndexerWorker {
	return &IndexerWorker{
		embeddingService: e,
		memoryStore:      m,
	}
}

func (w *IndexerWorker) Index(ctx context.Context, id string, text string) error {
	vector, err := w.embeddingService.Embed(ctx, text)
	if err != nil {
		return err
	}
	return w.memoryStore.Upsert(ctx, id, vector)
}
