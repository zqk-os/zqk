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
	ensureCreateLifecycleStatus(obj)
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
	ensureCreateLifecycleStatus(obj)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusProposed {
		t.Fatalf("expected coerce to proposed, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_ValidNonOriginPreserved(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	ensureCreateLifecycleStatus(obj)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusInProgress {
		t.Fatalf("expected valid non-origin preserved, got %q", got)
	}
}
