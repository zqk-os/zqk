package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Well-known identifiers for tool telemetry and ambient sensor integration.
const (
	// ToolUtilJobID is the canonical scheduler job ID for ambient tool utilization aggregation.
	ToolUtilJobID = "SCH-tool-util"

	// ToolUtilizationMetricKind is the kernel metric kind identifier for retention and querying.
	ToolUtilizationMetricKind = "tool_utilization_metric"

	// DefaultMaxSensorRecords is the default rolling capacity cap for in-memory records.
	DefaultMaxSensorRecords = 20000
)

// Sentinel errors for tool telemetry sensor operations.
var (
	ErrNilSensor        = errors.New("tool sensor cannot be nil")
	ErrEmptyToolName    = errors.New("tool name cannot be empty")
	ErrEmptyPayloadPath = errors.New("payload path cannot be empty")
)

// specializedToolPrefixes defines known standalone tool names and prefixes
// designed for AST search, token budgeting, and kernel-native operations.
var specializedToolPrefixes = []string{
	"zgrep",
	"read_code",
	"zqk-vet",
}

// specializedSubcommands defines CLI subcommands that qualify as specialized kernel operations.
var specializedSubcommands = []string{
	"grep",
	"read_code",
	"test",
	"object",
	"new",
	"agent",
	"do",
	"docman",
	"workflow",
	"scheduler",
	"system",
}

// fallbackToolNames defines generic fallback tools that bypass token budgets
// and structural indexing (e.g. unbudgeted shell searches).
var fallbackToolNames = []string{
	"grep",
	"egrep",
	"fgrep",
	"rgrep",
	"rg",
	"ripgrep",
	"find",
	"cat",
	"sed",
	"awk",
	"head",
	"tail",
	"go test",
}

// IsSpecializedTool returns true if the tool name belongs to the specialized/kernel-native suite.
func IsSpecializedTool(toolName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	if normalized == "" {
		return false
	}
	for _, prefix := range specializedToolPrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") || strings.HasPrefix(normalized, prefix+"_") {
			return true
		}
	}
	cliName := strings.ToLower(paths.CLIName())
	if normalized == cliName || strings.HasPrefix(normalized, cliName+" ") || strings.HasPrefix(normalized, cliName+"_") {
		return true
	}
	for _, sub := range specializedSubcommands {
		canonUsage := strings.ToLower(paths.CLIUsage(sub))
		if normalized == canonUsage || strings.HasPrefix(normalized, canonUsage+" ") {
			return true
		}
		if strings.HasPrefix(normalized, cliName+" "+sub) || strings.HasPrefix(normalized, cliName+"_"+sub) {
			return true
		}
		// Also match when prefixed by default brand token
		if strings.HasPrefix(normalized, paths.CLICommandNameDefault+" "+sub) || strings.HasPrefix(normalized, paths.CLICommandNameDefault+"_"+sub) {
			return true
		}
	}
	return false
}

// IsFallbackTool returns true if the tool name belongs to generic fallback utilities.
func IsFallbackTool(toolName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	if normalized == "" {
		return false
	}
	if IsSpecializedTool(normalized) {
		return false
	}
	for _, fb := range fallbackToolNames {
		if normalized == fb || strings.HasPrefix(normalized, fb+" ") {
			return true
		}
	}
	return false
}

