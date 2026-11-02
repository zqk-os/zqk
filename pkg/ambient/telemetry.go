package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Canonical telemetry status states for ambient engine health.
const (
	TelemetryStatusHealthy   = "healthy"
	TelemetryStatusDegraded  = "degraded"
	TelemetryStatusStopped   = "stopped"
	TelemetryStatusQuiescent = "quiescent"
	TelemetryStatusCritical  = "critical"
	TelemetryStatusUnknown   = "unknown"

	CircuitBreakerStatusClosed   = "closed"
	CircuitBreakerStatusOpen     = "open"
	CircuitBreakerStatusTripped  = "tripped"
	CircuitBreakerStatusHalfOpen = "half-open"
)

// Named sentinel errors for ambient telemetry file operations and channel boundaries.
var (
	ErrNilTelemetryReport     = errors.New("telemetry report cannot be nil")
	ErrStateFileNotFound      = errors.New("ambient state file does not exist")
	ErrClosedTelemetryChannel = errors.New("telemetry channel is closed")
	ErrTelemetryTimeout       = errors.New("telemetry channel operation timed out")
)

// ComponentHealth represents the health assessment of an individual ambient subsystem component.
type ComponentHealth struct {
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// IsHealthy returns true if the component status is healthy.
func (c ComponentHealth) IsHealthy() bool {
	return strings.EqualFold(c.Status, TelemetryStatusHealthy)
}

// HolonMeshMetrics aggregates operational throughput and convergence indicators
// across the distributed holon execution fabric.
type HolonMeshMetrics struct {
	IngestRate       float64 `json:"ingest_rate"`
	ConvergenceScore float64 `json:"convergence_score"`
	DivergenceCount  int64   `json:"divergence_count"`
	EventThroughput  float64 `json:"event_throughput"`
	StreamLatencyMs  float64 `json:"stream_latency_ms"`
}

// AmbientTelemetryReport represents a comprehensive, point-in-time runtime telemetry
// snapshot of the Ambient Engine, its holons, circuit breaker, and component health.
type AmbientTelemetryReport struct {
	mu                   sync.RWMutex               `json:"-"`
	Timestamp            time.Time                  `json:"timestamp"`
	Status               string                     `json:"status"`
	Uptime               time.Duration              `json:"uptime"`
	ActiveHolonsCount    int                        `json:"active_holons_count"`
	MeshMetrics          HolonMeshMetrics           `json:"mesh_metrics"`
	Components           map[string]ComponentHealth `json:"components"`
	CircuitBreakerStatus string                     `json:"circuit_breaker_status"`
	RunwayWatermark      int64                      `json:"runway_watermark"`
	Warnings             []string                   `json:"warnings,omitempty"`
}

// TelemetryReportOption configures an AmbientTelemetryReport during instantiation.
type TelemetryReportOption func(*AmbientTelemetryReport)

// NewAmbientTelemetryReport initializes an AmbientTelemetryReport with safe defaults
// and applies any provided functional configuration options.
func NewAmbientTelemetryReport(opts ...TelemetryReportOption) *AmbientTelemetryReport {
	report := &AmbientTelemetryReport{
		Timestamp:            time.Now().UTC(),
		Status:               TelemetryStatusHealthy,
		Components:           make(map[string]ComponentHealth),
		CircuitBreakerStatus: CircuitBreakerStatusClosed,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(report)
		}
	}
	return report
}

// WithStatus sets the overall status of the report.
func WithStatus(status string) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.Status = status
	}
}

// WithUptime sets the engine uptime duration.
func WithUptime(uptime time.Duration) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.Uptime = uptime
	}
}

// WithActiveHolonsCount sets the count of active holons.
func WithActiveHolonsCount(count int) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.ActiveHolonsCount = count
	}
}

// WithMeshMetrics sets the holon mesh metrics.
func WithMeshMetrics(metrics HolonMeshMetrics) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.MeshMetrics = metrics
	}
}

// WithComponents sets the full components health map.
func WithComponents(components map[string]ComponentHealth) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		if components == nil {
			r.Components = make(map[string]ComponentHealth)
			return
		}
		r.Components = make(map[string]ComponentHealth, len(components))
		for k, v := range components {
			r.Components[k] = v
		}
	}
}

// WithComponent registers or updates an individual component's health status.
func WithComponent(comp ComponentHealth) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		if r.Components == nil {
			r.Components = make(map[string]ComponentHealth)
		}
		r.Components[comp.Name] = comp
	}
}

// WithCircuitBreakerStatus sets the circuit breaker state.
func WithCircuitBreakerStatus(status string) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.CircuitBreakerStatus = status
	}
}

// WithRunwayWatermark sets the runway watermark index.
func WithRunwayWatermark(watermark int64) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.RunwayWatermark = watermark
	}
}

