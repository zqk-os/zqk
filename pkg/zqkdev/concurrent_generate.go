package zqkdev

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/logging"
	"golang.org/x/sync/errgroup"
)

// GenerateItem represents a single YAML spec file to process during concurrent generation.
type GenerateItem struct {
	// YAMLPath is the absolute or relative path to the YAML spec file.
	YAMLPath string
	// BaseName is the derived base name used for output file naming.
	BaseName string
}

// GenerateErrorDetail captures per-item error information for post-generation reporting.
type GenerateErrorDetail struct {
	Path string
	Err  error
}

// GenerateResult aggregates outcomes from a concurrent generation run.
type GenerateResult struct {
	Generated    int
	Skipped      int
	Errors       int
	ErrorDetails []GenerateErrorDetail
}

// GenerateFunc is the function signature for processing a single GenerateItem.
// Implementations should respect the context for cancellation.
type GenerateFunc func(ctx context.Context, item GenerateItem) error

// ConcurrentGenerate processes a slice of GenerateItems concurrently using errgroup
// with bounded concurrency (GOMAXPROCS). It collects per-item outcomes and returns
// an aggregate result. Errors in individual items do not abort the batch; all items
// are attempted unless the parent context is cancelled.
//
// This replaces the sequential for-range loops in the various generate-*-builders
// commands, per the bounded-concurrency mandate in PRE_CHANGE_CHECKLIST §2/§4.
func ConcurrentGenerate(ctx context.Context, items []GenerateItem, fn GenerateFunc) GenerateResult {
	if len(items) == 0 {
		return GenerateResult{}
	}

	logger := logging.GetLogger()
	maxWorkers := runtime.GOMAXPROCS(0)
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	var generated atomic.Int32
	var errCount atomic.Int32

	var mu sync.Mutex
	var errorDetails []GenerateErrorDetail

	eg, gCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxWorkers)

	for _, item := range items {
		item := item // capture loop variable
		eg.Go(func() error {
			if err := gCtx.Err(); err != nil {
				// Context already cancelled; count as error
				errCount.Add(1)
				mu.Lock()
				errorDetails = append(errorDetails, GenerateErrorDetail{Path: item.YAMLPath, Err: err})
				mu.Unlock()
				return nil // Don't abort others
			}

			if err := fn(gCtx, item); err != nil {
				errCount.Add(1)
				mu.Lock()
				errorDetails = append(errorDetails, GenerateErrorDetail{Path: item.YAMLPath, Err: err})
				mu.Unlock()

				logging.FluentEvent(logger).Error("Concurrent generation failed for item", err).
					WithFields(logging.String("path", item.YAMLPath)).
					Log()

				return nil // Continue processing other items
			}

			generated.Add(1)
			logging.FluentEvent(logger).Debug("Concurrent generation succeeded").
				WithFields(logging.String("path", item.YAMLPath)).
				Log()

			return nil
		})
	}

	// Wait for all workers; since individual funcs return nil, this won't error.
	_ = eg.Wait()

	return GenerateResult{
		Generated:    int(generated.Load()),
		Errors:       int(errCount.Load()),
		ErrorDetails: errorDetails,
	}
}
