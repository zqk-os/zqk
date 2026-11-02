package scheduler

import (
	"errors"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const (
	DefaultSchedulerTimeout      = 30 * time.Second
	DefaultSchedulerTickInterval = 1 * time.Second
	DefaultMaxConcurrency        = 10
)

var (
	ErrInvalidSchedulerTimeout = errors.New("scheduler timeout must be positive")
	ErrInvalidTickInterval     = errors.New("scheduler tick interval must be positive")
	ErrInvalidAdmissionTimeout = errors.New("scheduler admission timeout must be positive")
	ErrInvalidMaxConcurrency   = errors.New("scheduler max concurrency must be positive")
)

// SchedulerOptions encapsulates configuration parameters for the scheduler.
type SchedulerOptions struct {
	Timeout           time.Duration
	TickInterval      time.Duration
	AdmissionTimeout  time.Duration
	MaxConcurrency    int
	ProjectRoot       string
	OnlyManualJobs    bool
	StorageProvider   storagepkg.ObjectStorageProvider
	AdmissionPipeline *JobAdmissionPipeline
}

// SchedulerOption modifies a SchedulerOptions instance.
type SchedulerOption func(*SchedulerOptions)

// DefaultSchedulerOptions returns a SchedulerOptions instance populated with default values.
func DefaultSchedulerOptions() *SchedulerOptions {
	return &SchedulerOptions{
		Timeout:          DefaultSchedulerTimeout,
		TickInterval:     DefaultSchedulerTickInterval,
		AdmissionTimeout: getSchedulerAdmissionTimeout(),
		MaxConcurrency:   DefaultMaxConcurrency,
	}
}

// NewSchedulerOptions creates a SchedulerOptions initialized with defaults and customized by opts.
// Safe against nil options.
func NewSchedulerOptions(opts ...SchedulerOption) *SchedulerOptions {
	options := DefaultSchedulerOptions()
	options.Apply(opts...)
	return options
}

// Apply applies options to the receiver with nil safety.
func (o *SchedulerOptions) Apply(opts ...SchedulerOption) *SchedulerOptions {
	if o == nil {
		return nil
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

// Validate checks whether options satisfy invariant constraints.
func (o *SchedulerOptions) Validate() error {
	if o == nil {
		return errfmt.Errorf("scheduler options cannot be nil")
	}
	if o.Timeout <= 0 {
		return ErrInvalidSchedulerTimeout
	}
	if o.TickInterval <= 0 {
		return ErrInvalidTickInterval
	}
	if o.AdmissionTimeout <= 0 {
		return ErrInvalidAdmissionTimeout
	}
	if o.MaxConcurrency <= 0 {
		return ErrInvalidMaxConcurrency
	}
	return nil
}

// WithTimeout sets the operation timeout.
func WithTimeout(d time.Duration) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil && d > 0 {
			o.Timeout = d
		}
	}
}

// WithTickInterval sets the periodic tick interval.
func WithTickInterval(d time.Duration) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil && d > 0 {
			o.TickInterval = d
		}
	}
}

// WithAdmissionTimeout sets the admission deadline.
func WithAdmissionTimeout(d time.Duration) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil && d > 0 {
			o.AdmissionTimeout = d
		}
	}
}

// WithMaxConcurrency sets max worker concurrency.
func WithMaxConcurrency(n int) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil && n > 0 {
			o.MaxConcurrency = n
		}
	}
}

// WithProjectRoot sets the project root path.
func WithProjectRoot(root string) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil {
			o.ProjectRoot = root
		}
	}
}

// WithOnlyManualJobs configures whether non-manual jobs are suppressed.
func WithOnlyManualJobs(only bool) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil {
			o.OnlyManualJobs = only
		}
	}
}

// WithStorageProvider injects an ObjectStorageProvider.
func WithStorageProvider(sp storagepkg.ObjectStorageProvider) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil {
			o.StorageProvider = sp
		}
	}
}

// WithAdmissionPipeline injects an admission pipeline.
func WithAdmissionPipeline(pipeline *JobAdmissionPipeline) SchedulerOption {
	return func(o *SchedulerOptions) {
		if o != nil {
			o.AdmissionPipeline = pipeline
		}
	}
}
