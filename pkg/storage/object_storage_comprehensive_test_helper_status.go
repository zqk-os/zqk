// Extracted from object_storage_comprehensive_test_helper.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

func initializeStatusCache() {
	statusCacheOnce.Do(func() {
		statusCache = make(map[string]string)

		// Load from lifecycle loader (original read)
		lifecycleLoader := objects.NewLifecycleLoader("")

		// Try to load lifecycle for common kinds
		commonKinds := []string{
			objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone, objects.KindComponent, objects.KindPriorityPlan,
			objects.KindRequirement, objects.KindCriteria, objects.KindTestCase, objects.KindRoadmap, objects.KindDecision,
			objects.KindAccount, objects.KindWorkstream, objects.KindRelease, objects.KindRole, objects.KindMission, objects.KindVision,
		}

		for _, kind := range commonKinds {
			lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
			if err == nil && lifecycle != nil {
				// Find initial status from lifecycle definition
				for _, status := range lifecycle.Statuses {
					if status.Origin {
						statusCache[kind] = status.Value
						break
					}
				}
			}
		}

		// Fallback for built-in objects (immutable without admin intervention)
		// Built-ins may have different statuses than regular objects
		// These are used when lifecycle doesn't define an initial status
		builtInStatusFallback := map[string]string{
			objects.KindBacklogItem:  compTestStatusExploring,
			objects.KindGoal:         compTestStatusActive,
			objects.KindMilestone:    testStatusNotStarted,
			objects.KindWorkstream:   compTestStatusActive,
			objects.KindPriorityPlan: compTestStatusDraft,
			objects.KindRequirement:  compTestStatusDraft,
			objects.KindTestCase:     compTestStatusDraft,
			objects.KindRoadmap:      compTestStatusDraft,
			objects.KindDecision:     testStatusProposed,
			objects.KindComponent:    testStatusCreated,
			// Lifecycle allows only draft | active | archived; avoid generic "not_started" default.
			objects.KindVerificationMatrix: compTestStatusDraft,
		}

		// Only use fallback if lifecycle didn't provide a status
		// This handles conflicts: lifecycle takes precedence, but built-ins
		// can override if lifecycle is missing
		for kind, fallbackStatus := range builtInStatusFallback {
			if _, exists := statusCache[kind]; !exists {
				statusCache[kind] = fallbackStatus
			}
		}
	})
}

// getInitialStatusForKind returns a valid initial status for a given kind
// Uses cached statuses from lifecycle definitions, with fallback for built-in objects
func getInitialStatusForKind(kind string) string {
	// Initialize cache if not already done (happens during lifecycle initialization)
	initializeStatusCache()

	comprehensiveTestCacheMu.RLock()
	if status, ok := statusCache[kind]; ok {
		comprehensiveTestCacheMu.RUnlock()
		return status
	}
	comprehensiveTestCacheMu.RUnlock()

	// If not in cache, try to load lifecycle dynamically (outside lock — I/O)
	lifecycleLoader := objects.NewLifecycleLoader("")
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	var resolved string
	if err == nil && lifecycle != nil {
		for _, status := range lifecycle.Statuses {
			if status.Origin {
				resolved = status.Value
				break
			}
		}
	}
	if resolved == emptyValue {
		resolved = testStatusNotStarted
	}

	comprehensiveTestCacheMu.Lock()
	defer comprehensiveTestCacheMu.Unlock()
	if status, ok := statusCache[kind]; ok {
		return status
	}
	statusCache[kind] = resolved
	return resolved
}

