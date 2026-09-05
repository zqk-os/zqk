package mcp

import (
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// MCPMetrics tracks comprehensive metrics for MCP protocol operations
type MCPMetrics struct {
	// Lifecycle metrics
	InitializeCount    atomic.Int64
	InitializeDuration *DurationHistogram
	InitializeErrors   atomic.Int64
	ShutdownCount      atomic.Int64
	ShutdownDuration   *DurationHistogram

	// Tools metrics
	ToolsListCount    atomic.Int64
	ToolsListDuration *DurationHistogram
	ToolsListErrors   atomic.Int64
	ToolCallCount     atomic.Int64
	ToolCallDuration  *DurationHistogram
	ToolCallErrors    atomic.Int64
	ToolCallByTool    map[string]*ToolMetrics

	// Resources metrics
	ResourcesListCount    atomic.Int64
	ResourcesListDuration *DurationHistogram
	ResourcesListErrors   atomic.Int64
	ResourceGetCount      atomic.Int64
	ResourceGetDuration   *DurationHistogram
	ResourceGetErrors     atomic.Int64

	// Prompts metrics
	PromptsListCount    atomic.Int64
	PromptsListDuration *DurationHistogram
	PromptsListErrors   atomic.Int64
	PromptGetCount      atomic.Int64
	PromptGetDuration   *DurationHistogram
	PromptGetErrors     atomic.Int64

	// Roots metrics
	RootsListCount    atomic.Int64
	RootsListDuration *DurationHistogram
	RootsListErrors   atomic.Int64

	// Notification metrics
	LogMessageCount   atomic.Int64
	LogMessageDropped atomic.Int64
	EventCount        atomic.Int64
	MessageCount      atomic.Int64

	// Async operation metrics
	AsyncToolCallCount  atomic.Int64
	AsyncToolCallErrors atomic.Int64
	BatchToolCallCount  atomic.Int64
	BatchToolCallSize   *SizeHistogram
	BatchToolCallErrors atomic.Int64

	// Queue metrics
	QueueDepth   atomic.Int64
	QueueDropped atomic.Int64
	QueueSent    atomic.Int64
	QueueErrors  atomic.Int64

	// Concurrency metrics
	ConcurrentOperations atomic.Int64
	MaxConcurrentOps     atomic.Int64

	mu sync.RWMutex
}

// ToolMetrics tracks metrics for individual tools using lock-free atomic counters
type ToolMetrics struct {
	CallCount     atomic.Int64
	TotalDuration atomic.Int64 // nanoseconds
	ErrorCount    atomic.Int64
	LastCalled    atomic.Int64 // unix nanoseconds
}

// DurationHistogram tracks duration distributions
type DurationHistogram struct {
	Count      int64
	Total      int64   // nanoseconds
	Min        int64   // nanoseconds
	Max        int64   // nanoseconds
	P50        int64   // nanoseconds (approximate)
	P95        int64   // nanoseconds (approximate)
	P99        int64   // nanoseconds (approximate)
	samples    []int64 // For percentile calculation
	maxSamples int
	mu         sync.RWMutex
}

// SizeHistogram tracks size distributions
type SizeHistogram struct {
	Count      int64
	Total      int64
	Min        int64
	Max        int64
	samples    []int64
	maxSamples int
	mu         sync.RWMutex
}

// NewMCPMetrics creates a new metrics collector
func NewMCPMetrics() *MCPMetrics {
	return &MCPMetrics{
		InitializeDuration:    NewDurationHistogram(1000),
		ShutdownDuration:      NewDurationHistogram(100),
		ToolsListDuration:     NewDurationHistogram(1000),
		ToolCallDuration:      NewDurationHistogram(10000),
		ResourcesListDuration: NewDurationHistogram(1000),
		ResourceGetDuration:   NewDurationHistogram(10000),
		PromptsListDuration:   NewDurationHistogram(1000),
		PromptGetDuration:     NewDurationHistogram(1000),
		RootsListDuration:     NewDurationHistogram(1000),
		ToolCallByTool:        make(map[string]*ToolMetrics),
		BatchToolCallSize:     NewSizeHistogram(1000),
	}
}

// NewDurationHistogram creates a new duration histogram
func NewDurationHistogram(maxSamples int) *DurationHistogram {
	return &DurationHistogram{
		Min:        int64(^uint64(0) >> 1), // Max int64
		Max:        0,
		maxSamples: maxSamples,
		samples:    make([]int64, 0, maxSamples),
	}
}

// NewSizeHistogram creates a new size histogram
func NewSizeHistogram(maxSamples int) *SizeHistogram {
	return &SizeHistogram{
		Min:        int64(^uint64(0) >> 1),
		Max:        0,
		maxSamples: maxSamples,
		samples:    make([]int64, 0, maxSamples),
	}
}

// RecordDuration records a duration measurement
func (h *DurationHistogram) RecordDuration(duration time.Duration) {
	ns := duration.Nanoseconds()

	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameDurationHistogramRecord, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.Count++
			h.Total += ns

			if ns < h.Min {
				h.Min = ns
			}
			if ns > h.Max {
				h.Max = ns
			}

			// Store sample for percentile calculation
			if len(h.samples) < h.maxSamples {
				h.samples = append(h.samples, ns)
			} else {
				// Replace random sample (simple round-robin)
				idx := int(h.Count-1) % h.maxSamples
				h.samples[idx] = ns
			}

			// Calculate approximate percentiles
			if len(h.samples) > 0 {
				h.calculatePercentiles()
			}
			return nil
		},
	)
}

