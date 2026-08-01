package observer

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/llm/semanticcache"
	"github.com/lanceman/zqk/pkg/storage"
)

// SemanticEnhancer adds intent and embeddings to extracted entities.
type SemanticEnhancer struct {
	client llm.Client
}

// NewSemanticEnhancer creates a new SemanticEnhancer.
func NewSemanticEnhancer(client llm.Client, storageProvider storage.ObjectStorageProvider) *SemanticEnhancer {
	if client == nil {
		client = llm.NewClient(nil)
	}

	// Wrap client in SemanticCache if storage provider is available
	if storageProvider != nil {
		client = semanticcache.NewSemanticCache(client, storageProvider)
	}

	return &SemanticEnhancer{
		client: client,
	}
}

// Enhance adds semantic properties to a batch of entities concurrently.
func (se *SemanticEnhancer) Enhance(ctx context.Context, result *ExtractResult) error {
	if result == nil || len(result.Entities) == 0 {
		return nil
	}

	// We'll limit concurrency to avoid hitting API rate limits immediately
	concurrencyLimit := 10
	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	var errs []error
	var mu sync.Mutex

	for i := range result.Entities {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Only enhance functions, methods, types
		kind := result.Entities[i].Kind
		if kind != "function" && kind != "method" && kind != "type" && kind != "interface" && kind != "struct" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire token

		idx := i
		goroutinelabels.NewGoroutine("observer", "semantic_enhance").
			StartSimple(func() {
				defer wg.Done()
				defer func() { <-sem }() // Release token

				entity := &result.Entities[idx]

				// Generate Intent
				// Include signature and some basic context to give the model a hint
				codeContext := fmt.Sprintf("File: %s\nSignature: %s\nCalls: %s\nDependsOn: %s",
					entity.File,
					entity.Signature,
					strings.Join(entity.Calls, ", "),
					strings.Join(entity.DependsOn, ", "),
				)

				intent, err := se.client.GenerateIntent(ctx, codeContext)
				if err != nil {
					mu.Lock()
					errs = append(errs, errfmt.Newf("failed to generate intent for %s", entity.Name).Wrap(err))
					mu.Unlock()
					return
				}
				entity.Intent = intent

				// Generate Embedding
				// Combine the signature and intent for a rich semantic vector
				embedText := fmt.Sprintf("%s\n%s", entity.Signature, intent)
				embedding, err := se.client.GenerateEmbedding(ctx, embedText)
				if err != nil {
					mu.Lock()
					errs = append(errs, errfmt.Newf("failed to generate embedding for %s", entity.Name).Wrap(err))
					mu.Unlock()
					return
				}
				entity.Embedding = embedding

			})
	}

	wg.Wait()

	if len(errs) > 0 {
		return errfmt.Errorf("encountered %d errors during semantic enhancement, first error: %v", len(errs), errs[0])
	}

	return nil
}
