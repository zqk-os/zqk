package testing

import (
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	pkgmcp "github.com/lanceman/zqk/pkg/mcp"
)

// ProcessorRegistry manages named response processors
type ProcessorRegistry struct {
	processors map[string]func() ResponseProcessor
	mu         sync.RWMutex
}

var globalRegistry = &ProcessorRegistry{
	processors: make(map[string]func() ResponseProcessor),
}

func init() {
	// Register built-in processors
	globalRegistry.Register("elicitation_to_success", func() ResponseProcessor {
		return &ElicitationToSuccessProcessor{}
	})
	globalRegistry.Register("ignore_errors", func() ResponseProcessor {
		return &IgnoreErrorsProcessor{}
	})
	globalRegistry.Register("noop", func() ResponseProcessor {
		return ResponseProcessorFunc(func(result any, err error) (any, error, bool) {
			return result, err, true
		})
	})
}

// Register registers a named processor factory
func (r *ProcessorRegistry) Register(name string, factory func() ResponseProcessor) {
	_ = concurrency.RunInLockWithLogger(
		&r.mu, pkgmcp.LockNameProcessorRegistryRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			r.processors[name] = factory
			return nil
		},
	)
}

// Get creates an instance of a named processor
func (r *ProcessorRegistry) Get(name string) (ResponseProcessor, error) {
	var factory func() ResponseProcessor
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&r.mu, pkgmcp.LockNameProcessorRegistryGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			factory, ok = r.processors[name]
			exists = ok
			return nil
		},
	)

	if !exists {
		return nil, errfmt.Errorf("unknown processor: %s", name)
	}
	return factory(), nil
}

// List returns all registered processor names
func (r *ProcessorRegistry) List() []string {
	var names []string
	_ = concurrency.RunInRLockWithLogger(
		&r.mu, pkgmcp.LockNameProcessorRegistryList, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			names = make([]string, 0, len(r.processors))
			for name := range r.processors {
				names = append(names, name)
			}
			return nil
		},
	)
	return names
}

// GetGlobalRegistry returns the global processor registry
func GetGlobalProcessorRegistry() *ProcessorRegistry {
	return globalRegistry
}

// RegisterProcessor is a convenience function to register a processor in the global registry
func RegisterProcessor(name string, factory func() ResponseProcessor) {
	globalRegistry.Register(name, factory)
}

// GetProcessor is a convenience function to get a processor from the global registry
func GetProcessor(name string) (ResponseProcessor, error) {
	return globalRegistry.Get(name)
}
