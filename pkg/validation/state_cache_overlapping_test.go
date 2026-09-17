package validation

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestValidationStateCache_OverlappingSave(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	cache := NewValidationStateCache(tempDir, time.Hour)
	cache.cacheFile = filepath.Join(tempDir, "cache.json")

	cache.Set(&ValidationState{
		ObjectID:   "test-id",
		ObjectKind: "test_kind",
	})

	var wg sync.WaitGroup
	numSaves := 5
	errCh := make(chan error, numSaves)
	for i := 0; i < numSaves; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("validation_test", "overlapping cache Save").StartSimple(func() {
			defer wg.Done()
			if err := cache.Save(); err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Save failed: %v", err)
	}
}
