package breeding

import (
	"context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Listener watches the ZQK event mesh for ambient triggers to initiate breeding.
type Listener struct {
	router *events.Router
	engine *Engine
	ch     chan events.Shape
	insp   events.Inspector
}

// NewListener creates a new ambient listener for the breeding engine.
func NewListener(router *events.Router, engine *Engine) *Listener {
	return &Listener{
		router: router,
		engine: engine,
		ch:     make(chan events.Shape, 100),
		insp:   events.NewBitmaskInspector(events.EventTypeObjectMutated),
	}
}

// Start subscribes to the event mesh and processes events in the background.
// When a MaturationReport is mutated, it autonomously triggers a breeding pass.
func (l *Listener) Start(ctx context.Context) {
	l.router.Subscribe(l.insp, l.ch)

	goroutinelabels.StartNamedGoroutine("skill-breeding-listener", "listen for skill breeding events", func() {
		func() {
			defer l.router.Unsubscribe(l.insp, l.ch)
			for {
				select {
				case <-ctx.Done():
					return
				case shape := <-l.ch:
					if shape.Kind == objects.MaturationReport {
						log := logging.GetLoggerFromContext(ctx)
						log.LogInfo("ambient event triggered breeding check", logging.String("target_id", shape.TargetID))
						if err := l.engine.Breed(ctx); err != nil {
							log.LogError("ambient breeding run failed", err)
						}
					}
				}
			}
		}()
	})
}
