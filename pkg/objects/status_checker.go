package objects

// IStatusChecker provides solid, spec-driven status classification queries for any object kind.
type IStatusChecker interface {
	IsTerminal(kind, status string) bool
	IsPreliminary(kind, status string) bool
	IsActive(kind, status string) bool
	IsArchive(kind, status string) bool
	IsSystem(kind, status string) bool
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
		return !isPrelim && !s.Terminal && !s.Archive
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

// getStatusSpec loads the lifecycle of the kind and retrieves the matching status specification.
func (sc *StatusChecker) getStatusSpec(kind, status string) (Status, bool) {
	if sc.loader == nil {
		sc.loader = GetGlobalLifecycleLoader()
	}
	lifecycle, err := sc.loader.LoadLifecycle(kind)
	if err != nil {
		return Status{}, false
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	canonical := ApplyAliasesForStatus(status, validSet)
	for _, s := range lifecycle.Statuses {
		if s.Value == canonical {
			return s, true
		}
	}
	return Status{}, false
}
