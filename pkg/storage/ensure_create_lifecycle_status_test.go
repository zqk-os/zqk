package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestEnsureCreateLifecycleStatus_MissingUsesOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader() // ensure lazy init from repo lifecycles dir

	obj := map[string]any{
		objects.FieldKeyKind:  objects.KindAgentTask,
		objects.FieldKeyTitle: "t",
	}
	ensureCreateLifecycleStatus(obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusProposed {
		t.Fatalf("expected origin proposed, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_InvalidCoercedToOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusActive, // not in agent_task lifecycle
	}
	ensureCreateLifecycleStatus(obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusProposed {
		t.Fatalf("expected coerce to proposed, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_NonPreliminaryCoercedToOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// TRACK: [REDACTED-ID] — create must not birth shovel_ready into CAS.
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	ensureCreateLifecycleStatus(obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusProposed {
		t.Fatalf("expected non-preliminary create coerced to proposed, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_PromoteOnCreateKeepsShovelReady(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// Same as --promote / WithPromoteOnCreate: valid shovel-ready status skips draft coerce.
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindRiskBlocker,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusOpen,
	}
	ensureCreateLifecycleStatus(obj, true)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusOpen {
		t.Fatalf("promoteOnCreate: expected open kept, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_PolicyActiveCoercedToDraft(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindPolicy,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	ensureCreateLifecycleStatus(obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusDraft {
		t.Fatalf("expected policy create active→draft, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_GlossaryActiveOriginUnchanged(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// glossary_term origin is active (non-preliminary) — coerce cannot park until lifecycle grows draft.
	// TRACK: [REDACTED-ID]
	obj := map[string]any{
		objects.FieldKeyKind:   "glossary_term",
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	ensureCreateLifecycleStatus(obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusActive {
		t.Fatalf("expected glossary active origin left as-is until lifecycle fix, got %q", got)
	}
}
