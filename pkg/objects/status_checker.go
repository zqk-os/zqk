package objects

import (
	"errors"
	"strings"
)

// IStatusChecker provides solid, spec-driven status classification queries for any object kind.
type IStatusChecker interface {
	IsTerminal(kind, status string) bool
	IsPreliminary(kind, status string) bool
	IsActive(kind, status string) bool
	IsArchive(kind, status string) bool
	IsSystem(kind, status string) bool
	IsWorkDone(kind, status string) bool
	IsSatisfied(kind, status string) bool
	// Role returns the lifecycle YAML role for kind+status (empty if unset/unknown).
	Role(kind, status string) string
}

// StatusChecker provides solid, spec-driven status classification queries for any object kind.
// It acts as a single centralized component to eliminate one-off, hardcoded status string checks.
type StatusChecker struct {
	loader *LifecycleLoader
}

// NewStatusChecker creates a new StatusChecker using the given lifecycle loader.
func NewStatusChecker(loader *LifecycleLoader) *StatusChecker {
	return &StatusChecker{loader: loader}
}

// GetGlobalStatusChecker returns an IStatusChecker initialized with the global lifecycle loader.
func GetGlobalStatusChecker() IStatusChecker {
	return NewStatusChecker(GetGlobalLifecycleLoader())
}

// IsTerminal checks if the status is terminal for the given kind.
func (sc *StatusChecker) IsTerminal(kind, status string) bool {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return s.Terminal
	}
	return false
}

// IsPreliminary checks if the status is preliminary/draft/origin for the given kind.
func (sc *StatusChecker) IsPreliminary(kind, status string) bool {
	if sc.loader == nil {
		sc.loader = GetGlobalLifecycleLoader()
	}
	isPrelim, err := sc.loader.IsPreliminaryStatusForKind(kind, status)
	if err != nil {
		return false
	}
	return isPrelim
}

// IsActive checks if the status represents an active/in-progress state (neither preliminary nor terminal/archived).
func (sc *StatusChecker) IsActive(kind, status string) bool {
	if status == "" {
		return false
	}
	if s, ok := sc.getStatusSpec(kind, status); ok {
		isPrelim, _ := sc.loader.IsPreliminaryStatusForKind(kind, s.Value)
		if isPrelim || s.Terminal || s.Archive || s.System {
			return false
		}
		if s.Role != "" {
			return s.Role == LifecycleRoleShovelReady || s.Role == LifecycleRoleExecutionLocked || s.Role == LifecycleRoleEnforced
		}
		return true
	}
	return false
}

// IsArchive checks if the status represents an archived/inactive state for the given kind.
func (sc *StatusChecker) IsArchive(kind, status string) bool {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return s.Archive
	}
	return false
}

// IsSystem checks if the status is a system-controlled/failure/error state for the given kind.
func (sc *StatusChecker) IsSystem(kind, status string) bool {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return s.System
	}
	return false
}

// IsWorkDone reports whether the status is a work-interval finish (not archive).
// envelope autofill keys off this flag, not status==complete.
func (sc *StatusChecker) IsWorkDone(kind, status string) bool {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return s.WorkDone && !s.Archive
	}
	return false
}

// IsSatisfied reports whether a satisfiable kind's predicate holds at this status.
func (sc *StatusChecker) IsSatisfied(kind, status string) bool {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return s.Satisfied
	}
	return false
}

// Role returns the cross-kind semantic role from lifecycle YAML (status.role).
func (sc *StatusChecker) Role(kind, status string) string {
	if s, ok := sc.getStatusSpec(kind, status); ok {
		return strings.TrimSpace(s.Role)
	}
	return ""
}

// getStatusSpec loads the lifecycle of the kind and retrieves the matching status specification.
func (sc *StatusChecker) getStatusSpec(kind, status string) (Status, bool) {
	if sc.loader == nil {
		sc.loader = GetGlobalLifecycleLoader()
	}
	resolved, err := sc.loader.ResolveStatusForKind(kind, status)
	if err != nil {
		if !errors.Is(err, ErrLifecycleStatusUnknown) {
			return Status{}, false
		}
		return Status{}, false
	}
	return resolved, true
}
