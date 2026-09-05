package storage

import (
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// AuditEventBufferRegistry manages scoped AuditEventBuffer instances.
type AuditEventBufferRegistry struct {
	mu      sync.RWMutex
	buffers map[string]*AuditEventBuffer
}

var (
	registry     *AuditEventBufferRegistry
	registryOnce sync.Once
)

// GetGlobalBufferRegistry returns the singleton registry instance.
func GetGlobalBufferRegistry() *AuditEventBufferRegistry {
	registryOnce.Do(func() {
		registry = &AuditEventBufferRegistry{
			buffers: make(map[string]*AuditEventBuffer),
		}
	})
	return registry
}

// GetOrCreate returns a buffer for the given project root, creating it if necessary.
func (r *AuditEventBufferRegistry) GetOrCreate(projectRoot string, secCtx *pkgctx.SecurityContext) *AuditEventBuffer {
	r.mu.Lock()
	defer r.mu.Unlock()

	if buf, ok := r.buffers[projectRoot]; ok {
		return buf
	}

	// Load configuration
	config, err := LoadAggregationConfig(projectRoot)
	if err != nil {
		config = DefaultAggregationConfig()
	}

	windowSize := time.Hour
	if config.WindowSize != emptyValue {
		if parsed, err := ParseWindowSize(config.WindowSize); err == nil {
			windowSize = parsed
		}
	}

	newBuffer := NewAuditEventBufferWithConfig(projectRoot, secCtx, config.Rules, windowSize, config.Threshold, config.Enabled)
	r.buffers[projectRoot] = newBuffer
	return newBuffer
}

// Unregister flushes and removes the buffer for the given project root.
func (r *AuditEventBufferRegistry) Unregister(projectRoot string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if buf, ok := r.buffers[projectRoot]; ok {
		buf.Flush()
		buf.cancel()
		delete(r.buffers, projectRoot)
	}
}
