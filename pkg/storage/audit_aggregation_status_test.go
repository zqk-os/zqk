package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestAuditEventAggregationAllowedStatusesMatchesLifecycle verifies that the
// hard-coded allow-list for aggregation eligible audit_event statuses remains
// aligned with the lifecycle spec (docs/process/_internal/lifecycles/audit_event_lifecycle.yaml).
func TestAuditEventAggregationAllowedStatusesMatchesLifecycle(t *testing.T) {

	// Current lifecycle statuses for audit_event (from spec):
	// - pending (initial)
	// - completed (terminal, successful)
	// - failed (terminal)
	// - reverted (terminal)
	// - archived (terminal, archive: true)
	// - error (system: true)
	//
	// Must match auditEventAggregationAllowedStatuses in audit_aggregation.go: terminal
	// non-archived statuses eligible for aggregation (completed, failed, reverted, error).
	expected := map[string]bool{
		"completed":            true,
		objects.FieldKeyFailed: true,
		"reverted":             true,
		"error":                true,
	}

	if len(auditEventAggregationAllowedStatuses) != len(expected) {
		t.Fatalf("auditEventAggregationAllowedStatuses length = %d, want %d", len(auditEventAggregationAllowedStatuses), len(expected))
	}

	for _, status := range auditEventAggregationAllowedStatuses {
		if !expected[status] {
			t.Errorf("status %q is not expected to be aggregation-eligible", status)
		}
	}
}
