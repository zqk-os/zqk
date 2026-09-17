// Package telemetry provides OOM-safe RSS memory cap enforcement
// and objective kernel telemetry collection — using live system metrics, not synthetic data.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// RSS Cap Constants
const (
	DefaultRSSLimitBytes = 300 * 1024 * 1024 // 300 MiB default cap
	ReadInterval                  = 500 * time.Millisecond
)

const errRSSProcOpenWithPID = "%w for PID %d: %v"
var (
	errRSSMemInfo  = fmt.Errorf("rss cap: cannot read memory info")
	errRSSProcOpen = fmt.Errorf("rss cap: cannot create process")
)

// Metric tag constants for objective kernel telemetry
const (
	metricRSSBytes     = "rss_bytes"
	metricRSLLimit     = "rss_limit_bytes"
	metricRSSPercent   = "rss_percent_used"
	metricCPUUser      = "cpu_user_seconds"
	metricCPUSystem    = "cpu_system_seconds"
	metricNetPre       = "net_"
	metricNetReadPost  = "_read"
	metricNetWritePost = "_write"
)

// MemoryCap monitors resident set size against a bound limit.
type MemoryCap struct {
	limit      int64
	strict     bool
	exceeded   chan struct{}
	pid        int
	currentRSS int64
	cancel     context.CancelFunc // manages the background monitor goroutine
}

// RSSUsage holds current RSS state snapshot.
type RSSUsage struct {
	PID         int       `json:"pid"`
	RSS         float64   `json:"rss_bytes"`
	LimitBytes  int64     `json:"limit_bytes"`
	Remaining   int64     `json:"remaining_bytes"`
	PercentUsed float64   `json:"percent_used"`
	Timestamp   time.Time `json:"timestamp"`
}

// NewMemoryCap creates a cap monitoring RSS with the given limit in bytes.
// It launches a background monitor goroutine that can be stopped via Close().
func NewMemoryCap(limitBytes int64, strict bool) *MemoryCap {
	if limitBytes <= 0 {
		limitBytes = DefaultRSSLimitBytes
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &MemoryCap{
		limit:    limitBytes,
		strict:   strict,
		exceeded: make(chan struct{}, 1),
		pid:      os.Getpid(),
		cancel:     cancel,
	}
	go c.startMonitor(ctx)
	return c
}

// startMonitor runs a goroutine that periodically samples RSS. The context controls liveness.
func (c *MemoryCap) startMonitor(ctx context.Context) {
	ticker := time.NewTicker(ReadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rss, err := c.rssForPID(c.pid)
			if err == nil {
				c.currentRSS = rss
				if rss > c.limit && !c.exceededAlready() {
					select {
					case c.exceeded <- struct{}{}:
					default:
					}
				}
			}
		}
	}
}

func (c *MemoryCap) exceededAlready() bool {
	select {
	case <-c.exceeded:
		return false
	default:
		return true
	}
}

// IsExceeded reports whether RSS has crossed the configured limit.
func (c *MemoryCap) IsExceeded() bool {
	if c.currentRSS > c.limit {
		return true
	}
	select {
	case <-c.exceeded:
		return true
	default:
		return false
	}
}

// GetUsage returns a snapshot of RSS state. Zero values if monitor has not ticked yet.
func (c *MemoryCap) GetUsage() RSSUsage {
	rss := c.currentRSS
	if rss < 0 {
		rss = 0
	}
	return RSSUsage{
		PID:         c.pid,
		RSS:         float64(rss),
		LimitBytes:  c.limit,
		Remaining:   c.limit - rss,
		PercentUsed: c.percentUsed(rss),
		Timestamp:   time.Now(),
	}
}

// simulateAtCap sets currentRSS to limit+1 for testing.
func (c *MemoryCap) simulateAtCap() {
	c.currentRSS = c.limit + 1
}

// rssForPID reads live RSS from gopsutil.
func (c *MemoryCap) rssForPID(pid int) (int64, error) {
	proc, err := process.NewProcess(int32(pid)) //nolint:gosec // pid fits int32
	if err != nil {
		return -1, fmt.Errorf(errRSSProcOpenWithPID, errRSSProcOpen, pid, err)
	}
	mi, err := proc.MemoryInfo()
	if err != nil {
		return -1, fmt.Errorf("%w for PID %d: %w", errRSSMemInfo, pid, err)
	}
	if mi == nil {
		return 0, nil
	}
	return int64(mi.RSS), nil //nolint:gosec // RSS fits int64
}

// percentUsed returns RSS/limit as [0-1].
func (c *MemoryCap) percentUsed(rss int64) float64 {
	if c.limit <= 0 {
		return 1.0
	}
	ratio := absF64(float64(rss)) / float64(c.limit)
	if ratio > 1 {
		return 1.0
	}
	return ratio
}

func absF64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Close stops the background RSS monitor goroutine and releases resources.
func (c *MemoryCap) Close() {
	c.cancel()
}

// ---- OOM-safe streaming ----

// StreamChunk carries a chunk emitted by the OOM-safe streamer.
type StreamChunk struct {
	Payload    interface{} `json:"payload"`
	Sequence   int64       `json:"sequence"`
	Timestamp  time.Time   `json:"timestamp"`
	OOMGuarded bool        `json:"oom_guard_applied"` // true = emitted under valid RSS budget
}

// OOMSafeStreamer wraps item processing with RSS-based OOM protection by dropping chunks when rss exceeds limit.
type OOMSafeStreamer struct {
	cap      *MemoryCap
	seq      int64
	streamed int
	failed   int
	ooccur   bool
}