// RecordSize records a size measurement
func (h *SizeHistogram) RecordSize(size int64) {
	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameSizeHistogramRecord, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.Count++
			h.Total += size

			if size < h.Min {
				h.Min = size
			}
			if size > h.Max {
				h.Max = size
			}

			if len(h.samples) < h.maxSamples {
				h.samples = append(h.samples, size)
			}
			return nil
		},
	)
}

// calculatePercentiles calculates approximate percentiles
func (h *DurationHistogram) calculatePercentiles() {
	if len(h.samples) == 0 {
		return
	}

	// Simple percentile calculation (not exact, but good enough for metrics)
	sorted := make([]int64, len(h.samples))
	copy(sorted, h.samples)

	// Simple insertion sort (samples are small, so this is fine)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Calculate percentiles
	if len(sorted) > 0 {
		h.P50 = sorted[len(sorted)*50/100]
		if len(sorted) > 1 {
			h.P95 = sorted[len(sorted)*95/100]
			h.P99 = sorted[len(sorted)*99/100]
		}
	}
}

// GetStats returns histogram statistics
func (h *DurationHistogram) GetStats() DurationStats {
	var stats DurationStats
	_ = concurrency.RunInRLockWithLogger(
		&h.mu, LockNameDurationHistogramGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			avg := int64(0)
			if h.Count > 0 {
				avg = h.Total / h.Count
			}

			stats = DurationStats{
				Count:   h.Count,
				Total:   time.Duration(h.Total),
				Average: time.Duration(avg),
				Min:     time.Duration(h.Min),
				Max:     time.Duration(h.Max),
				P50:     time.Duration(h.P50),
				P95:     time.Duration(h.P95),
				P99:     time.Duration(h.P99),
			}
			return nil
		},
	)
	return stats
}

// DurationStats represents duration statistics
type DurationStats struct {
	Count   int64
	Total   time.Duration
	Average time.Duration
	Min     time.Duration
	Max     time.Duration
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
}

// RecordInitialize records an initialize operation
func (m *MCPMetrics) RecordInitialize(duration time.Duration, err error) {
	m.InitializeCount.Add(1)
	m.InitializeDuration.RecordDuration(duration)
	if err != nil {
		m.InitializeErrors.Add(1)
	}
}

// RecordShutdown records a shutdown operation
func (m *MCPMetrics) RecordShutdown(duration time.Duration) {
	m.ShutdownCount.Add(1)
	m.ShutdownDuration.RecordDuration(duration)
}

// RecordToolsList records a tools/list operation
func (m *MCPMetrics) RecordToolsList(duration time.Duration, err error) {
	m.ToolsListCount.Add(1)
	m.ToolsListDuration.RecordDuration(duration)
	if err != nil {
		m.ToolsListErrors.Add(1)
	}
}

// RecordToolCall records a tool call operation
func (m *MCPMetrics) RecordToolCall(toolName string, duration time.Duration, err error) {
	m.ToolCallCount.Add(1)
	m.ToolCallDuration.RecordDuration(duration)
	if err != nil {
		m.ToolCallErrors.Add(1)
	}

	// Record per-tool metrics
	var toolMetrics *ToolMetrics
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMcpMetricsRecordToolGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var exists bool
			toolMetrics, exists = m.ToolCallByTool[toolName]
			if !exists {
				toolMetrics = &ToolMetrics{}
				m.ToolCallByTool[toolName] = toolMetrics
			}
			return nil
		},
	)

	// Update tool metrics lock-free
	toolMetrics.CallCount.Add(1)
	toolMetrics.TotalDuration.Add(duration.Nanoseconds())
	if err != nil {
		toolMetrics.ErrorCount.Add(1)
	}
	toolMetrics.LastCalled.Store(time.Now().UnixNano())
}

