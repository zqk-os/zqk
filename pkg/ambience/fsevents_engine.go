package ambience

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// FSEventsEngine implements AnticipatoryEngine for file system events.
type FSEventsEngine struct {
	mesh    EventMesh
	mu      sync.Mutex
	running atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewFSEventsEngine creates a new FSEventsEngine.
func NewFSEventsEngine(mesh EventMesh) *FSEventsEngine {
	return &FSEventsEngine{
		mesh: mesh,
	}
}

// Start begins listening to file modification events to proactively load context.
func (e *FSEventsEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running.Load() {
		return nil // Already running
	}

	ctx, cancel := context.WithCancel(ctx)
	ch, err := e.mesh.Subscribe(ctx, []EventType{EventFileModified})
	if err != nil {
		cancel()
		return err
	}

	done := make(chan struct{})
	e.cancel = cancel
	e.done = done
	e.running.Store(true)
	goroutinelabels.NewGoroutine("ambience.fsevents_loop", "listening for fsevents").StartSimple(func() {
		defer close(done)
		e.loop(ctx, ch)
	})
	return nil
}

func (e *FSEventsEngine) loop(ctx context.Context, ch <-chan AmbientEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// Just a simple prediction logic for now
			// In a real system this would parse AST or update Semantic Graph
			_, _ = e.PredictIntent(ev)
		}
	}
}

// Stop gracefully shuts down the engine.
func (e *FSEventsEngine) Stop() error {
	e.mu.Lock()
	if !e.running.Swap(false) {
		e.mu.Unlock()
		return nil
	}
	cancel := e.cancel
	done := e.done
	e.cancel = nil
	e.done = nil
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

// PredictIntent analyzes an ambient event to determine the likely user intent.
func (e *FSEventsEngine) PredictIntent(event AmbientEvent) (Intent, error) {
	if event.Type != EventFileModified {
		return Intent{}, nil
	}

	// Basic heuristic: if it's a Go test file, intent is likely testing.
	// If it's a Go source file, intent is likely development/building.
	target := event.URI

	if strings.HasSuffix(target, "_test.go") {
		return Intent{
			Action:     "run_tests",
			Target:     target,
			Confidence: 0.8,
		}, nil
	}

	if strings.HasSuffix(target, ".go") {
		return Intent{
			Action:     "build",
			Target:     target,
			Confidence: 0.7,
		}, nil
	}

	if strings.HasSuffix(target, "go.mod") || strings.HasSuffix(target, "go.sum") {
		return Intent{
			Action:     "mod_tidy",
			Target:     target,
			Confidence: 0.9,
		}, nil
	}

	return Intent{
		Action:     "edit",
		Target:     target,
		Confidence: 0.5,
	}, nil
}
