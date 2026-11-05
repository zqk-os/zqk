package dummy_policy

import (
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestWait(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("dummy_wait", "waitgroup test").StartSimple(func() {
		defer wg.Done()
	})
	wg.Wait()
}