// RegisterUncalledToolMetrics initializes a ToolMetrics entry for a registered tool before any calls occur.
func (m *MCPMetrics) RegisterUncalledToolMetrics(toolName string) {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMcpMetricsRecordToolGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if _, exists := m.ToolCallByTool[toolName]; !exists {
				m.ToolCallByTool[toolName] = &ToolMetrics{}
			}
			return nil
		},
	)
}

// RecordResourcesList records a resources/list operation
func (m *MCPMetrics) RecordResourcesList(duration time.Duration, err error) {
	m.ResourcesListCount.Add(1)
	m.ResourcesListDuration.RecordDuration(duration)
	if err != nil {
		m.ResourcesListErrors.Add(1)
	}
}

// RecordResourceGet records a resources/get operation
func (m *MCPMetrics) RecordResourceGet(duration time.Duration, err error) {
	m.ResourceGetCount.Add(1)
	m.ResourceGetDuration.RecordDuration(duration)
	if err != nil {
		m.ResourceGetErrors.Add(1)
	}
}

// RecordPromptsList records a prompts/list operation
func (m *MCPMetrics) RecordPromptsList(duration time.Duration, err error) {
	m.PromptsListCount.Add(1)
	m.PromptsListDuration.RecordDuration(duration)
	if err != nil {
		m.PromptsListErrors.Add(1)
	}
}

// RecordPromptGet records a prompts/get operation
func (m *MCPMetrics) RecordPromptGet(duration time.Duration, err error) {
	m.PromptGetCount.Add(1)
	m.PromptGetDuration.RecordDuration(duration)
	if err != nil {
		m.PromptGetErrors.Add(1)
	}
}

// RecordRootsList records a roots/list operation
func (m *MCPMetrics) RecordRootsList(duration time.Duration, err error) {
	m.RootsListCount.Add(1)
	m.RootsListDuration.RecordDuration(duration)
	if err != nil {
		m.RootsListErrors.Add(1)
	}
}

// RecordLogMessage records a log message notification
func (m *MCPMetrics) RecordLogMessage(dropped bool) {
	m.LogMessageCount.Add(1)
	if dropped {
		m.LogMessageDropped.Add(1)
	}
}

// RecordEvent records an event notification
func (m *MCPMetrics) RecordEvent() {
	m.EventCount.Add(1)
}

// RecordMessage records a message notification
func (m *MCPMetrics) RecordMessage() {
	m.MessageCount.Add(1)
}

// RecordAsyncToolCall records an async tool call
func (m *MCPMetrics) RecordAsyncToolCall(err error) {
	m.AsyncToolCallCount.Add(1)
	if err != nil {
		m.AsyncToolCallErrors.Add(1)
	}
}

// RecordBatchToolCall records a batch tool call
func (m *MCPMetrics) RecordBatchToolCall(size int, errors int) {
	m.BatchToolCallCount.Add(1)
	m.BatchToolCallSize.RecordSize(int64(size))
	if errors > 0 {
		m.BatchToolCallErrors.Add(int64(errors))
	}
}

// RecordQueueMetrics records queue metrics
func (m *MCPMetrics) RecordQueueMetrics(depth, dropped, sent, errors int64) {
	m.QueueDepth.Store(depth)
	m.QueueDropped.Add(dropped)
	m.QueueSent.Add(sent)
	m.QueueErrors.Add(errors)
}

// RecordConcurrentOperation records concurrent operation metrics
func (m *MCPMetrics) RecordConcurrentOperation(delta int64) {
	current := m.ConcurrentOperations.Add(delta)
	max := m.MaxConcurrentOps.Load()
	if current > max {
		m.MaxConcurrentOps.CompareAndSwap(max, current)
	}
}