// WithTimestamp sets the timestamp on the report.
func WithTimestamp(ts time.Time) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.Timestamp = ts
	}
}

// WithWarnings sets the warnings list on the report.
func WithWarnings(warnings ...string) TelemetryReportOption {
	return func(r *AmbientTelemetryReport) {
		r.Warnings = append([]string(nil), warnings...)
	}
}

// IsHealthy returns true if the report is non-nil, its status is healthy,
// the circuit breaker is closed/normal, no component is degraded or stopped,
// and there are zero warnings.
func (r *AmbientTelemetryReport) IsHealthy() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !strings.EqualFold(r.Status, TelemetryStatusHealthy) {
		return false
	}
	if strings.EqualFold(r.CircuitBreakerStatus, CircuitBreakerStatusOpen) ||
		strings.EqualFold(r.CircuitBreakerStatus, CircuitBreakerStatusTripped) {
		return false
	}
	if len(r.Warnings) > 0 {
		return false
	}
	for _, comp := range r.Components {
		if strings.EqualFold(comp.Status, TelemetryStatusDegraded) ||
			strings.EqualFold(comp.Status, TelemetryStatusStopped) ||
			strings.EqualFold(comp.Status, TelemetryStatusCritical) ||
			strings.EqualFold(comp.Status, "unhealthy") ||
			strings.EqualFold(comp.Status, "error") ||
			strings.EqualFold(comp.Status, "down") {
			return false
		}
	}
	return true
}

// ToJSON safely serializes the telemetry report into standard JSON.
// Handles nil receivers cleanly by returning []byte("null"), nil.
func (r *AmbientTelemetryReport) ToJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r)
}

// MarshalJSON provides thread-safe custom JSON serialization.
func (r *AmbientTelemetryReport) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	type Alias AmbientTelemetryReport
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	})
}

