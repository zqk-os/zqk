package lifecycle_builders

import (
	"maps"

	"github.com/zqk-os/zqk/pkg/objects"
)

// BaseLifecycleBuilder provides common functionality for lifecycle builders
type BaseLifecycleBuilder struct {
	objectType      string
	version         string
	extends         string
	statusMapping   map[string]string
	statuses        []objects.Status
	transitions     []objects.Transition
	percentComplete objects.PercentCompleteConfig
}

// NewBaseLifecycleBuilder creates a new base lifecycle builder
func NewBaseLifecycleBuilder(objectType, version string) *BaseLifecycleBuilder {
	return &BaseLifecycleBuilder{
		objectType:      objectType,
		version:         version,
		statusMapping:   make(map[string]string),
		statuses:        []objects.Status{},
		transitions:     []objects.Transition{},
		percentComplete: objects.PercentCompleteConfig{},
	}
}

// SetExtends sets the extends field
func (b *BaseLifecycleBuilder) SetExtends(extends string) *BaseLifecycleBuilder {
	b.extends = extends
	return b
}

// SetStatusMapping sets the status mapping
func (b *BaseLifecycleBuilder) SetStatusMapping(mapping map[string]string) *BaseLifecycleBuilder {
	b.statusMapping = mapping
	return b
}

// AddStatus adds a status
func (b *BaseLifecycleBuilder) AddStatus(status objects.Status) *BaseLifecycleBuilder {
	b.statuses = append(b.statuses, status)
	return b
}

// AddTransition adds a transition
func (b *BaseLifecycleBuilder) AddTransition(transition objects.Transition) *BaseLifecycleBuilder {
	b.transitions = append(b.transitions, transition)
	return b
}

// SetPercentComplete sets the percent complete configuration
func (b *BaseLifecycleBuilder) SetPercentComplete(config objects.PercentCompleteConfig) *BaseLifecycleBuilder {
	b.percentComplete = config
	return b
}

// Build builds the lifecycle
func (b *BaseLifecycleBuilder) Build() *objects.Lifecycle {
	lifecycle := &objects.Lifecycle{
		ObjectType:      b.objectType,
		Extends:         b.extends,
		PercentComplete: b.percentComplete,
	}

	lifecycle.StatusMapping = make(map[string]string)
	maps.Copy(lifecycle.StatusMapping, b.statusMapping)

	lifecycle.Statuses = make([]objects.Status, len(b.statuses))
	copy(lifecycle.Statuses, b.statuses)

	lifecycle.Transitions = make([]objects.Transition, len(b.transitions))
	copy(lifecycle.Transitions, b.transitions)

	return lifecycle
}

// GetVersion returns the version
func (b *BaseLifecycleBuilder) GetVersion() string {
	return b.version
}

// GetObjectType returns the object type
func (b *BaseLifecycleBuilder) GetObjectType() string {
	return b.objectType
}