// GetSnapshot returns a snapshot of all metrics
func (m *MCPMetrics) GetSnapshot() MetricsSnapshot {
	var toolMetrics map[string]ToolMetricsSnapshot
	var toolCallByTool map[string]*ToolMetrics
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameMcpMetricsGetSnapshotCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			toolCallByTool = make(map[string]*ToolMetrics)
			for name, tm := range m.ToolCallByTool {
				toolCallByTool[name] = tm
			}
			return nil
		},
	)

	// Get per-tool metrics lock-free
	toolMetrics = make(map[string]ToolMetricsSnapshot)
	for name, tm := range toolCallByTool {
		callCount := tm.CallCount.Load()
		totalDuration := tm.TotalDuration.Load()
		errorCount := tm.ErrorCount.Load()
		lastCalledNano := tm.LastCalled.Load()
		var lastCalled time.Time
		if lastCalledNano > 0 {
			lastCalled = time.Unix(0, lastCalledNano)
		}

		avgDuration := time.Duration(0)
		if callCount > 0 {
			avgDuration = time.Duration(totalDuration / callCount)
		}
		toolMetrics[name] = ToolMetricsSnapshot{
			CallCount:       callCount,
			AverageDuration: avgDuration,
			ErrorCount:      errorCount,
			LastCalled:      lastCalled,
		}
	}

	return MetricsSnapshot{
		Lifecycle: LifecycleMetrics{
			InitializeCount:    m.InitializeCount.Load(),
			InitializeDuration: m.InitializeDuration.GetStats(),
			InitializeErrors:   m.InitializeErrors.Load(),
			ShutdownCount:      m.ShutdownCount.Load(),
			ShutdownDuration:   m.ShutdownDuration.GetStats(),
		},
		Tools: ToolsMetrics{
			ListCount:    m.ToolsListCount.Load(),
			ListDuration: m.ToolsListDuration.GetStats(),
			ListErrors:   m.ToolsListErrors.Load(),
			CallCount:    m.ToolCallCount.Load(),
			CallDuration: m.ToolCallDuration.GetStats(),
			CallErrors:   m.ToolCallErrors.Load(),
			ByTool:       toolMetrics,
		},
		Resources: ResourcesMetrics{
			ListCount:    m.ResourcesListCount.Load(),
			ListDuration: m.ResourcesListDuration.GetStats(),
			ListErrors:   m.ResourcesListErrors.Load(),
			GetCount:     m.ResourceGetCount.Load(),
			GetDuration:  m.ResourceGetDuration.GetStats(),
			GetErrors:    m.ResourceGetErrors.Load(),
		},
		Prompts: PromptsMetrics{
			ListCount:    m.PromptsListCount.Load(),
			ListDuration: m.PromptsListDuration.GetStats(),
			ListErrors:   m.PromptsListErrors.Load(),
			GetCount:     m.PromptGetCount.Load(),
			GetDuration:  m.PromptGetDuration.GetStats(),
			GetErrors:    m.PromptGetErrors.Load(),
		},
		Roots: RootsMetrics{
			ListCount:    m.RootsListCount.Load(),
			ListDuration: m.RootsListDuration.GetStats(),
			ListErrors:   m.RootsListErrors.Load(),
		},
		Notifications: NotificationsMetrics{
			LogMessageCount:   m.LogMessageCount.Load(),
			LogMessageDropped: m.LogMessageDropped.Load(),
			EventCount:        m.EventCount.Load(),
			MessageCount:      m.MessageCount.Load(),
		},
		Async: AsyncMetrics{
			ToolCallCount:   m.AsyncToolCallCount.Load(),
			ToolCallErrors:  m.AsyncToolCallErrors.Load(),
			BatchCallCount:  m.BatchToolCallCount.Load(),
			BatchCallErrors: m.BatchToolCallErrors.Load(),
		},
		Queue: QueueMetrics{
			Depth:   m.QueueDepth.Load(),
			Dropped: m.QueueDropped.Load(),
			Sent:    m.QueueSent.Load(),
			Errors:  m.QueueErrors.Load(),
		},
		Concurrency: ConcurrencyMetrics{
			Current: m.ConcurrentOperations.Load(),
			Max:     m.MaxConcurrentOps.Load(),
		},
		Timestamp: time.Now(),
	}
}

// MetricsSnapshot represents a snapshot of all metrics
type MetricsSnapshot struct {
	Lifecycle     LifecycleMetrics
	Tools         ToolsMetrics
	Resources     ResourcesMetrics
	Prompts       PromptsMetrics
	Roots         RootsMetrics
	Notifications NotificationsMetrics
	Async         AsyncMetrics
	Queue         QueueMetrics
	Concurrency   ConcurrencyMetrics
	Timestamp     time.Time
}