// UnmarshalJSON provides thread-safe JSON deserialization, supporting both
// numeric nanoseconds and string duration representations for Uptime.
func (r *AmbientTelemetryReport) UnmarshalJSON(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	type Alias AmbientTelemetryReport
	aux := struct {
		RawUptime any `json:"uptime"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.RawUptime.(type) {
	case float64:
		r.Uptime = time.Duration(v)
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			r.Uptime = d
		}
	}

	if r.Components == nil {
		r.Components = make(map[string]ComponentHealth)
	}
	return nil
}

// FromJSON decodes a JSON byte slice into an AmbientTelemetryReport pointer.
func FromJSON(data []byte) (*AmbientTelemetryReport, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var report AmbientTelemetryReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// GetComponent returns the component health by name in a thread-safe and nil-safe manner.
func (r *AmbientTelemetryReport) GetComponent(name string) (ComponentHealth, bool) {
	if r == nil {
		return ComponentHealth{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Components == nil {
		return ComponentHealth{}, false
	}
	comp, ok := r.Components[name]
	return comp, ok
}

// SetComponent safely adds or updates a component's health status.
func (r *AmbientTelemetryReport) SetComponent(comp ComponentHealth) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Components == nil {
		r.Components = make(map[string]ComponentHealth)
	}
	r.Components[comp.Name] = comp
}

// GetStatus returns the report status string in a nil-safe manner.
func (r *AmbientTelemetryReport) GetStatus() string {
	if r == nil {
		return TelemetryStatusStopped
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Status
}

// SetStatus updates the report status string in a thread-safe and nil-safe manner.
func (r *AmbientTelemetryReport) SetStatus(status string) {
	if r != nil {
		r.mu.Lock()
		r.Status = status
		r.mu.Unlock()
	}
}

// GetActiveHolonsCount returns the active holon count in a nil-safe manner.
func (r *AmbientTelemetryReport) GetActiveHolonsCount() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	count := r.ActiveHolonsCount
	r.mu.RUnlock()
	return count
}

// GetCircuitBreakerStatus returns the circuit breaker status in a nil-safe manner.
func (r *AmbientTelemetryReport) GetCircuitBreakerStatus() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.CircuitBreakerStatus
}

// GetRunwayWatermark returns the runway watermark index in a nil-safe manner.
func (r *AmbientTelemetryReport) GetRunwayWatermark() int64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	wm := r.RunwayWatermark
	r.mu.RUnlock()
	return wm
}

// GetMeshMetrics returns the holon mesh metrics in a nil-safe manner.
func (r *AmbientTelemetryReport) GetMeshMetrics() HolonMeshMetrics {
	if r == nil {
		return HolonMeshMetrics{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.MeshMetrics
}

// GetWarnings returns a copy of the warnings slice in a thread-safe and nil-safe manner.
func (r *AmbientTelemetryReport) GetWarnings() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.Warnings) == 0 {
		return nil
	}
	out := make([]string, len(r.Warnings))
	copy(out, r.Warnings)
	return out
}

// AddWarning appends a warning to the report in a thread-safe and nil-safe manner.
func (r *AmbientTelemetryReport) AddWarning(warning string) {
	if r == nil || strings.TrimSpace(warning) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Warnings = append(r.Warnings, strings.TrimSpace(warning))
}

// Clone creates an isolated deep copy of the report.
func (r *AmbientTelemetryReport) Clone() *AmbientTelemetryReport {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	cloned := &AmbientTelemetryReport{
		Timestamp:            r.Timestamp,
		Status:               r.Status,
		Uptime:               r.Uptime,
		ActiveHolonsCount:    r.ActiveHolonsCount,
		MeshMetrics:          r.MeshMetrics,
		Components:           make(map[string]ComponentHealth, len(r.Components)),
		CircuitBreakerStatus: r.CircuitBreakerStatus,
		RunwayWatermark:      r.RunwayWatermark,
		Warnings:             append([]string(nil), r.Warnings...),
	}
	for k, v := range r.Components {
		cloned.Components[k] = v
	}
	r.mu.RUnlock()
	return cloned
}

// SummaryTable returns a human-readable, deterministic tabular overview of the telemetry report.
// Nil receivers return an informative fallback placeholder without panicking.
func (r *AmbientTelemetryReport) SummaryTable() string {
	if r == nil {
		return "<nil report>"
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString("                         AMBIENT RUNTIME TELEMETRY REPORT                       \n")
	sb.WriteString("================================================================================\n")
	fmt.Fprintf(&sb, "Status:                 %s\n", r.Status)
	fmt.Fprintf(&sb, "Timestamp:              %s\n", r.Timestamp.UTC().Format(time.RFC3339))
	fmt.Fprintf(&sb, "Uptime:                 %s\n", r.Uptime.Round(time.Millisecond))
	fmt.Fprintf(&sb, "Active Holons:          %d\n", r.ActiveHolonsCount)
	fmt.Fprintf(&sb, "Circuit Breaker:        %s\n", r.CircuitBreakerStatus)
	fmt.Fprintf(&sb, "Runway Watermark:       %d\n", r.RunwayWatermark)
	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString("MESH METRICS\n")
	fmt.Fprintf(&sb, "  Ingest Rate:          %.2f ops/sec\n", r.MeshMetrics.IngestRate)
	fmt.Fprintf(&sb, "  Convergence Score:    %.2f%%\n", r.MeshMetrics.ConvergenceScore)
	fmt.Fprintf(&sb, "  Divergence Count:     %d\n", r.MeshMetrics.DivergenceCount)
	fmt.Fprintf(&sb, "  Event Throughput:     %.2f events/sec\n", r.MeshMetrics.EventThroughput)
	fmt.Fprintf(&sb, "  Stream Latency:       %.2f ms\n", r.MeshMetrics.StreamLatencyMs)
	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString("COMPONENTS\n")
	if len(r.Components) == 0 {
		sb.WriteString("  (no registered components)\n")
	} else {
		names := make([]string, 0, len(r.Components))
		for name := range r.Components {
			names = append(names, name)
		}
		sort.Strings(names)
		sb.WriteString(fmt.Sprintf("  %-25s %-12s %-25s %s\n", "NAME", "STATUS", "LAST HEARTBEAT", "MESSAGE"))
		for _, name := range names {
			comp := r.Components[name]
			hbStr := "-"
			if !comp.LastHeartbeat.IsZero() {
				hbStr = comp.LastHeartbeat.UTC().Format("15:04:05.000")
			}
			msg := comp.Message
			if msg == "" {
				msg = "-"
			}
			sb.WriteString(fmt.Sprintf("  %-25s %-12s %-25s %s\n", comp.Name, comp.Status, hbStr, msg))
		}
	}
	if len(r.Warnings) > 0 {
		sb.WriteString("--------------------------------------------------------------------------------\n")
		sb.WriteString("WARNINGS\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&sb, "  ! %s\n", w)
		}
	}
	sb.WriteString("================================================================================\n")
	return sb.String()
}

// DegradedTelemetryReport constructs an AmbientTelemetryReport representing a degraded
// or fallback runtime state with appropriate circuit breaker and warning diagnostics.
func DegradedTelemetryReport(status string, reason string, warnings ...string) *AmbientTelemetryReport {
	effectiveStatus := strings.TrimSpace(status)
	if effectiveStatus == "" {
		effectiveStatus = TelemetryStatusDegraded
	}

	cbStatus := CircuitBreakerStatusTripped

	var allWarnings []string
	if strings.TrimSpace(reason) != "" {
		allWarnings = append(allWarnings, strings.TrimSpace(reason))
	}
	for _, w := range warnings {
		if tw := strings.TrimSpace(w); tw != "" {
			allWarnings = append(allWarnings, tw)
		}
	}

	return &AmbientTelemetryReport{
		Timestamp:            time.Now().UTC(),
		Status:               effectiveStatus,
		Components:           make(map[string]ComponentHealth),
		CircuitBreakerStatus: cbStatus,
		Warnings:             allWarnings,
	}
}

// SafeLoadTelemetryReport safely reads and deserializes an AmbientTelemetryReport from disk.
// If the path is empty, it returns ErrStatePathRequired and a degraded report.
// If the file does not exist, it returns a degraded report with status "stopped" and ErrStateFileNotFound.
// If the file is empty, it returns ErrEmptyAmbientState and a degraded report with warnings.
// If the file is corrupt or truncated JSON, it returns ErrCorruptAmbientState and a degraded report with warnings.
// This function guarantees zero panics under all invalid, missing, or corrupted input states.
func SafeLoadTelemetryReport(path string) (*AmbientTelemetryReport, error) {
	if strings.TrimSpace(path) == "" {
		report := DegradedTelemetryReport(TelemetryStatusDegraded, "ambient state file path is empty", "empty file path provided")
		return report, ErrStatePathRequired
	}

	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			reason := fmt.Sprintf("state file does not exist: %s", path)
			report := DegradedTelemetryReport(TelemetryStatusStopped, reason, "ambient daemon is offline or state file not written")
			return report, fmt.Errorf("%w: %s", ErrStateFileNotFound, path)
		}
		reason := fmt.Sprintf("failed to read state file: %v", err)
		report := DegradedTelemetryReport(TelemetryStatusDegraded, reason, err.Error())
		return report, fmt.Errorf("%w: %s: %v", ErrCorruptAmbientState, path, err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		reason := fmt.Sprintf("state file %q is empty", path)
		report := DegradedTelemetryReport(TelemetryStatusDegraded, reason, "state file contained zero bytes")
		return report, fmt.Errorf("%w: state file %q is empty", ErrEmptyAmbientState, path)
	}

	var report AmbientTelemetryReport
	if err := json.Unmarshal(data, &report); err != nil {
		reason := fmt.Sprintf("failed to parse telemetry json from %s: %v", path, err)
		degraded := DegradedTelemetryReport(TelemetryStatusDegraded, reason, fmt.Sprintf("corrupt JSON payload: %v", err))
		return degraded, fmt.Errorf("%w: %s: %v", ErrCorruptAmbientState, path, err)
	}

	if report.Components == nil {
		report.Components = make(map[string]ComponentHealth)
	}
	if report.Status == "" {
		report.Status = TelemetryStatusUnknown
	}
	if report.CircuitBreakerStatus == "" {
		report.CircuitBreakerStatus = CircuitBreakerStatusClosed
	}

	return &report, nil
}

// SaveTelemetryReport persists an AmbientTelemetryReport to disk securely as formatted JSON.
func SaveTelemetryReport(path string, report *AmbientTelemetryReport) error {
	if strings.TrimSpace(path) == "" {
		return ErrStatePathRequired
	}
	if report == nil {
		return ErrNilTelemetryReport
	}
	return fileutil.WriteSecureJSONIndent(path, report)
}

func validateChannelAndReport(ch chan<- *AmbientTelemetryReport, report *AmbientTelemetryReport) error {
	if ch == nil {
		return ErrNilTelemetryChannel
	}
	if report == nil {
		return ErrNilTelemetryReport
	}
	return nil
}

// EmitTelemetryReport attempts to emit an AmbientTelemetryReport to a channel non-blockingly.
func EmitTelemetryReport(ctx context.Context, ch chan<- *AmbientTelemetryReport, report *AmbientTelemetryReport) error {
	if err := validateChannelAndReport(ch, report); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	select {
	case ch <- report:
		return nil
	default:
		return ErrTelemetryChannelFull
	}
}

// EmitTelemetryReportWithTimeout attempts to emit an AmbientTelemetryReport to ch within the given timeout.
func EmitTelemetryReportWithTimeout(ctx context.Context, ch chan<- *AmbientTelemetryReport, report *AmbientTelemetryReport, timeout time.Duration) error {
	if err := validateChannelAndReport(ch, report); err != nil {
		return err
	}
	if timeout <= 0 {
		return EmitTelemetryReport(ctx, ch, report)
	}

	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ch <- report:
		return nil
	case <-doneCh:
		return ctx.Err()
	case <-timer.C:
		return ErrTelemetryTimeout
	}
}

// SafeEmitTelemetryReport emits a report to ch, recovering gracefully from any panic if the channel was closed.
func SafeEmitTelemetryReport(ctx context.Context, ch chan<- *AmbientTelemetryReport, report *AmbientTelemetryReport, timeout time.Duration) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrClosedTelemetryChannel, r)
		}
	}()
	return EmitTelemetryReportWithTimeout(ctx, ch, report, timeout)
}
