package community

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	// ErrInvalidProviderDescriptor is returned when a provider descriptor fails validation.
	ErrInvalidProviderDescriptor = errors.New("invalid provider descriptor")
	// ErrProviderAlreadyRegistered is returned when attempting to register a duplicate provider.
	ErrProviderAlreadyRegistered = errors.New("provider already registered")
	// ErrProviderNotFound is returned when a requested provider cannot be located.
	ErrProviderNotFound = errors.New("provider not found")
	// ErrHookTimeout is returned when hook execution exceeds the permitted deadline.
	ErrHookTimeout = errors.New("hook execution timed out")
	// ErrHookPanicked is returned when hook execution panics.
	ErrHookPanicked = errors.New("hook execution panicked")
)

var validProviderNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// LifecycleHookStage defines when an ecosystem hook executes.
type LifecycleHookStage string

const (
	// HookStagePreExecution runs before tool or command execution.
	HookStagePreExecution LifecycleHookStage = "pre_execution"
	// HookStagePostExecution runs after successful tool or command execution.
	HookStagePostExecution LifecycleHookStage = "post_execution"
	// HookStageErrorRecovery runs upon failure or error detection.
	HookStageErrorRecovery LifecycleHookStage = "error_recovery"
	// HookStageTelemetryEmit runs during telemetry publication.
	HookStageTelemetryEmit LifecycleHookStage = "telemetry_emit"
)

// LifecycleHookHandler handles an isolated lifecycle hook event.
type LifecycleHookHandler func(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error)

// ProviderDescriptor defines metadata, compatibility, and hook handlers for an ecosystem provider.
type ProviderDescriptor struct {
	Name          string                                      `json:"name"`
	Version       string                                      `json:"version"`
	TargetVersion string                                      `json:"target_engine_version"`
	Capabilities  []string                                    `json:"capabilities"`
	Timeout       time.Duration                               `json:"timeout"`
	HookHandlers  map[LifecycleHookStage]LifecycleHookHandler `json:"-"`
}

// Validate validates the provider descriptor.
func (pd *ProviderDescriptor) Validate() error {
	if pd.Name == "" || !validProviderNameRegex.MatchString(pd.Name) {
		return fmt.Errorf("%w: name %q must be non-empty alphanumeric", ErrInvalidProviderDescriptor, pd.Name)
	}
	if pd.Version == "" {
		return fmt.Errorf("%w: version cannot be empty", ErrInvalidProviderDescriptor)
	}
	if len(pd.Capabilities) == 0 {
		return fmt.Errorf("%w: at least one capability must be declared", ErrInvalidProviderDescriptor)
	}
	if pd.Timeout <= 0 {
		pd.Timeout = 5 * time.Second
	} else if pd.Timeout > 30*time.Second {
		return fmt.Errorf("%w: timeout %v exceeds maximum limit (30s)", ErrInvalidProviderDescriptor, pd.Timeout)
	}
	return nil
}

// ProviderRegistry manages external ecosystem providers and isolated hook dispatching.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]ProviderDescriptor
}

// NewProviderRegistry creates an empty thread-safe ProviderRegistry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ProviderDescriptor),
	}
}

// Register registers a new provider after validation.
func (pr *ProviderRegistry) Register(desc ProviderDescriptor) error {
	if err := desc.Validate(); err != nil {
		return err
	}

	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.providers[desc.Name]; exists {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyRegistered, desc.Name)
	}

	pr.providers[desc.Name] = desc
	return nil
}

// Get retrieves a provider by name.
func (pr *ProviderRegistry) Get(name string) (ProviderDescriptor, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	desc, ok := pr.providers[name]
	if !ok {
		return ProviderDescriptor{}, ErrProviderNotFound
	}
	return desc, nil
}

// Count returns the number of registered providers.
func (pr *ProviderRegistry) Count() int {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return len(pr.providers)
}

// DispatchHook executes a lifecycle hook with isolated fail-closed recovery and timeout protection.
func (pr *ProviderRegistry) DispatchHook(ctx context.Context, providerName string, stage LifecycleHookStage, payload map[string]interface{}) (res map[string]interface{}, err error) {
	desc, err := pr.Get(providerName)
	if err != nil {
		return nil, err
	}

	handler, ok := desc.HookHandlers[stage]
	if !ok || handler == nil {
		// No hook registered for this stage: no-op
		return payload, nil
	}

	timeout := desc.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type hookResult struct {
		data map[string]interface{}
		err  error
	}

	done := make(chan hookResult, 1)

	goroutinelabels.NewGoroutine("ecosystem_dispatch_hook", "dispatch ecosystem lifecycle hook handler").StartSimple(func() {
		defer func() {
			if r := recover(); r != nil {
				done <- hookResult{data: nil, err: fmt.Errorf("%w: %v", ErrHookPanicked, r)}
			}
		}()

		resultData, hookErr := handler(hookCtx, payload)
		done <- hookResult{data: resultData, err: hookErr}
	})

	select {
	case <-hookCtx.Done():
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrHookTimeout
		}
		return nil, hookCtx.Err()
	case r := <-done:
		return r.data, r.err
	}
}
