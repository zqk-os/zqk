package id_generation

import (
	"fmt"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

var (
	strategies     = make(map[string]Strategy)
	strategiesLock sync.RWMutex
)

// RegisterStrategy registers an ID generation strategy
// Should be called during package initialization
func RegisterStrategy(strategy Strategy) error {
	if strategy == nil {
		return fmt.Errorf("%s", ConstCannotRegisterNilStrategy)
	}
	name := strategy.Name()
	if name == emptyValue {
		return fmt.Errorf("%s", ConstStrategyMustHaveANonEmptyName)
	}

	return concurrency.RunInLockOrLog(&strategiesLock, locknames.LockNameIdGenerationRegisterStrategy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if _, exists := strategies[name]; exists {
			return fmt.Errorf(ConstStrategySAlreadyRegistered, name)
		}
		strategies[name] = strategy
		return nil
	})
}

// GetStrategy returns a strategy by name
// Returns nil if strategy not found
func GetStrategy(name string) Strategy {
	var strategy Strategy
	_ = concurrency.RunInRLockOrLog(&strategiesLock, locknames.LockNameIdGenerationGetStrategy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		strategy = strategies[name]
		return nil
	})
	return strategy
}

// GetDefaultStrategy returns the default sequential strategy
func GetDefaultStrategy() Strategy {
	var strategy Strategy
	_ = concurrency.RunInRLockOrLog(&strategiesLock, locknames.LockNameIdGenerationGetDefaultStrategy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if s, ok := strategies["sequential"]; ok {
			strategy = s
		}
		return nil
	})
	return strategy
}

// ListStrategies returns all registered strategy names
func ListStrategies() []string {
	var names []string
	_ = concurrency.RunInRLockOrLog(&strategiesLock, locknames.LockNameIdGenerationListStrategies, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		names = make([]string, 0, len(strategies))
		for name := range strategies {
			names = append(names, name)
		}
		return nil
	})
	return names
}