// ToolExecutionRecord captures point-in-time telemetry for a single tool invocation.
type ToolExecutionRecord struct {
	ToolName      string            `json:"tool_name"`
	Duration      time.Duration     `json:"duration"`
	ExitCode      int               `json:"exit_code"`
	TokensUsed    int               `json:"tokens_used"`
	TokensSaved   int               `json:"tokens_saved"`
	IsSpecialized bool              `json:"is_specialized"`
	IsFallback    bool              `json:"is_fallback"`
	Success       bool              `json:"success"`
	Timestamp     time.Time         `json:"timestamp"`
	AgentID       string            `json:"agent_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	ErrorMessage  string            `json:"error_message,omitempty"`
}

// ToolAggregatedStats holds aggregate utilization indicators for a single tool.
type ToolAggregatedStats struct {
	ToolName         string        `json:"tool_name"`
	TotalInvocations int64         `json:"total_invocations"`
	SuccessCount     int64         `json:"success_count"`
	FailureCount     int64         `json:"failure_count"`
	TotalDuration    time.Duration `json:"total_duration"`
	AverageDuration  time.Duration `json:"average_duration"`
	TotalTokensUsed  int64         `json:"total_tokens_used"`
	TotalTokensSaved int64         `json:"total_tokens_saved"`
	IsSpecialized    bool          `json:"is_specialized"`
	IsFallback       bool          `json:"is_fallback"`
}

// ToolUtilizationMetrics aggregates adoption rates, latency, and token savings across all tool executions.
type ToolUtilizationMetrics struct {
	TotalInvocations                     int64                          `json:"total_invocations"`
	SuccessCount                         int64                          `json:"success_count"`
	FailureCount                         int64                          `json:"failure_count"`
	SuccessRate                          float64                        `json:"success_rate"`
	SpecializedInvocations               int64                          `json:"specialized_invocations"`
	FallbackInvocations                  int64                          `json:"fallback_invocations"`
	SpecializedAdoptionRate              float64                        `json:"specialized_adoption_rate"`
	FallbackRate                         float64                        `json:"fallback_rate"`
	TotalDuration                        time.Duration                  `json:"total_duration"`
	AverageDuration                      time.Duration                  `json:"average_duration"`
	TotalTokensUsed                      int64                          `json:"total_tokens_used"`
	TotalTokensSaved                     int64                          `json:"total_tokens_saved"`
	AverageTokensSavedPerSpecializedCall float64                        `json:"avg_tokens_saved_per_specialized_call"`
	ByTool                               map[string]ToolAggregatedStats `json:"by_tool"`
	GeneratedAt                          time.Time                      `json:"generated_at"`
	Window                               time.Duration                  `json:"window,omitempty"`
}

// SummaryTable returns a human-readable tabular ASCII report of tool utilization metrics.
func (m *ToolUtilizationMetrics) SummaryTable() string {
	if m == nil {
		return "<nil tool metrics>"
	}

	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString("            AMBIENT TOOL UTILIZATION & EFFECTIVENESS REPORT                     \n")
	sb.WriteString("================================================================================\n")
	fmt.Fprintf(&sb, "Generated At:                  %s\n", m.GeneratedAt.UTC().Format(time.RFC3339))
	if m.Window > 0 {
		fmt.Fprintf(&sb, "Window:                        %s\n", m.Window)
	}
	fmt.Fprintf(&sb, "Total Invocations:             %d\n", m.TotalInvocations)
	fmt.Fprintf(&sb, "Success Rate:                  %.1f%% (%d passed, %d failed)\n", m.SuccessRate*100, m.SuccessCount, m.FailureCount)
	fmt.Fprintf(&sb, "Specialized Adoption Rate:     %.1f%% (%d specialized, %d fallback)\n", m.SpecializedAdoptionRate*100, m.SpecializedInvocations, m.FallbackInvocations)
	fmt.Fprintf(&sb, "Fallback Rate:                 %.1f%%\n", m.FallbackRate*100)
	fmt.Fprintf(&sb, "Average Latency:               %s\n", m.AverageDuration.Round(time.Microsecond))
	fmt.Fprintf(&sb, "Total Tokens Used:             %d\n", m.TotalTokensUsed)
	fmt.Fprintf(&sb, "Total Tokens Saved:            %d\n", m.TotalTokensSaved)
	fmt.Fprintf(&sb, "Avg Tokens Saved / Spec Call:  %.1f\n", m.AverageTokensSavedPerSpecializedCall)
	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString("PER-TOOL BREAKDOWN\n")
	if len(m.ByTool) == 0 {
		sb.WriteString("  (no recorded tool executions)\n")
	} else {
		tools := make([]string, 0, len(m.ByTool))
		for t := range m.ByTool {
			tools = append(tools, t)
		}
		sort.Strings(tools)
		sb.WriteString(fmt.Sprintf("  %-20s %-8s %-10s %-12s %-12s %-12s\n", "TOOL", "CALLS", "SUCCESS", "AVG LATENCY", "TOKENS USED", "TOKENS SAVED"))
		for _, name := range tools {
			st := m.ByTool[name]
			sb.WriteString(fmt.Sprintf("  %-20s %-8d %-10s %-12s %-12d %-12d\n",
				st.ToolName,
				st.TotalInvocations,
				fmt.Sprintf("%d/%d", st.SuccessCount, st.TotalInvocations),
				st.AverageDuration.Round(time.Microsecond),
				st.TotalTokensUsed,
				st.TotalTokensSaved,
			))
		}
	}
	sb.WriteString("================================================================================\n")
	return sb.String()
}

// ToJSON serializes the utilization metrics to formatted JSON.
func (m *ToolUtilizationMetrics) ToJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.MarshalIndent(m, "", "  ")
}

// ToolSensor manages concurrent recording and aggregation of tool execution events.
type ToolSensor struct {
	mu         sync.RWMutex
	records    []ToolExecutionRecord
	maxRecords int
}

// NewToolSensor creates a new thread-safe ToolSensor with default capacity.
func NewToolSensor(maxRecords ...int) *ToolSensor {
	capacity := DefaultMaxSensorRecords
	if len(maxRecords) > 0 && maxRecords[0] > 0 {
		capacity = maxRecords[0]
	}
	return &ToolSensor{
		records:    make([]ToolExecutionRecord, 0, 128),
		maxRecords: capacity,
	}
}

// Record appends a single ToolExecutionRecord thread-safely with auto-classification.
func (s *ToolSensor) Record(record ToolExecutionRecord) error {
	if s == nil {
		return ErrNilSensor
	}
	record.ToolName = strings.TrimSpace(record.ToolName)
	if record.ToolName == "" {
		return ErrEmptyToolName
	}

	// Auto-classify specialized vs fallback if not explicitly provided
	if !record.IsSpecialized && !record.IsFallback {
		if IsSpecializedTool(record.ToolName) {
			record.IsSpecialized = true
		} else if IsFallbackTool(record.ToolName) {
			record.IsFallback = true
		}
	}

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Rolling eviction if capacity is reached
	if len(s.records) >= s.maxRecords {
		copy(s.records[0:], s.records[1:])
		s.records[len(s.records)-1] = record
	} else {
		s.records = append(s.records, record)
	}
	return nil
}

// Count returns the number of currently retained records.
func (s *ToolSensor) Count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// Clear removes all retained records.
func (s *ToolSensor) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = s.records[:0]
}

// GetRecords returns a copy of all current records.
func (s *ToolSensor) GetRecords() []ToolExecutionRecord {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ToolExecutionRecord, len(s.records))
	copy(out, s.records)
	return out
}

// Aggregate returns the aggregated metrics across all recorded executions.
func (s *ToolSensor) Aggregate() *ToolUtilizationMetrics {
	if s == nil {
		return &ToolUtilizationMetrics{
			ByTool:      make(map[string]ToolAggregatedStats),
			GeneratedAt: time.Now().UTC(),
		}
	}
	s.mu.RLock()
	records := make([]ToolExecutionRecord, len(s.records))
	copy(records, s.records)
	s.mu.RUnlock()

	return aggregateRecords(records, 0)
}

// AggregateWindow returns metrics for executions occurring within the past duration window.
func (s *ToolSensor) AggregateWindow(window time.Duration) *ToolUtilizationMetrics {
	if s == nil {
		return &ToolUtilizationMetrics{
			ByTool:      make(map[string]ToolAggregatedStats),
			GeneratedAt: time.Now().UTC(),
			Window:      window,
		}
	}
	if window <= 0 {
		return s.Aggregate()
	}

	cutoff := time.Now().UTC().Add(-window)
	s.mu.RLock()
	var filtered []ToolExecutionRecord
	for _, r := range s.records {
		if !r.Timestamp.Before(cutoff) {
			filtered = append(filtered, r)
		}
	}
	s.mu.RUnlock()

	return aggregateRecords(filtered, window)
}

// aggregateRecords computes summary statistics safely with zero division safeguards.
func aggregateRecords(records []ToolExecutionRecord, window time.Duration) *ToolUtilizationMetrics {
	metrics := &ToolUtilizationMetrics{
		ByTool:      make(map[string]ToolAggregatedStats),
		GeneratedAt: time.Now().UTC(),
		Window:      window,
	}

	total := int64(len(records))
	metrics.TotalInvocations = total
	if total == 0 {
		return metrics
	}

	toolStats := make(map[string]*ToolAggregatedStats)

	for _, r := range records {
		if r.Success {
			metrics.SuccessCount++
		} else {
			metrics.FailureCount++
		}

		if r.IsSpecialized {
			metrics.SpecializedInvocations++
		} else if r.IsFallback {
			metrics.FallbackInvocations++
		}

		metrics.TotalDuration += r.Duration
		metrics.TotalTokensUsed += int64(r.TokensUsed)
		metrics.TotalTokensSaved += int64(r.TokensSaved)

		st, exists := toolStats[r.ToolName]
		if !exists {
			st = &ToolAggregatedStats{
				ToolName:      r.ToolName,
				IsSpecialized: r.IsSpecialized,
				IsFallback:    r.IsFallback,
			}
			toolStats[r.ToolName] = st
		}
		st.TotalInvocations++
		if r.Success {
			st.SuccessCount++
		} else {
			st.FailureCount++
		}
		st.TotalDuration += r.Duration
		st.TotalTokensUsed += int64(r.TokensUsed)
		st.TotalTokensSaved += int64(r.TokensSaved)
	}

	metrics.SuccessRate = float64(metrics.SuccessCount) / float64(total)
	metrics.SpecializedAdoptionRate = float64(metrics.SpecializedInvocations) / float64(total)
	metrics.FallbackRate = float64(metrics.FallbackInvocations) / float64(total)
	metrics.AverageDuration = time.Duration(int64(metrics.TotalDuration) / total)

	if metrics.SpecializedInvocations > 0 {
		metrics.AverageTokensSavedPerSpecializedCall = float64(metrics.TotalTokensSaved) / float64(metrics.SpecializedInvocations)
	}

	for name, st := range toolStats {
		if st.TotalInvocations > 0 {
			st.AverageDuration = time.Duration(int64(st.TotalDuration) / st.TotalInvocations)
		}
		metrics.ByTool[name] = *st
	}

	return metrics
}

// Summary returns a formatted summary report of the current sensor state.
func (s *ToolSensor) Summary() string {
	return s.Aggregate().SummaryTable()
}

// SaveToFile serializes all records to disk securely.
func (s *ToolSensor) SaveToFile(path string) error {
	if s == nil {
		return ErrNilSensor
	}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return ErrEmptyPayloadPath
	}

	records := s.GetRecords()
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sensor records: %w", err)
	}

	dir := filepath.Dir(cleanPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return fileutil.WriteFile(cleanPath, data, paths.FilePerm644)
}

// LoadFromFile restores sensor records from disk safely.
func (s *ToolSensor) LoadFromFile(path string) error {
	if s == nil {
		return ErrNilSensor
	}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return ErrEmptyPayloadPath
	}

	data, err := fileutil.ReadFile(cleanPath)
	if err != nil {
		return err
	}

	var records []ToolExecutionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("failed to parse tool records JSON: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = records
	return nil
}

// RunToolUtilizationAggregatorJob executes the SCH-tool-util scheduler aggregator lifecycle,
// calculating current metrics and optionally persisting the result snapshot to outputPath.
func RunToolUtilizationAggregatorJob(ctx context.Context, sensor *ToolSensor, outputPath string) (*ToolUtilizationMetrics, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if sensor == nil {
		return nil, ErrNilSensor
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	metrics := sensor.Aggregate()

	if strings.TrimSpace(outputPath) != "" {
		data, err := metrics.ToJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to encode metrics: %w", err)
		}
		dir := filepath.Dir(outputPath)
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return nil, fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
		if err := fileutil.WriteFile(outputPath, data, paths.FilePerm644); err != nil {
			return nil, fmt.Errorf("failed to write metrics snapshot: %w", err)
		}
	}

	return metrics, nil
}
