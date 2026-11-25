package ambience

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestFSEventsEngine_ConcurrentStartStop(t *testing.T) {
	mesh := NewInMemoryEventMesh()
	engine := NewFSEventsEngine(mesh)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		goroutinelabels.NewGoroutine("test.fsevents_start", "concurrent start").StartSimple(func() {
			defer wg.Done()
			assert.NoError(t, engine.Start(context.Background()))
		})
		goroutinelabels.NewGoroutine("test.fsevents_stop", "concurrent stop").StartSimple(func() {
			defer wg.Done()
			assert.NoError(t, engine.Stop())
		})
	}
	wg.Wait()
	assert.NoError(t, engine.Stop())
}