// LifecycleMetrics represents lifecycle operation metrics
type LifecycleMetrics struct {
	InitializeCount    int64
	InitializeDuration DurationStats
	InitializeErrors   int64
	ShutdownCount      int64
	ShutdownDuration   DurationStats
}

// ToolsMetrics represents tools operation metrics
type ToolsMetrics struct {
	ListCount    int64
	ListDuration DurationStats
	ListErrors   int64
	CallCount    int64
	CallDuration DurationStats
	CallErrors   int64
	ByTool       map[string]ToolMetricsSnapshot
}

// ToolMetricsSnapshot represents per-tool metrics
type ToolMetricsSnapshot struct {
	CallCount       int64
	AverageDuration time.Duration
	ErrorCount      int64
	LastCalled      time.Time
}

// ResourcesMetrics represents resources operation metrics
type ResourcesMetrics struct {
	ListCount    int64
	ListDuration DurationStats
	ListErrors   int64
	GetCount     int64
	GetDuration  DurationStats
	GetErrors    int64
}

// PromptsMetrics represents prompts operation metrics
type PromptsMetrics struct {
	ListCount    int64
	ListDuration DurationStats
	ListErrors   int64
	GetCount     int64
	GetDuration  DurationStats
	GetErrors    int64
}

// RootsMetrics represents roots operation metrics
type RootsMetrics struct {
	ListCount    int64
	ListDuration DurationStats
	ListErrors   int64
}

// NotificationsMetrics represents notification metrics
type NotificationsMetrics struct {
	LogMessageCount   int64
	LogMessageDropped int64
	EventCount        int64
	MessageCount      int64
}

// AsyncMetrics represents async operation metrics
type AsyncMetrics struct {
	ToolCallCount   int64
	ToolCallErrors  int64
	BatchCallCount  int64
	BatchCallErrors int64
}

// QueueMetrics represents queue metrics
type QueueMetrics struct {
	Depth   int64
	Dropped int64
	Sent    int64
	Errors  int64
}

// ConcurrencyMetrics represents concurrency metrics
type ConcurrencyMetrics struct {
	Current int64
	Max     int64
}

// Reset resets all metrics (useful for testing)
func (m *MCPMetrics) Reset() {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMcpMetricsReset, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.InitializeErrors.Store(0)
			m.ShutdownCount.Store(0)
			m.ToolsListCount.Store(0)
			m.ToolsListErrors.Store(0)
			m.ToolCallCount.Store(0)
			m.ToolCallErrors.Store(0)
			m.ResourcesListCount.Store(0)
			m.ResourcesListErrors.Store(0)
			m.ResourceGetCount.Store(0)
			m.ResourceGetErrors.Store(0)
			m.PromptsListCount.Store(0)
			m.PromptsListErrors.Store(0)
			m.PromptGetCount.Store(0)
			m.PromptGetErrors.Store(0)
			m.RootsListCount.Store(0)
			m.RootsListErrors.Store(0)
			m.LogMessageCount.Store(0)
			m.LogMessageDropped.Store(0)
			m.EventCount.Store(0)
			m.MessageCount.Store(0)
			m.AsyncToolCallCount.Store(0)
			m.AsyncToolCallErrors.Store(0)
			m.BatchToolCallCount.Store(0)
			m.BatchToolCallErrors.Store(0)
			m.QueueDepth.Store(0)
			m.QueueDropped.Store(0)
			m.QueueSent.Store(0)
			m.QueueErrors.Store(0)
			m.ConcurrentOperations.Store(0)
			m.MaxConcurrentOps.Store(0)

			m.ToolCallByTool = make(map[string]*ToolMetrics)

			// Reset histograms
			m.InitializeDuration = NewDurationHistogram(1000)
			m.ShutdownDuration = NewDurationHistogram(100)
			m.ToolsListDuration = NewDurationHistogram(1000)
			m.ToolCallDuration = NewDurationHistogram(10000)
			m.ResourcesListDuration = NewDurationHistogram(1000)
			m.ResourceGetDuration = NewDurationHistogram(10000)
			m.PromptsListDuration = NewDurationHistogram(1000)
			m.PromptGetDuration = NewDurationHistogram(1000)
			m.RootsListDuration = NewDurationHistogram(1000)
			m.BatchToolCallSize = NewSizeHistogram(1000)
			return nil
		},
	)
}
