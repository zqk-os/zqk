package cli

import (
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// CommandTimeoutConfig holds configuration for command timeout calculations
// This abstracts timeout values to avoid duplication and improve maintainability
type CommandTimeoutConfig struct {
	// InfiniteTimeout is used when timeout is disabled (treated as infinite)
	InfiniteTimeout time.Duration

	// SystemCheckTimeoutConfig holds timeout calculation parameters for system check commands
	SystemCheckTimeoutConfig struct {
		BaseTimePerObject    time.Duration // Time per object estimate
		WorkerCount          int           // Default worker count
		SafetyMargin         float64       // Safety margin multiplier (e.g., 2.0 = 2x)
		Overhead             time.Duration // Overhead for enqueueing and result collection
		MinTimeout           time.Duration // Minimum timeout bound
		MaxTimeout           time.Duration // Maximum timeout bound
		MinReasonableMinutes int           // Floor in minutes so baseline*3 doesn't kill long runs (e.g. 10)
	}

	// DefaultTimeoutBounds holds default timeout bounds for all commands
	DefaultTimeoutBounds struct {
		MinTimeout           time.Duration // Minimum timeout (safety net)
		MaxReasonableTimeout time.Duration // Maximum reasonable timeout for non-server commands
	}

	// MetricsRecordingTimeout is timeout for metrics recording operations
	MetricsRecordingTimeout time.Duration
}

// Product target for system check: full run ≤20s cold (no validation cache); repeat run ≤1–2s warm
// (validation cache hit, object ID cache + CAS index already warm). This floor is a safety net only—
// the goal is to meet the target via caching and throughput, not to allow long timeouts.
func defaultMinReasonableMinutesForSystemCheck() int { return 5 }

// DefaultCommandTimeoutConfig returns default configuration for command timeouts
func DefaultCommandTimeoutConfig() *CommandTimeoutConfig {
	return &CommandTimeoutConfig{
		InfiniteTimeout: 8760 * time.Hour, // 1 year (treated as infinite)
		SystemCheckTimeoutConfig: struct {
			BaseTimePerObject    time.Duration
			WorkerCount          int
			SafetyMargin         float64
			Overhead             time.Duration
			MinTimeout           time.Duration
			MaxTimeout           time.Duration
			MinReasonableMinutes int
		}{
			BaseTimePerObject:    100 * time.Millisecond,
			WorkerCount:          4,
			SafetyMargin:         2.0,
			Overhead:             30 * time.Second,
			MinTimeout:           1 * time.Minute,
			MaxTimeout:           30 * time.Minute,
			MinReasonableMinutes: defaultMinReasonableMinutesForSystemCheck(), // Safety net; target is 20s cold / 1–2s warm
		},
		DefaultTimeoutBounds: struct {
			MinTimeout           time.Duration
			MaxReasonableTimeout time.Duration
		}{
			MinTimeout:           10 * time.Second,
			MaxReasonableTimeout: 5 * time.Minute,
		},
		MetricsRecordingTimeout: 5 * time.Second,
	}
}

// TimeoutHook manages command execution timeouts and metrics
type TimeoutHook struct {
	enabled         bool
	maxTimeout      time.Duration
	commandTimeouts map[string]time.Duration // Command-specific timeouts (e.g., "mcp serve" -> 5m)
	metricsStore    MetricsStore
	logger          *logging.EventLogger
	mu              sync.RWMutex
	timeoutConfig   *CommandTimeoutConfig // Timeout configuration (uses global if nil)
	wg              sync.WaitGroup
}

// MetricsStore interface for storing command metrics
type MetricsStore interface {
	RecordCommandExecution(metric *CommandMetric) error
	GetCommandMetrics(command string) (*CommandMetrics, error)
	GetAllMetrics() (map[string]*CommandMetrics, error)
}

// CommandMetric represents a single command execution with complete traceability
type CommandMetric struct {
	Command       string         `json:"command"`
	NormalizedCmd string         `json:"normalized_cmd"`
	Duration      time.Duration  `json:"duration"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	Success       bool           `json:"success"`
	ExitCode      int            `json:"exit_code"`
	Error         string         `json:"error,omitempty"`
	TimedOut      bool           `json:"timed_out"`
	Timestamp     time.Time      `json:"timestamp"`
	Args          []string       `json:"args,omitempty"`  // Sanitized args
	Flags         map[string]any `json:"flags,omitempty"` // Command flags (sanitized)

	// Execution context
	PriorityPlan string `json:"priority_plan,omitempty"`
	Workstream   string `json:"workstream,omitempty"`
	Milestone    string `json:"milestone,omitempty"`

	// Actor information
	ActorID    string   `json:"actor_id,omitempty"`    // account:username or ACC-###
	ActorRoles []string `json:"actor_roles,omitempty"` // admin, developer, etc.

	// System state changes
	ObjectsCreated []string `json:"objects_created,omitempty"` // Object IDs created
	ObjectsUpdated []string `json:"objects_updated,omitempty"` // Object IDs updated
	ObjectsDeleted []string `json:"objects_deleted,omitempty"` // Object IDs deleted

	// Resource tracking
	MaxMemoryBytes uint64 `json:"max_memory_bytes,omitempty"`
}

// CommandMetrics aggregates metrics for a command
type CommandMetrics struct {
	Command          string        `json:"command"`
	NormalizedCmd    string        `json:"normalized_cmd"`
	InvocationCount  int           `json:"invocation_count"`
	SuccessCount     int           `json:"success_count"`
	FailureCount     int           `json:"failure_count"`
	TimeoutCount     int           `json:"timeout_count"`
	BaselineDuration time.Duration `json:"baseline_duration"`
	FastestDuration  time.Duration `json:"fastest_duration"`
	SlowestDuration  time.Duration `json:"slowest_duration"`
	AvgDuration      time.Duration `json:"avg_duration"`
	FirstSeen        time.Time     `json:"first_seen"`
	LastSeen         time.Time     `json:"last_seen"`
	ErrorRate        float64       `json:"error_rate"`   // Percentage
	TimeoutRate      float64       `json:"timeout_rate"` // Percentage
	AvgMemoryBytes   uint64        `json:"avg_memory_bytes,omitempty"`
	MaxMemoryBytes   uint64        `json:"max_memory_bytes,omitempty"`
}

// CommandContext provides execution context for command tracking
type CommandContext struct {
	PriorityPlan string         // Priority plan ID (e.g., PRI-208)
	Workstream   string         // Workstream ID (e.g., WS-007)
	Milestone    string         // Milestone ID (e.g., MIL-036)
	ActorID      string         // Actor account ID (e.g., ACC-1785920548450214012-68b850c0)
	ActorRoles   []string       // Actor roles (e.g., ["admin"])
	Flags        map[string]any // Command flags (sanitized)
}

var (
	globalTimeoutHook *TimeoutHook
	hookOnce          sync.Once
	// Global timeout configuration (shared across all TimeoutHook instances)
	globalTimeoutConfig = DefaultCommandTimeoutConfig()
)
