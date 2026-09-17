package instance_builders

import (
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
)

var (
	globalInstanceBuilderRegistry   *VersionedInstanceBuilderRegistry
	instanceBuilderRegistryOnce     sync.Once
	globalInstanceBuilderRegistryMu sync.RWMutex
)

func init() {
	globalInstanceBuilderRegistry = NewVersionedInstanceBuilderRegistry(nil)
	// Builders from bldr_instance_v1 package register themselves via their init() functions
	// Note: bldr_instance_v1 package must be imported (e.g., in tests or main) to trigger init()
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder InstanceBuilder) {
	_ = concurrency.RunInLock(&globalInstanceBuilderRegistryMu, func() error {
		if globalInstanceBuilderRegistry == nil {
			globalInstanceBuilderRegistry = NewVersionedInstanceBuilderRegistry(nil)
		}
		globalInstanceBuilderRegistry.Register(builder)
		return nil
	})
}

// GetGlobalRegistry returns the global instance builder registry
// Note: This requires setting the spec registry first via SetSpecRegistry
func GetGlobalRegistry() *VersionedInstanceBuilderRegistry {
	instanceBuilderRegistryOnce.Do(func() {
		// Create registry without spec registry initially
		// Caller must set spec registry via SetSpecRegistry
		if globalInstanceBuilderRegistry == nil {
			globalInstanceBuilderRegistry = NewVersionedInstanceBuilderRegistry(nil)
		}
	})
	return globalInstanceBuilderRegistry
}

// SetSpecRegistry sets the spec builder registry for the global instance builder registry
// This allows instance builders to reference spec builders
func SetSpecRegistry(specRegistry SpecBuilderRegistryInterface) {
	_ = concurrency.RunInLock(&globalInstanceBuilderRegistryMu, func() error {
		if globalInstanceBuilderRegistry == nil {
			globalInstanceBuilderRegistry = NewVersionedInstanceBuilderRegistry(specRegistry)
		} else {
			globalInstanceBuilderRegistry.specRegistry = specRegistry
		}
		return nil
	})
}
