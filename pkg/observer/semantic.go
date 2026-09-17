package observer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/llm/semanticcache"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// SemanticEnhancer adds intent and embeddings to extracted entities.
type SemanticEnhancer struct {
	client llm.Client
}

// NewSemanticEnhancer creates a new SemanticEnhancer.
func NewSemanticEnhancer(ctx context.Context, client llm.Client, storageProvider storage.ObjectStorageProvider) *SemanticEnhancer {
	if client == nil {
		client = llm.NewClient(ctx, nil)
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

	process.TouchMeaningfulActivity()
	logger := logging.GetLogger()
	includeTests := zqkenv.Get("ZQK_OBSERVER_INCLUDE_TESTS").BoolOrDefault(false)

	// Filter eligible entities first to know total count
	var eligibleIndices []int
	for i := range result.Entities {
		ent := &result.Entities[i]
		if !includeTests {
			base := filepath.Base(ent.File)
			if strings.HasSuffix(ent.File, "_test.go") || strings.HasPrefix(base, "mock_") || strings.HasSuffix(ent.File, "_mock.go") {
				continue
			}
		}
		kind := ent.Kind
		if kind == "function" || kind == "method" || kind == "type" || kind == "interface" || kind == "struct" {
			eligibleIndices = append(eligibleIndices, i)
		}
	}
	totalEligible := len(eligibleIndices)
	if totalEligible == 0 {
		return nil
	}

	pulseDone := make(chan struct{})
	defer close(pulseDone)
	goroutinelabels.NewGoroutine("observer_semantic_pulse", "touch meaningful activity during semantic enhancement").
		StartWithContext(ctx, func(pulseCtx context.Context) error {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-pulseCtx.Done():
					return nil
				case <-pulseDone:
					return nil
				case <-ticker.C:
					process.TouchMeaningfulActivity()
				}
			}
		})

	// Limit concurrency: configurable via ZQK_OBSERVER_CONCURRENCY, default 6
	concurrencyLimit := zqkenv.Get("ZQK_OBSERVER_CONCURRENCY").IntOrDefault(6)
	if concurrencyLimit <= 0 {
		concurrencyLimit = 6
	}

	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	var errs []error
	var mu sync.Mutex
	var completed atomic.Int64

	for _, idx := range eligibleIndices {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire token

		targetIdx := idx
		goroutinelabels.NewGoroutine("observer", "semantic_enhance").
			StartSimple(func() {
				defer wg.Done()
				defer func() { <-sem }() // Release token

				entity := &result.Entities[targetIdx]

				// Generate Intent
				codeContext := fmt.Sprintf("File: %s\nSignature: %s\nCalls: %s\nDependsOn: %s",
					entity.File,
					entity.Signature,
					strings.Join(entity.Calls, ", "),
					strings.Join(entity.DependsOn, ", "),
				)

				skipIntent := zqkenv.Get("ZQK_OBSERVER_SKIP_INTENT").BoolOrDefault(false)
				var intent string
				if !skipIntent {
					var err error
					intent, err = se.client.GenerateIntent(ctx, codeContext)
					if err != nil {
						logging.FluentEvent(logger).Debug("Intent generation failed, proceeding with direct embedding").
							String("entity", entity.Name).
							WithError(err).
							Log()
					} else {
						entity.Intent = intent
					}
				}

				// Generate Embedding
				embedText := entity.Signature
				if intent != "" {
					embedText = fmt.Sprintf("%s\n%s", entity.Signature, intent)
				} else if doc, ok := entity.Metadata["doc"]; ok && doc != "" {
					embedText = fmt.Sprintf("%s\n%s", entity.Signature, doc)
				}
				embedding, err := se.client.GenerateEmbedding(ctx, embedText)
				if err != nil {
					mu.Lock()
					errs = append(errs, errfmt.Newf("failed to generate embedding for %s", entity.Name).Wrap(err))
					mu.Unlock()
					return
				}
				entity.Embedding = embedding

				done := completed.Add(1)
				process.TouchMeaningfulActivity()
				if done%25 == 0 || int(done) == totalEligible {
					logging.FluentEvent(logger).Info("Semantic enhancement progress").
						Int("completed", int(done)).
						Int("total", totalEligible).
						Log()
				}
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("observer", "wait_semantic_enhance").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})

	select {
	case <-waitDone:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Fail closed only if significant failure rate (>50% failed) or if nothing succeeded
	if len(errs) > 0 {
		if len(errs) == totalEligible || len(errs) > totalEligible/2 {
			return errfmt.Errorf("encountered %d errors during semantic enhancement (total eligible %d), first error: %v", len(errs), totalEligible, errs[0])
		}
		logging.FluentEvent(logger).Warn("Some entities failed semantic enhancement, continuing with partial").
			Int("failed_count", len(errs)).
			Int("total", totalEligible).
			WithError(errs[0]).
			Log()
	}

	return nil
}
