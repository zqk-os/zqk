package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// ListSegmentsParallel safely lists file segments using a semaphore-bounded worker pool
func (f *FileObjectStorage) ListSegmentsParallel(ctx context.Context, kind string, maxConcurrency int) ([]string, error) {
	segDir := filepath.Join(f.projectRoot, "data", "streams", kind)
	entries, err := os.ReadDir(segDir)
	if err != nil {
		return nil, err
	}

	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan string, len(entries))
	var wg sync.WaitGroup

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}
		goroutinelabels.NewGoroutine("storage", ConstMiscListSegmentWorker).StartSimple(func() {
			func(name string) {
				defer wg.Done()
				defer func() { <-semaphore }()
				results <- filepath.Join(segDir, name)
			}(entry.Name())
		})
	}

	goroutinelabels.NewGoroutine("storage", ConstMiscListSegmentsResultsCloser).StartSimple(func() {
		wg.Wait()
		close(results)
	})

	var output []string
	for r := range results {
		output = append(output, r)
	}
	return output, nil
}
