package mesh

import (
	"context"
	"sync"
)

// BackpressureConfig holds threshold parameters for queue depth regulation.
type BackpressureConfig struct {
	HighWatermark int
	LowWatermark  int
}

// BackpressureController monitors queue depth and signals backpressure state transitions.
type BackpressureController struct {
	mu        sync.Mutex
	healthyCh chan struct{}
	config    BackpressureConfig
	throttled bool
}

// NewBackpressureController creates a new backpressure controller.
func NewBackpressureController(cfg BackpressureConfig) *BackpressureController {
	if cfg.LowWatermark >= cfg.HighWatermark {
		cfg.LowWatermark = cfg.HighWatermark / 2
	}
	ch := make(chan struct{})
	close(ch) // Initially healthy (open)

	return &BackpressureController{
		config:    cfg,
		healthyCh: ch,
		throttled: false,
	}
}

// RecordEnqueue checks if enqueuing causes depth to meet or exceed HighWatermark.
func (c *BackpressureController) RecordEnqueue(currentDepth int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.throttled && currentDepth >= c.config.HighWatermark {
		c.throttled = true
		c.healthyCh = make(chan struct{})
	}
}

// RecordDrain checks if draining causes depth to drop to or below LowWatermark.
func (c *BackpressureController) RecordDrain(currentDepth int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.throttled && currentDepth <= c.config.LowWatermark {
		c.throttled = false
		close(c.healthyCh)
	}
}

// IsThrottled returns whether the controller is currently in throttled state.
func (c *BackpressureController) IsThrottled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.throttled
}

// WaitUntilHealthy blocks until backpressure is relieved or ctx is canceled.
func (c *BackpressureController) WaitUntilHealthy(ctx context.Context) error {
	c.mu.Lock()
	ch := c.healthyCh
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}
