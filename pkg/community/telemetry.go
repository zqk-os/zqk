package community

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultMaxDiagnosticBatchSize is the maximum items in a single telemetry collection buffer.
	DefaultMaxDiagnosticBatchSize = 1000
	// DefaultMaxMetricPayloadBytes limits individual telemetry payload records to 256 KB.
	DefaultMaxMetricPayloadBytes = 256 * 1024
)

// MetricEvent represents an individual anonymized operational metric event.
type MetricEvent struct {
	EventID       string            `json:"event_id"`
	Timestamp     time.Time         `json:"timestamp"`
	Category      string            `json:"category"`
	Action        string            `json:"action"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	DurationMs    int64             `json:"duration_ms,omitempty"`
	ClusterHash   string            `json:"cluster_hash,omitempty"`
	ExecutionMode string            `json:"execution_mode,omitempty"`
}

// DiagnosticSnapshot captures a snapshot of node health and anonymous execution metrics.
type DiagnosticSnapshot struct {
	SnapshotID  string        `json:"snapshot_id"`
	CollectedAt time.Time     `json:"collected_at"`
	ClusterHash string        `json:"cluster_hash"`
	OptedIn     bool          `json:"opted_in"`
	TotalEvents int           `json:"total_events"`
	Events      []MetricEvent `json:"events"`
	Summary     MetricSummary `json:"summary"`
}

// MetricSummary summarizes aggregated metrics for a diagnostic snapshot.
type MetricSummary struct {
	Categories   map[string]int `json:"categories"`
	TotalErrors  int            `json:"total_errors"`
	TotalSuccess int            `json:"total_success"`
}

// TelemetryCollector manages opt-in state, diagnostic event buffering, sanitization, and local export.
type TelemetryCollector struct {
	mu          sync.RWMutex
	optedIn     bool
	clusterSeed string
	buffer      []MetricEvent
	maxBatch    int
}

// NewTelemetryCollector creates a new TelemetryCollector. Default posture is opt-out (optedIn=false).
func NewTelemetryCollector(clusterSeed string) *TelemetryCollector {
	if clusterSeed == "" {
		clusterSeed = "community-default"
	}
	return &TelemetryCollector{
		optedIn:     false,
		clusterSeed: clusterSeed,
		buffer:      make([]MetricEvent, 0),
		maxBatch:    DefaultMaxDiagnosticBatchSize,
	}
}

// SetOptIn toggles the collection permission gate.
func (c *TelemetryCollector) SetOptIn(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.optedIn = enabled
	telemetry.SetOptIn(enabled)
}

// IsOptedIn reports whether collection is active.
func (c *TelemetryCollector) IsOptedIn() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.optedIn
}

// AnonymizedClusterHash returns a deterministic one-way SHA-256 hash of the node/cluster identifier.
func (c *TelemetryCollector) AnonymizedClusterHash() string {
	hasher := sha256.New()
	hasher.Write([]byte(c.clusterSeed))
	return hex.EncodeToString(hasher.Sum(nil))[:16]
}

// RecordEvent records a metric event if opted-in. If opted-out, it silently discards the event.
func (c *TelemetryCollector) RecordEvent(event MetricEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.optedIn {
		// Opt-out zero transmission gate
		return nil
	}

	if event.Category == "" || event.Action == "" {
		return errors.New("event category and action are required")
	}

	// Sanitize all string attributes
	cleanAttributes := make(map[string]string, len(event.Attributes))
	for k, v := range event.Attributes {
		cleanK := telemetry.SanitizePayload(k)
		cleanV := telemetry.SanitizePayload(v)
		cleanAttributes[cleanK] = cleanV
	}
	event.Attributes = cleanAttributes

	if event.EventID == "" {
		event.EventID = fmt.Sprintf("met-%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	event.ClusterHash = c.AnonymizedClusterHash()

	// Check payload byte limit
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	if len(data) > DefaultMaxMetricPayloadBytes {
		return fmt.Errorf("metric event exceeds maximum allowed size of %d bytes", DefaultMaxMetricPayloadBytes)
	}

	// Buffer management
	if len(c.buffer) >= c.maxBatch {
		// Drop oldest event if buffer is full
		c.buffer = c.buffer[1:]
	}
	c.buffer = append(c.buffer, event)
	return nil
}

// Snapshot creates an aggregated diagnostic report snapshot and optionally drains the buffer.
func (c *TelemetryCollector) Snapshot(drain bool) DiagnosticSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	events := make([]MetricEvent, len(c.buffer))
	copy(events, c.buffer)

	if drain {
		c.buffer = make([]MetricEvent, 0)
	}

	summary := MetricSummary{
		Categories: make(map[string]int),
	}
	for _, e := range events {
		summary.Categories[e.Category]++
		if e.Attributes["result"] == "error" || e.Attributes["error"] != "" {
			summary.TotalErrors++
		} else {
			summary.TotalSuccess++
		}
	}

	return DiagnosticSnapshot{
		SnapshotID:  fmt.Sprintf("snap-%d", time.Now().UnixNano()),
		CollectedAt: time.Now().UTC(),
		ClusterHash: c.AnonymizedClusterHash(),
		OptedIn:     c.optedIn,
		TotalEvents: len(events),
		Events:      events,
		Summary:     summary,
	}
}

// ExportToFile writes the diagnostic snapshot to a secure local file path.
func (c *TelemetryCollector) ExportToFile(destPath string, snapshot DiagnosticSnapshot) error {
	if destPath == "" {
		return errors.New("destination export path cannot be empty")
	}

	dir := filepath.Dir(destPath)
	if err := fileutil.MkdirAll(dir, fileutil.StandardDirPerm); err != nil {
		return fmt.Errorf("failed to create target export directory: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode diagnostic snapshot: %w", err)
	}

	return fileutil.WriteDurableFile(destPath, data, fileutil.SecureFilePerm)
}
