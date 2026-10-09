package agent

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

type envelopeTestFixture struct {
	proj testkit.IsolatedTempProject
	root string
	fs   *storage.FileObjectStorage
	ctx  context.Context
	sec  *pkgctx.SecurityContext
}

func initEnvelopeFixture(t *testing.T, forceCleanup bool) *envelopeTestFixture {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "agent_prepared_ctx",
		SeedSchemaPlane:          true,
		ForceRemoveRootOnCleanup: forceCleanup,
	})
	return &envelopeTestFixture{
		proj: proj,
		root: proj.Root,
		fs:   proj.FileStorage,
		ctx:  context.Background(),
		sec:  pkgctx.NewSystemSecurityContext(),
	}
}

// TestEnvelopeHydration_FunctionalAcceptance verifies that AssemblePreparedContext
// builds a comprehensive task envelope with standing mandates and context.
func TestEnvelopeHydration_FunctionalAcceptance(t *testing.T) {
	f := initEnvelopeFixture(t, true)

	// Seed a test task
	taskID := "ATK-TEST-HYDRATION-001"
	task := map[string]any{
		"id":          taskID,
		"kind":        "agent_task",
		"title":       "Test Task for Hydration",
		"description": "Ensure task envelope is properly hydrated",
		"status":      "in_progress",
	}
	if err := f.fs.Create(f.ctx, f.sec, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	prepared, err := AssemblePreparedContext(f.ctx, f.sec, f.fs, PreparedContextInput{
		TaskID:      taskID,
		Persona:     "PER-DEFAULT-AGENT",
		ProjectRoot: f.root,
		Depth:       1,
		IncludeTDD:  true,
	})
	if err != nil {
		t.Fatalf("AssemblePreparedContext failed: %v", err)
	}

	if prepared.TaskID != taskID {
		t.Errorf("expected TaskID %s, got %s", taskID, prepared.TaskID)
	}
	if !strings.Contains(prepared.Prompt, "Standing mandates") {
		t.Errorf("expected prompt to contain Standing mandates")
	}
	if !strings.Contains(prepared.Prompt, "Ensure task envelope is properly hydrated") {
		t.Errorf("expected prompt to contain task description")
	}
	if !prepared.HasSemanticContext() {
		t.Errorf("expected prepared context to have semantic context")
	}
}

// TestEnvelopeHydration_BoundaryAndErrorHandling verifies fail-closed refusal
// when task ID and description are missing or storage is nil.
func TestEnvelopeHydration_BoundaryAndErrorHandling(t *testing.T) {
	f := initEnvelopeFixture(t, false)

	// 1. Both TaskID and Description empty must error
	_, err := AssemblePreparedContext(f.ctx, f.sec, f.fs, PreparedContextInput{
		TaskID:      "",
		Description: "",
	})
	if err == nil {
		t.Error("expected error for empty task ID and description, got nil")
	}
	if !isPreparedContextRefusal(err) {
		t.Errorf("expected error to be recognized as prepared context refusal: %v", err)
	}

	// 2. Nil storage must error
	_, errNilStorage := AssemblePreparedContext(f.ctx, f.sec, nil, PreparedContextInput{
		TaskID: "ATK-1",
	})
	if errNilStorage == nil {
		t.Error("expected error for nil storage, got nil")
	}
}

// TestEnvelopeHydration_IntegrationAndConformance verifies that ad-hoc descriptions
// assemble into an actionable prompt without cold-start failures.
func TestEnvelopeHydration_IntegrationAndConformance(t *testing.T) {
	f := initEnvelopeFixture(t, true)

	desc := "Ad-hoc bugfix for race condition in queue"
	prepared, err := AssemblePreparedContext(f.ctx, f.sec, f.fs, PreparedContextInput{
		Description: desc,
		ProjectRoot: f.root,
		IncludeTDD:  true,
	})
	if err != nil {
		t.Fatalf("AssemblePreparedContext for ad-hoc task failed: %v", err)
	}

	if !strings.Contains(prepared.Prompt, desc) {
		t.Errorf("expected prompt to contain ad-hoc description")
	}
	if !strings.Contains(prepared.Prompt, "Policy Compliance") {
		t.Errorf("expected prompt to include Policy Compliance section")
	}
}