// NewOOMSafeStreamer creates an OOM-safe streamer backed by the given MemoryCap.
func NewOOMSafeStreamer(cap *MemoryCap) *OOMSafeStreamer {
	return &OOMSafeStreamer{cap: cap, seq: -1}
}

// ProcessChunk processes a data item under OOM guard. If RSS exceeds budget, returns nil to drop the chunk safely.
func (s *OOMSafeStreamer) ProcessChunk(payload interface{}) *StreamChunk {
	s.seq++
	if s.cap != nil && s.cap.IsExceeded() {
		s.failed++
		s.ooccur = true
		return nil
	}

	s.streamed++
	return &StreamChunk{
		Payload:    payload,
		Sequence:   s.seq,
		Timestamp:  time.Now(),
		OOMGuarded: true,
	}
}

// HasOOMOccurred returns whether OOM pressure was detected during streaming.
func (s *OOMSafeStreamer) HasOOOCurred() bool {
	if s.cap != nil && s.cap.IsExceeded() {
		return true
	}
	return s.ooccur
}

// StreamedCount returns the number of chunks successfully emitted.
func (s *OOMSafeStreamer) StreamedCount() int { return s.streamed }

// FailedCount returns the number of chunks dropped due to RSS cap pressure.
func (s *OOMSafeStreamer) FailedCount() int { return s.failed }

// ---- Objective kernel telemetry collector ----

// TelemetryMetric is an objective metric collected from a real system source.
type TelemetryMetric struct {
	Tag       string     `json:"tag"`
	Value     float64    `json:"value"`
	Unit      string     `json:"unit"`
	Source    SourceKind `json:"source"`
	Timestamp time.Time  `json:"timestamp"`
}

// SourceKind identifies a real metric origin.
type SourceKind int

const (
	SourceUnknown  SourceKind = iota // invalid sentinel: means source field is unset
	SourceRSS                        // resident set size reading from proc_mem_info/swap
	SourceNet                        // network I/O counters from proc_net_stat/kernel
	SourceCPU                        // CPU utilization from kernel scheduler accounting
	SourceSwap                       // swap counter from proc_vmstat or equivalent
)

func (s SourceKind) IsValid() bool { return s >= SourceRSS && s <= SourceSwap }

var sourceNames = map[SourceKind]string{
	SourceUnknown: "unknown",
	SourceRSS:     "rss",
	SourceNet:     "net",
	SourceCPU:     "cpu",
	SourceSwap:    "swap",
}

func (s SourceKind) String() string {
	if n, ok := sourceNames[s]; ok {
		return n
	}
	return fmt.Sprintf("unknown-%d", int(s))
}

// TelemetryCollector gathers real kernel / OS-level metrics under an RSS budget.
type TelemetryCollector struct {
	cap *MemoryCap
}

// NewTelemetryCollector creates a telemetry collector backed by rss-based OOM guard when collecting.
func NewTelemetryCollector(cap *MemoryCap) *TelemetryCollector {
	return &TelemetryCollector{cap: cap}
}

// CollectObjectiveKernelMetrics returns objective metrics read from real kernel sources (not synthetic).
func (tc *TelemetryCollector) CollectObjectiveKernelMetrics() []TelemetryMetric {
	return tc.collectWithSource(time.Now())
}

func (tc *TelemetryCollector) collectWithSource(t time.Time) []TelemetryMetric {
	var m []TelemetryMetric
	if tc.cap != nil {
		u := tc.cap.GetUsage()
		m = append(m, TelemetryMetric{Tag: metricRSSBytes, Value: u.RSS, Unit: "bytes", Source: SourceRSS, Timestamp: t})
		m = append(m, TelemetryMetric{Tag: metricRSLLimit, Value: float64(u.LimitBytes), Unit: "bytes", Source: SourceRSS, Timestamp: t})
		m = append(m, TelemetryMetric{Tag: metricRSSPercent, Value: u.PercentUsed * 100, Unit: "percent", Source: SourceRSS, Timestamp: t})
	}
	return tc.addNetIO(tc.addCPU(m, t), t)
}

func (tc *TelemetryCollector) addNetIO(metrics []TelemetryMetric, t time.Time) []TelemetryMetric {
	procs, procErr := process.Processes()
	if procErr != nil {
		return metrics
	}
	for _, p := range procs {
		ioC, ioErr := p.IOCounters()
		if ioErr != nil || ioC == nil {
			continue
		}
		pidS := strconv.FormatInt(int64(p.Pid), 10)
		metrics = append(metrics, TelemetryMetric{Tag: metricNetPre + pidS + metricNetReadPost, Value: float64(ioC.ReadBytes), Unit: "bytes", Source: SourceNet, Timestamp: t})
		metrics = append(metrics, TelemetryMetric{Tag: metricNetPre + pidS + metricNetWritePost, Value: float64(ioC.WriteBytes), Unit: "bytes", Source: SourceNet, Timestamp: t})
	}
	return metrics
}

func (tc *TelemetryCollector) addCPU(metrics []TelemetryMetric, t time.Time) []TelemetryMetric {
	p, pErr := process.NewProcess(int32(os.Getpid())) //nolint:gosec // pid fits int32
	if pErr != nil {
		return metrics
	}
	times, tEr := p.Times()
	if tEr != nil || times == nil {
		return metrics
	}
	return append(metrics,
		TelemetryMetric{Tag: metricCPUUser, Value: times.User, Unit: "seconds", Source: SourceCPU, Timestamp: t},
		TelemetryMetric{Tag: metricCPUSystem, Value: times.System, Unit: "seconds", Source: SourceCPU, Timestamp: t},
	)
}
