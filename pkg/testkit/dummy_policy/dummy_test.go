package dummy_policy

import (
	"sync"
	"testing"
)

func TestWait(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	wg.Wait()
}
