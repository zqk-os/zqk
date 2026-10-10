package id_generation

import (
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAllocateSequenceRange_Concurrent(t *testing.T) {
	dir := t.TempDir()
	prefix := "AUD"
	const numGoroutines = 20
	const allocationsPerGoroutine = 5

	var wg sync.WaitGroup
	idChan := make(chan string, numGoroutines*allocationsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("sequence_test", "concurrent sequence allocation").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < allocationsPerGoroutine; j++ {
				ids, err := AllocateSequenceRange(dir, prefix, 0, 1, 1, nil)
				if err != nil {
					t.Errorf("AllocateSequenceRange error: %v", err)
					return
				}
				if len(ids) != 1 {
					t.Errorf("expected 1 id, got %d", len(ids))
					return
				}
				idChan <- ids[0]
			}
		})
	}

	wg.Wait()
	close(idChan)

	seen := make(map[string]bool)
	for id := range idChan {
		if seen[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != numGoroutines*allocationsPerGoroutine {
		t.Errorf("expected %d unique IDs, got %d", numGoroutines*allocationsPerGoroutine, len(seen))
	}
}

