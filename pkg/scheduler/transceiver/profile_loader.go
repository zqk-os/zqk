package transceiver

import (
	"context"

	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// RouterProfileSpec represents a loaded router profile specification
type RouterProfileSpec struct {
	Enabled           bool
	MaxWorkers        int
	QueueSize         int
	EnqueueTimeoutMs  int
	DefaultTimeoutMs  int
	BackoffStrategy   string
	MaxBackoffMs      int
	MetricsIntervalMs int
	IsDefault         bool
}

// RouterProfileLoader loads router profiles from the unified profile system
type RouterProfileLoader struct {
	profileLoader *config.ProfileLoader
	logger        logging.Logger
}

// NewRouterProfileLoader creates a new router profile loader
func NewRouterProfileLoader(profilesDir string, logger logging.Logger) *RouterProfileLoader {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &RouterProfileLoader{
		profileLoader: config.NewProfileLoader(profilesDir),
		logger:        logger,
	}
}

// LoadProfile loads a router profile and resolves inheritance
func (rpl *RouterProfileLoader) LoadProfile(profileName string) (*RouterProfileSpec, error) {
	// Load unified profile
	unifiedProfile, err := rpl.profileLoader.LoadProfileWithInheritance(profileName)
	if err != nil {
		return nil, errfmt.Errorf("failed to load profile %s: %w", profileName, err)
	}

	// Validate profile type
	if unifiedProfile.Type != config.ProfileTypeTransceiverRouter {
		return nil, errfmt.Errorf("profile %s is not a transceiver_router profile (type: %s)", profileName, unifiedProfile.Type)
	}

	// Extract router-specific spec
	spec, err := rpl.extractRouterSpec(unifiedProfile.ResolvedSpec)
	if err != nil {
		return nil, errfmt.Errorf("failed to extract router spec from profile %s: %w", profileName, err)
	}

	logging.Fluent(rpl.logger).Debug(LogEventSchedulerTransceiverProfileLoaded).
		ProfileName(profileName).
		MaxWorkers(spec.MaxWorkers).
		QueueSize(spec.QueueSize).
		Log()

	return spec, nil
}

// extractRouterSpec extracts router configuration from resolved spec
func (rpl *RouterProfileLoader) extractRouterSpec(resolvedSpec map[string]any) (*RouterProfileSpec, error) {
	spec := &RouterProfileSpec{
		// Defaults
		Enabled:           true,
		MaxWorkers:        10,
		QueueSize:         100,
		EnqueueTimeoutMs:  100,
		DefaultTimeoutMs:  5000,
		BackoffStrategy:   "exponential",
		MaxBackoffMs:      30000,
		MetricsIntervalMs: 0,
		IsDefault:         false,
	}

	// Extract enabled
	if enabled, ok := resolvedSpec[objects.FieldKeyEnabled].(bool); ok {
		spec.Enabled = enabled
	}

	// Extract max_workers
	if maxWorkers, ok := resolvedSpec["max_workers"].(int); ok {
		if maxWorkers > 0 {
			spec.MaxWorkers = maxWorkers
		}
	}

	// Extract queue_size
	if queueSize, ok := resolvedSpec["queue_size"].(int); ok {
		if queueSize > 0 {
			spec.QueueSize = queueSize
		}
	}

	// Extract enqueue_timeout_ms
	if enqueueTimeoutMs, ok := resolvedSpec["enqueue_timeout_ms"].(int); ok {
		if enqueueTimeoutMs > 0 {
			spec.EnqueueTimeoutMs = enqueueTimeoutMs
		}
	}

	// Extract default_timeout_ms
	if defaultTimeoutMs, ok := resolvedSpec["default_timeout_ms"].(int); ok {
		if defaultTimeoutMs > 0 {
			spec.DefaultTimeoutMs = defaultTimeoutMs
		}
	}

	// Extract backoff_strategy
	if backoffStrategy, ok := resolvedSpec["backoff_strategy"].(string); ok {
		if backoffStrategy != emptyValue {
			spec.BackoffStrategy = backoffStrategy
		}
	}

	// Extract max_backoff_ms
	if maxBackoffMs, ok := resolvedSpec["max_backoff_ms"].(int); ok {
		if maxBackoffMs > 0 {
			spec.MaxBackoffMs = maxBackoffMs
		}
	}

	// Extract metrics_interval_ms
	if metricsIntervalMs, ok := resolvedSpec["metrics_interval_ms"].(int); ok {
		spec.MetricsIntervalMs = metricsIntervalMs
	}

	// Extract is_default
	if isDefault, ok := resolvedSpec[objects.FieldKeyIsDefault].(bool); ok {
		spec.IsDefault = isDefault
	}

	return spec, nil
}

// NewAsyncRouterFromProfile creates an AsyncRouter from a profile
// NewAsyncRouterFromProfile creates an async router from a profile
// ctx: parent context from command entry point (should not be created here)
func NewAsyncRouterFromProfile(ctx context.Context, router *Router, profileName string, logger logging.Logger) (*AsyncRouter, error) {
	// Create profile loader
	profileLoader := NewRouterProfileLoader("", logger)

	// Load profile
	profile, err := profileLoader.LoadProfile(profileName)
	if err != nil {
		return nil, errfmt.Errorf("failed to load router profile %s: %w", profileName, err)
	}

	// Create async router with profile settings
	asyncRouter := NewAsyncRouter(
		ctx,
		router,
		profile.MaxWorkers,
		profile.QueueSize,
		logger,
	)

	// Set enqueue timeout if different from default
	// (This would require adding a field to AsyncRouter - for now, we'll use defaults)

	logging.Fluent(logger).Info(LogEventSchedulerTransceiverProfileCreatedAsyncRouter).
		ProfileName(profileName).
		MaxWorkers(profile.MaxWorkers).
		QueueSize(profile.QueueSize).
		Enabled(profile.Enabled).
		Log()

	return asyncRouter, nil
}

// ApplyProfileToRouter applies profile settings to an existing router
// This can be used to update router configuration at runtime
func ApplyProfileToRouter(router *Router, profile *RouterProfileSpec) error {
	// For now, router doesn't have configurable backoff strategy
	// This would require adding fields to Router struct
	// Future enhancement: make router configurable via profile

	// Log profile application
	logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info(LogEventSchedulerTransceiverProfileApplied).
		BackoffStrategy(profile.BackoffStrategy).
		MaxBackoffMs(profile.MaxBackoffMs).
		DefaultTimeoutMs(profile.DefaultTimeoutMs).
		Log()

	return nil
}

// GetDefaultProfileName returns the name of the default router profile
func GetDefaultProfileName() string {
	return "default_router"
}

// LoadDefaultProfile loads the default router profile
func LoadDefaultProfile(logger logging.Logger) (*RouterProfileSpec, error) {
	profileLoader := NewRouterProfileLoader("", logger)
	return profileLoader.LoadProfile(GetDefaultProfileName())
}