// initializeValidStatusesCache loads valid statuses from lifecycle definitions
// This should be called during test initialization to establish fallback cache
// Lifecycles and built-in objects may have conflicting statuses because built-ins
// are immutable without admin intervention
func initializeValidStatusesCache() {
	validStatusesCacheOnce.Do(func() {
		validStatusesCache = make(map[string][]string)

		// Load from lifecycle loader (original read)
		lifecycleLoader := objects.NewLifecycleLoader("")

		// Try to load lifecycle for common kinds
		commonKinds := []string{
			objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone, objects.KindComponent, objects.KindPriorityPlan,
			objects.KindRequirement, objects.KindCriteria, objects.KindTestCase, objects.KindRoadmap, objects.KindDecision,
			objects.KindAccount, objects.KindWorkstream, objects.KindRelease, objects.KindRole, objects.KindMission, objects.KindVision,
		}

		for _, kind := range commonKinds {
			lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
			if err == nil && lifecycle != nil {
				// Extract all valid statuses from lifecycle definition
				statuses := make([]string, 0, len(lifecycle.Statuses))
				for _, status := range lifecycle.Statuses {
					statuses = append(statuses, status.Value)
				}
				if len(statuses) > 0 {
					validStatusesCache[kind] = statuses
				}
			}
		}

		// Fallback for built-in objects (immutable without admin intervention)
		// Built-ins may have different statuses than regular objects
		// These are used when lifecycle doesn't define statuses
		builtInStatusesFallback := map[string][]string{
			objects.KindBacklogItem:        {compTestStatusExploring, testStatusValidated, "planned", testStatusInProgress, testStatusComplete, testStatusArchived},
			objects.KindGoal:               {compTestStatusActive, testStatusComplete, testStatusDeferred},
			objects.KindMilestone:          {testStatusNotStarted, testStatusInProgress, "blocked", testStatusComplete, testStatusDeferred},
			objects.KindWorkstream:         {compTestStatusActive, testStatusComplete, "paused"},
			objects.KindPriorityPlan:       {compTestStatusDraft, compTestStatusActive, testStatusComplete},
			objects.KindRequirement:        {compTestStatusDraft, "approved", "implemented", "verified"},
			objects.KindTestCase:           {compTestStatusDraft, "ready", "passed", "failed"},
			objects.KindRoadmap:            {compTestStatusDraft, "published", testStatusArchived},
			objects.KindDecision:           {testStatusProposed, "approved", testStatusRejected, "superseded"},
			objects.KindComponent:          {testStatusCreated, testStatusValidated, "placed", "rendered"},
			objects.KindVerificationMatrix: {compTestStatusDraft, compTestStatusActive, testStatusArchived},
		}

		// Only use fallback if lifecycle didn't provide statuses
		// This handles conflicts: lifecycle takes precedence, but built-ins
		// can override if lifecycle is missing
		for kind, fallbackStatuses := range builtInStatusesFallback {
			if _, exists := validStatusesCache[kind]; !exists {
				validStatusesCache[kind] = fallbackStatuses
			}
		}
	})
}

// getValidStatusesForKind returns valid status values for a given kind
// Uses cached statuses from lifecycle definitions, with fallback for built-in objects
func getValidStatusesForKind(kind string) []string {
	// Initialize cache if not already done (happens during lifecycle initialization)
	initializeValidStatusesCache()

	comprehensiveTestCacheMu.RLock()
	if statuses, ok := validStatusesCache[kind]; ok {
		comprehensiveTestCacheMu.RUnlock()
		return statuses
	}
	comprehensiveTestCacheMu.RUnlock()

	lifecycleLoader := objects.NewLifecycleLoader("")
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	var resolved []string
	if err == nil && lifecycle != nil {
		statuses := make([]string, 0, len(lifecycle.Statuses))
		for _, status := range lifecycle.Statuses {
			statuses = append(statuses, status.Value)
		}
		if len(statuses) > 0 {
			resolved = statuses
		}
	}
	if len(resolved) == 0 {
		resolved = []string{testStatusNotStarted, testStatusInProgress, testStatusComplete}
	}

	comprehensiveTestCacheMu.Lock()
	defer comprehensiveTestCacheMu.Unlock()
	if statuses, ok := validStatusesCache[kind]; ok {
		return statuses
	}
	validStatusesCache[kind] = resolved
	return resolved
}

// setKindSpecificFields sets required fields specific to certain kinds
