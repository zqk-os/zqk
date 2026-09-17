package infrastructure

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// BufferedSpine wraps a SpinalSpine with an internal ring buffer to prevent blocking.
type BufferedSpine struct {
	inner  SpinalSpine
	queue  chan Event
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewBufferedSpine creates a new BufferedSpine with the specified buffer size.
func NewBufferedSpine(inner SpinalSpine, bufferSize int) *BufferedSpine {
	ctx, cancel := context.WithCancel(context.Background())
	s := &BufferedSpine{
		inner:  inner,
		queue:  make(chan Event, bufferSize),
		ctx:    ctx,
		cancel: cancel,
	}
	s.start()
	return s
}

func (s *BufferedSpine) start() {
	s.wg.Add(1)
	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				defer s.wg.Done()
				goroutinelabels.SetGoroutineLabel(ConstStreamInfrastructureSpine, "worker")
				for {
					select {
					case <-s.ctx.Done():
						return
					case event, ok := <-s.queue:
						if !ok {
							return
						}
						// Attempt to publish to the inner spine with a timeout
						publishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_ = s.inner.Publish(publishCtx, event)
						cancel()
					}
				}
			}()
		})
}

func (s *BufferedSpine) Publish(ctx context.Context, event Event) error {
	select {
	case <-s.ctx.Done():
		return context.Canceled
	case s.queue <- event:
		return nil
	default:
		// Buffer full, drop or block? For high-volume, we drop but log a warning.
		// In a production scenario, we might want to spill to disk.
		return nil
	}
}

func (s *BufferedSpine) Subscribe(ctx context.Context, kind string, handler Handler) error {
	return s.inner.Subscribe(ctx, kind, handler)
}

func (s *BufferedSpine) Replay(ctx context.Context, appliedSeq int64, handler Handler) error {
	return s.inner.Replay(ctx, appliedSeq, handler)
}

func (s *BufferedSpine) Close() error {
	s.cancel()
	close(s.queue)
	s.wg.Wait()
	return s.inner.Close()
}

// Constants for goroutine labels
const ConstStreamInfrastructureSpine = "infrastructure_spine"
