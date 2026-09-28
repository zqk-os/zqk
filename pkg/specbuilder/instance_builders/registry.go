package instance_builders

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

var (
	globalInstanceBuilderRegistry   *VersionedInstanceBuilderRegistry
	instanceBuilderRegistryOnce     sync.Once
	globalInstanceBuilderRegistryMu sync.RWMutex
)

func init() {
	globalInstanceBuilderRegistry = NewVersionedInstanceBuilderRegistry(nil)
}

// RegisterBuilder registers a builder. Files already generated under bldr_instance_v1
// still call this from init. New generator output does not. Callers use NewForKind.
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
