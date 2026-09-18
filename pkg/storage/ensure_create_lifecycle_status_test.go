package storage

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

func TestEnsureCreateLifecycleStatus_MissingUsesOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader() // ensure lazy init from repo lifecycles dir

	obj := map[string]any{
		objects.FieldKeyKind:  objects.KindAgentTask,
		objects.FieldKeyTitle: "t",
	}
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != "conceptual" {
		t.Fatalf("expected origin conceptual, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_InvalidCoercedToOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	bogusStatus := "not_" + "a_real_status"
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: bogusStatus,
	}
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != "conceptual" {
		t.Fatalf("expected coerce to conceptual, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_CLINonPreliminaryCoercedToOrigin(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// CLI interactive create without --promote: non-preliminary status coerced to origin
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != "conceptual" {
		t.Fatalf("expected CLI non-preliminary create coerced to conceptual, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_NonCLIKeepsShovelReady(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// Non-CLI (system-generated): valid shovel-ready status must NOT be coerced to conceptual/draft plane
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindRiskBlocker,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusOpen,
	}
	nonCLICtx := context.Background()
	ensureCreateLifecycleStatus(nonCLICtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusOpen {
		t.Fatalf("non-CLI create: expected open kept, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_BypassKindKeepsShovelReady(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	// Bypass kinds (stream storage e.g. audit_event): must NEVER be coerced to preliminary/draft plane, even under CLI context
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindAuditEvent,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: "pending",
	}
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != "pending" {
		t.Fatalf("stream storage kind audit_event: expected pending kept, got %q", got)
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
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, true)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != objects.ObjectStatusOpen {
		t.Fatalf("promoteOnCreate: expected open kept, got %q", got)
	}
}

func TestEnsureCreateLifecycleStatus_CLIPolicyActiveCoercedToConceptual(t *testing.T) {
	_ = objects.GetGlobalLifecycleLoader()

	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindPolicy,
		objects.FieldKeyTitle:  "t",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	cliCtx := audit.WithCLIOperation(context.Background())
	ensureCreateLifecycleStatus(cliCtx, obj, false)
	got, _ := obj[objects.FieldKeyStatus].(string)
	if got != "conceptual" {
		t.Fatalf("expected CLI policy create active→conceptual, got %q", got)
	}
}

