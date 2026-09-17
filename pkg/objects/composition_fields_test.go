package objects

import (
	"testing"
)

func TestIsCompositionFieldAllowed(t *testing.T) {
	// Universal fields
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyVersionContext, nil) {
		t.Errorf("expected version_context to be allowed universally")
	}
	if !IsCompositionFieldAllowed(KindWorkflow, FieldKeyVersionContext, nil) {
		t.Errorf("expected version_context to be allowed for workflow")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyTags, nil) {
		t.Errorf("expected tags to be allowed universally")
	}

	// Occupancy fields
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyClaimedBy, nil) {
		t.Errorf("expected claimed_by to be allowed on backlog_item")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyClaimedAt, nil) {
		t.Errorf("expected claimed_at to be allowed on backlog_item")
	}

	// Work-envelope effort & clock fields
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyActualEffort, nil) {
		t.Errorf("expected actual_effort to be allowed on backlog_item")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, FieldKeyPercentComplete, nil) {
		t.Errorf("expected percent_complete to be allowed on backlog_item")
	}

	// Workflow specific fields
	if !IsCompositionFieldAllowed(KindWorkflow, "workflow_steps", nil) {
		t.Errorf("expected workflow_steps to be allowed on workflow")
	}
	if !IsCompositionFieldAllowed(KindWorkflow, "name", nil) {
		t.Errorf("expected name to be allowed on workflow")
	}

	// Backlog legacy/tracking compatibility fields
	if !IsCompositionFieldAllowed(KindBacklogItem, "acceptance_criteria", nil) {
		t.Errorf("expected acceptance_criteria to be allowed on backlog_item")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, "next_action", nil) {
		t.Errorf("expected next_action to be allowed on backlog_item")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, "stakeholder_refs", nil) {
		t.Errorf("expected stakeholder_refs to be allowed on backlog_item")
	}
	if !IsCompositionFieldAllowed(KindBacklogItem, "document_refs", nil) {
		t.Errorf("expected leftover document_refs to be allowed on backlog_item")
	}

	// Criteria composition & evidence fields
	if !IsCompositionFieldAllowed(KindCriteria, "evidence", nil) {
		t.Errorf("expected evidence to be allowed on criteria")
	}
	if !IsCompositionFieldAllowed(KindCriteria, "bundle_command_fingerprint", nil) {
		t.Errorf("expected bundle_command_fingerprint to be allowed on criteria")
	}

	// Invented random field must NOT be allowed
	if IsCompositionFieldAllowed(KindBacklogItem, "some_fake_field_123", nil) {
		t.Errorf("did not expect some_fake_field_123 to be allowed")
	}
}
