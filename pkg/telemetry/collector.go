package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
)

var (
	ErrTelemetryOptedOut   = errors.New("telemetry: collection is disabled (opt-in required)")
	ErrEmptyTelemetryEvent = errors.New("telemetry: event name cannot be empty")
	ErrCollectorStopped    = errors.New("telemetry: collector is stopped")
)

// MetricEvent represents an anonymized telemetry data point.
type MetricEvent struct {
	EventID   string         `json:"event_id"`
	EventName string         `json:"event_name"`
	Timestamp time.Time      `json:"timestamp"`
	OS        string         `json:"os"`
	Arch      string         `json:"arch"`
	GoVersion string         `json:"go_version"`
	Metrics   map[string]any `json:"metrics,omitempty"`
}

// DiagnosticSnapshot records anonymous system performance telemetry.
type DiagnosticSnapshot struct {
	Timestamp       time.Time `json:"timestamp"`
	NumCPU          int       `json:"num_cpu"`
	NumGoroutine    int       `json:"num_goroutine"`
	AllocBytes      uint64    `json:"alloc_bytes"`
	TotalAllocBytes uint64    `json:"total_alloc_bytes"`
	SysBytes        uint64    `json:"sys_bytes"`
	NumGC           uint32    `json:"num_gc"`
}

// DiagnosticMetricsCollector coordinates anonymized diagnostic metrics collection with strict opt-in controls.
type DiagnosticMetricsCollector struct {
	mu          sync.RWMutex
	events      []MetricEvent
	maxBuffered int
	stopped     bool
	targetURL   string
	httpClient  *http.Client
}

// CollectorOption configures the DiagnosticMetricsCollector.
type CollectorOption func(*DiagnosticMetricsCollector)

// WithMaxBuffered sets the buffer capacity for collected telemetry events.
func WithMaxBuffered(limit int) CollectorOption {
	return func(c *DiagnosticMetricsCollector) {
		if limit > 0 {
			c.maxBuffered = limit
		}
	}
}

// WithCollectorEndpoint sets an optional remote endpoint for exporting diagnostic reports.
func WithCollectorEndpoint(endpoint string) CollectorOption {
	return func(c *DiagnosticMetricsCollector) {
		c.targetURL = endpoint
	}
}

// NewDiagnosticMetricsCollector creates a new DiagnosticMetricsCollector instance.
func NewDiagnosticMetricsCollector(opts ...CollectorOption) *DiagnosticMetricsCollector {
	c := &DiagnosticMetricsCollector{
		events:      make([]MetricEvent, 0, 100),
		maxBuffered: 500,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// RecordEvent records an anonymized telemetry event if opted-in.
func (c *DiagnosticMetricsCollector) RecordEvent(eventName string, rawAttributes map[string]any) (MetricEvent, error) {
	if !IsOptedIn() {
		return MetricEvent{}, ErrTelemetryOptedOut
	}
	if strings.TrimSpace(eventName) == "" {
		return MetricEvent{}, ErrEmptyTelemetryEvent
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return MetricEvent{}, ErrCollectorStopped
	}

	sanitizedMetrics := make(map[string]any, len(rawAttributes))
	for k, v := range rawAttributes {
		switch val := v.(type) {
		case string:
			sanitizedMetrics[k] = SanitizePayload(val)
		default:
			sanitizedMetrics[k] = val
		}
	}

	event := MetricEvent{
		EventID:   fmt.Sprintf("telem-%d", time.Now().UnixNano()),
		EventName: eventName,
		Timestamp: time.Now().UTC(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Metrics:   sanitizedMetrics,
	}

	c.events = append(c.events, event)
	if len(c.events) > c.maxBuffered {
		c.events = c.events[len(c.events)-c.maxBuffered:]
	}

	return event, nil
}

// CaptureDiagnosticSnapshot captures memory and runtime performance stats with zero personal data.
func (c *DiagnosticMetricsCollector) CaptureDiagnosticSnapshot() (DiagnosticSnapshot, error) {
	if !IsOptedIn() {
		return DiagnosticSnapshot{}, ErrTelemetryOptedOut
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return DiagnosticSnapshot{
		Timestamp:       time.Now().UTC(),
		NumCPU:          runtime.NumCPU(),
		NumGoroutine:    runtime.NumGoroutine(),
		AllocBytes:      m.Alloc,
		TotalAllocBytes: m.TotalAlloc,
		SysBytes:        m.Sys,
		NumGC:           m.NumGC,
	}, nil
}

// Flush returns all buffered events and empties the collector buffer.
func (c *DiagnosticMetricsCollector) Flush() []MetricEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	flushed := make([]MetricEvent, len(c.events))
	copy(flushed, c.events)
	c.events = c.events[:0]
	return flushed
}

// Close gracefully stops the collector.
func (c *DiagnosticMetricsCollector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
}

// ExportJSON marshals a slice of events into sanitized JSON bytes.
func ExportJSON(events []MetricEvent) ([]byte, error) {
	return json.Marshal(events)
}

// ExportToFile writes anonymized diagnostic telemetry to a secure file.
func ExportToFile(filePath string, events []MetricEvent) error {
	data, err := ExportJSON(events)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, paths.FilePerm600)
}

// AsyncExport launches background export adhering to ZQK goroutine policies.
func (c *DiagnosticMetricsCollector) AsyncExport(ctx context.Context, callback func(events []MetricEvent, err error)) {
	events := c.Flush()
	if len(events) == 0 {
		if callback != nil {
			callback(nil, nil)
		}
		return
	}

	goroutinelabels.NewGoroutine("telemetry", "export").StartSimple(func() {
		if callback != nil {
			callback(events, nil)
		}
	})
}
