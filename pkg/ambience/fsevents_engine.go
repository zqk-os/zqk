package ambience

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// FSEventsEngine implements AnticipatoryEngine for file system events.
type FSEventsEngine struct {
	mesh    EventMesh
	running atomic.Bool
	cancel  context.CancelFunc
}

// NewFSEventsEngine creates a new FSEventsEngine.
func NewFSEventsEngine(mesh EventMesh) *FSEventsEngine {
	return &FSEventsEngine{
		mesh: mesh,
	}
}

// Start begins listening to file modification events to proactively load context.
func (e *FSEventsEngine) Start(ctx context.Context) error {
	if !e.running.CompareAndSwap(false, true) {
		return nil // Already running
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	ch, err := e.mesh.Subscribe(ctx, []EventType{EventFileModified})
	if err != nil {
		e.running.Store(false)
		return err
	}

	goroutinelabels.NewGoroutine("ambience.fsevents_loop", "listening for fsevents").StartSimple(func() {
		e.loop(ctx, ch)
	})
	return nil
}

func (e *FSEventsEngine) loop(ctx context.Context, ch <-chan AmbientEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			// Just a simple prediction logic for now
			// In a real system this would parse AST or update Semantic Graph
			_, _ = e.PredictIntent(ev)
		}
	}
}

// Stop gracefully shuts down the engine.
func (e *FSEventsEngine) Stop() error {
	if e.running.CompareAndSwap(true, false) && e.cancel != nil {
		e.cancel()
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
