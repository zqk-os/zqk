package mcp

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// LifecycleEnumerator provides a unified interface for querying and formatting lifecycle information
// Can be used by both MCP server and CLI commands
type LifecycleEnumerator struct {
	lifecycles *LifecyclesContext
	loader     LifecycleLoader // Optional: for loading full lifecycle definitions
}

// LifecycleLoader is an interface for loading full lifecycle definitions
// This allows the enumerator to work with both summary data and full definitions
type LifecycleLoader interface {
	LoadLifecycle(objectType string) (*objects.Lifecycle, error)
}

// NewLifecycleEnumerator creates a new lifecycle enumerator from a LifecyclesContext
func NewLifecycleEnumerator(lifecycles *LifecyclesContext) *LifecycleEnumerator {
	return &LifecycleEnumerator{
		lifecycles: lifecycles,
		loader:     nil,
	}
}

// NewLifecycleEnumeratorWithLoader creates a new lifecycle enumerator with a loader for full definitions
func NewLifecycleEnumeratorWithLoader(lifecycles *LifecyclesContext, loader LifecycleLoader) *LifecycleEnumerator {
	return &LifecycleEnumerator{
		lifecycles: lifecycles,
		loader:     loader,
	}
}

// GetObjectKinds returns all object kinds that have lifecycles
func (e *LifecycleEnumerator) GetObjectKinds() []string {
	if e.lifecycles == nil {
		return nil
	}
	return e.lifecycles.ObjectKinds
}

// HasLifecycle checks if an object kind has a lifecycle definition
func (e *LifecycleEnumerator) HasLifecycle(objectKind string) bool {
	if e.lifecycles == nil {
		return false
	}
	_, exists := e.lifecycles.LifecycleMap[objectKind]
	return exists
}

// GetLifecycleSummary returns the lifecycle summary for an object kind
func (e *LifecycleEnumerator) GetLifecycleSummary(objectKind string) (*LifecycleSummary, bool) {
	if e.lifecycles == nil {
		return nil, false
	}
	summary, exists := e.lifecycles.LifecycleMap[objectKind]
	return &summary, exists
}

// GetStatuses returns all statuses for an object kind
func (e *LifecycleEnumerator) GetStatuses(objectKind string) []string {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return nil
	}
	return summary.Statuses
}

// GetTransitions returns transitions for an object kind, optionally limited to a count
func (e *LifecycleEnumerator) GetTransitions(objectKind string, limit int) []TransitionSummary {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return nil
	}
	transitions := summary.Transitions
	if limit > 0 && limit < len(transitions) {
		return transitions[:limit]
	}
	return transitions
}

// FormatLifecycleOverview formats a markdown overview of all lifecycles
func (e *LifecycleEnumerator) FormatLifecycleOverview() string {
	if e.lifecycles == nil || len(e.lifecycles.ObjectKinds) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "## Object Lifecycles\n", "**Available Object Kinds with Lifecycles**:\n")

	for _, kind := range e.lifecycles.ObjectKinds {
		parts = append(parts, fmt.Sprintf("- %s\n", kind))
		if summary, ok := e.lifecycles.LifecycleMap[kind]; ok {
			if len(summary.Statuses) > 0 {
				parts = append(parts, fmt.Sprintf("  - Statuses: %s\n", strings.Join(summary.Statuses, ", ")))
			}
			if len(summary.Transitions) > 0 {
				parts = append(parts, "  - Key Transitions:\n")
				// Show first 3 transitions
				transitions := summary.Transitions
				if len(transitions) > 3 {
					transitions = transitions[:3]
				}
				for _, trans := range transitions {
					parts = append(parts, fmt.Sprintf("    - %s → %s (manual: %v)\n", trans.From, trans.To, trans.Manual))
				}
			}
		}
	}

	parts = append(parts, "\n**IMPORTANT**: Always check lifecycle definitions before changing object states. Use resources/list to find the 'lifecycles_guide' resource for full details.\n")

	return strings.Join(parts, "")
}

// FormatLifecycleForKind formats detailed information about a specific object kind's lifecycle
func (e *LifecycleEnumerator) FormatLifecycleForKind(objectKind string) string {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return fmt.Sprintf("No lifecycle definition found for object kind: %s\n", objectKind)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("## Lifecycle: %s\n\n", objectKind))

	if len(summary.Statuses) > 0 {
		parts = append(parts, "### Statuses\n\n")
		parts = append(parts, strings.Join(summary.Statuses, ", "), "\n\n")
	}

	if len(summary.Transitions) > 0 {
		parts = append(parts, "### Transitions\n\n")
		for _, trans := range summary.Transitions {
			manualLabel := "automatic"
			if trans.Manual {
				manualLabel = "manual"
			}
			parts = append(parts, fmt.Sprintf("- **%s → %s** (%s)\n", trans.From, trans.To, manualLabel))
			if len(trans.Preconditions) > 0 {
				parts = append(parts, fmt.Sprintf("  - Preconditions: %s\n", strings.Join(trans.Preconditions, ", ")))
			}
		}
		parts = append(parts, "\n")
	}

	return strings.Join(parts, "")
}

// FormatLifecycleRequirements formats the lifecycle requirements section for execution context
func (e *LifecycleEnumerator) FormatLifecycleRequirements() string {
	if e.lifecycles == nil {
		return ""
	}

	return strings.Join([]string{
		"## Lifecycle Requirements\n\n",
		"Before changing any object state, you MUST:\n",
		"1. Understand the object's lifecycle\n",
		"2. Verify the transition is valid\n",
		"3. Check preconditions are met\n",
		"4. Use the correct transition command\n\n",
		"Use resources/list to find the 'lifecycles_guide' resource for lifecycle details.\n\n",
	}, "")
}

// ValidateTransition checks if a transition is valid for an object kind
func (e *LifecycleEnumerator) ValidateTransition(objectKind, fromStatus, toStatus string) (bool, string) {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return false, fmt.Sprintf("No lifecycle definition found for object kind: %s", objectKind)
	}

	for _, trans := range summary.Transitions {
		if trans.From == fromStatus && trans.To == toStatus {
			return true, ""
		}
	}

	return false, fmt.Sprintf("Invalid transition from '%s' to '%s' for object kind '%s'", fromStatus, toStatus, objectKind)
}

// GetValidTransitionsFrom returns all valid transitions from a given status
func (e *LifecycleEnumerator) GetValidTransitionsFrom(objectKind, fromStatus string) []TransitionSummary {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return nil
	}

	var valid []TransitionSummary
	for _, trans := range summary.Transitions {
		if trans.From == fromStatus {
			valid = append(valid, trans)
		}
	}
	return valid
}

// GetValidTransitionsTo returns all valid transitions to a given status
func (e *LifecycleEnumerator) GetValidTransitionsTo(objectKind, toStatus string) []TransitionSummary {
	summary, exists := e.GetLifecycleSummary(objectKind)
	if !exists {
		return nil
	}

	var valid []TransitionSummary
	for _, trans := range summary.Transitions {
		if trans.To == toStatus {
			valid = append(valid, trans)
		}
	}
	return valid
}
