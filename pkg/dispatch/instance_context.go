package dispatch

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/storage"
)

// Registry keys for the instance context. Use GetRegistry(key) and type-assert.
// Callers: e.g. dispatch.GetInstanceContext().GetRegistry(dispatch.KeySpecLoader).(*objects.SpecLoader)
const (
	KeySpecLoader        = "spec_loader"
	KeyLifecycleLoader   = "lifecycle_loader"
	KeySynonymResolver   = "synonym_resolver"
	KeyKindMapper        = "kind_mapper"
	KeyValidatorRegistry = "validator_registry"
	KeyFieldRegistry     = "field_registry"
	emptyValue           = ""
)

// InstanceContext is the one-stop-shop for expensive process-scoped components when running
// under the dispatcher (e.g. MCP serve). Bootstrap creates storage and registers loaders/registries
// here so all tool calls and in-process CLI use the same instances. See docs/architecture/DISPATCH_LOOP_AND_MCP.md.
type InstanceContext struct {
	Storage     storage.ObjectStorageProvider
	ProjectRoot string
	Profile     string
	registries  map[string]any
}

// Storage returns the shared object storage provider for this instance.
func (c *InstanceContext) GetStorage() storage.ObjectStorageProvider {
	if c == nil {
		return nil
	}
	return c.Storage
}

// GetProjectRoot returns the project root for this instance.
func (c *InstanceContext) GetProjectRoot() string {
	if c == nil {
		return emptyValue
	}
	return c.ProjectRoot
}

// GetProfile returns the logging profile for this instance (e.g. "mcp").
func (c *InstanceContext) GetProfile() string {
	if c == nil {
		return emptyValue
	}
	return c.Profile
}

// GetRegistry returns a registered component by key (e.g. KeySpecLoader). Returns nil if not set.
// Type-assert to the concrete type: e.g. GetRegistry(KeySpecLoader).(*objects.SpecLoader).
func (c *InstanceContext) GetRegistry(key string) any {
	if c == nil || c.registries == nil {
		return nil
	}
	return c.registries[key]
}

// SetRegistry registers a component under key. Used by bootstrap to populate the container.
func (c *InstanceContext) SetRegistry(key string, value any) {
	if c == nil {
		return
	}
	if c.registries == nil {
		c.registries = make(map[string]any)
	}
	c.registries[key] = value
}

var (
	instanceContextMu sync.Mutex
	instanceContext   *InstanceContext
)

// SetInstanceContext sets the global instance context (e.g. after bootstrap). Call ClearInstanceContext on shutdown.
func SetInstanceContext(c *InstanceContext) {
	instanceContextMu.Lock()
	defer instanceContextMu.Unlock()
	instanceContext = c
}

// GetInstanceContext returns the current instance context, or nil if not set (e.g. one-shot CLI).
func GetInstanceContext() *InstanceContext {
	instanceContextMu.Lock()
	defer instanceContextMu.Unlock()
	return instanceContext
}

// ClearInstanceContext clears the global instance context. Call when the server/dispatcher shuts down.
func ClearInstanceContext() {
	SetInstanceContext(nil)
}
