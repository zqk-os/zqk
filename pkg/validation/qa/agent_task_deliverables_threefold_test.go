package qa

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Issue #935: 'agent_task' draft template generates 'outputs' field while qa_success verification gate requires 'artifacts'.
// Three-Fold Proof:
// 1. Static Floor Invariant
// 2. Operational Dynamic Proof
// 3. Negative Adversarial Boundary

func TestAgentTaskDeliverables_StaticFloorInvariant(t *testing.T) {
	// Invariant 1: agent_task must be recognized as a deliverable-bearing kind
	require.True(t, IsDeliverableBearingKind(objects.KindAgentTask), "agent_task must be a deliverable-bearing kind")
	require.True(t, IsDeliverableBearingKind(objects.KindBacklogItem), "backlog_item must be a deliverable-bearing kind")
	require.False(t, IsDeliverableBearingKind(objects.KindGoal), "goal is not deliverable-bearing")

	// Invariant 2: ExtractArtifactList handles all polymorphic representations
	// Slice of any strings
	paths1 := ExtractArtifactList([]any{"pkg/a.go", " pkg/b.go ", ""})
	require.Equal(t, []string{"pkg/a.go", "pkg/b.go"}, paths1)

	// Slice of strings
	paths2 := ExtractArtifactList([]string{"pkg/c.go", " pkg/d.go "})
	require.Equal(t, []string{"pkg/c.go", "pkg/d.go"}, paths2)

	// Single string
	paths3 := ExtractArtifactList("pkg/e.go")
	require.Equal(t, []string{"pkg/e.go"}, paths3)

	// Structured commit manifest maps in outputs
	manifestMap := map[string]any{
		objects.FieldKeyChangedPaths: []string{"pkg/f.go", "pkg/g.go"},
	}
	paths4 := ExtractArtifactList([]any{manifestMap})
	require.Equal(t, []string{"pkg/f.go", "pkg/g.go"}, paths4)

	// Path / artifact / file keys in map
	mapItem := map[string]any{"path": "pkg/h.go"}
	paths5 := ExtractArtifactList([]any{mapItem})
	require.Equal(t, []string{"pkg/h.go"}, paths5)

	// Direct map as value
	paths6 := ExtractArtifactList(manifestMap)
	require.Equal(t, []string{"pkg/f.go", "pkg/g.go"}, paths6)

	// Invariant 3: ExtractObjectArtifacts fallback hierarchy:
	// artifacts (primary) -> outputs (alias/fallback) -> code_location (legacy fallback)
	taskWithArtifacts := map[string]any{
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyArtifacts:    []any{"pkg/primary.go"},
		objects.FieldKeyOutputs:      []any{"pkg/ignored_output.go"},
		objects.FieldKeyCodeLocation: "pkg/ignored_legacy.go",
	}
	require.Equal(t, []string{"pkg/primary.go"}, ExtractObjectArtifacts(taskWithArtifacts))

	taskWithOutputsOnly := map[string]any{
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyArtifacts:    []any{},
		objects.FieldKeyOutputs:      []any{"pkg/output_alias.go"},
		objects.FieldKeyCodeLocation: "pkg/ignored_legacy.go",
	}
	require.Equal(t, []string{"pkg/output_alias.go"}, ExtractObjectArtifacts(taskWithOutputsOnly))

	taskWithLegacyOnly := map[string]any{
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyArtifacts:    []any{},
		objects.FieldKeyOutputs:      []any{},
		objects.FieldKeyCodeLocation: "pkg/legacy_code.go",
	}
	require.Equal(t, []string{"pkg/legacy_code.go"}, ExtractObjectArtifacts(taskWithLegacyOnly))

	// backlog_item polymorphic outputs, artifacts, and code_location verification
	bliWithArtifacts := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyArtifacts:    []any{"pkg/bli_primary.go"},
		objects.FieldKeyOutputs:      []any{"pkg/bli_ignored_output.go"},
		objects.FieldKeyCodeLocation: "pkg/bli_ignored_legacy.go",
	}
	require.Equal(t, []string{"pkg/bli_primary.go"}, ExtractObjectArtifacts(bliWithArtifacts))

	bliWithOutputsOnly := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyArtifacts:    []any{},
		objects.FieldKeyOutputs:      []any{"pkg/bli_output_alias.go"},
		objects.FieldKeyCodeLocation: "pkg/bli_ignored_legacy.go",
	}
	require.Equal(t, []string{"pkg/bli_output_alias.go"}, ExtractObjectArtifacts(bliWithOutputsOnly))

	bliWithLegacyOnly := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyArtifacts:    []any{},
		objects.FieldKeyOutputs:      []any{},
		objects.FieldKeyCodeLocation: "pkg/bli_legacy_code.go",
	}
	require.Equal(t, []string{"pkg/bli_legacy_code.go"}, ExtractObjectArtifacts(bliWithLegacyOnly))
}

func TestAgentTaskDeliverables_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	cleanFile := filepath.Join(tmpDir, "pkg", "worker.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(cleanFile), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(cleanFile, []byte("package worker\n\nfunc Work() {}\n"), paths.FilePerm644))

	// Operational Proof 1: agent_task with artifacts satisfies ValidateDeliverableArtifacts
	taskWithArtifacts := map[string]any{
		objects.FieldKeyID:        "ATK-DYN-001",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{cleanFile},
	}
	verified, err := ValidateDeliverableArtifacts(taskWithArtifacts, tmpDir)
	require.NoError(t, err)
	require.Equal(t, []string{cleanFile}, verified)

	// Operational Proof 2: agent_task with outputs (string alias) satisfies ValidateDeliverableArtifacts
	taskWithOutputs := map[string]any{
		objects.FieldKeyID:        "ATK-DYN-002",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{cleanFile},
	}
	verifiedOutputs, err := ValidateDeliverableArtifacts(taskWithOutputs, tmpDir)
	require.NoError(t, err)
	require.Equal(t, []string{cleanFile}, verifiedOutputs)

	// Operational Proof 3: agent_task with structured output manifest satisfies ValidateDeliverableArtifacts
	relPath := "pkg/worker.go"
	taskWithManifestOutput := map[string]any{
		objects.FieldKeyID:        "ATK-DYN-003",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs: []any{
			map[string]any{
				objects.FieldKeyChangedPaths: []string{relPath},
			},
		},
	}
	verifiedManifest, err := ValidateDeliverableArtifacts(taskWithManifestOutput, tmpDir)
	require.NoError(t, err)
	require.Equal(t, []string{relPath}, verifiedManifest)

	// Operational Proof 4: QA Service audit does not fail with deliverable disparity interrupt
	// when agent_task uses outputs alias.
	emitter := NewInterruptEmitter(tmpDir)
	store := newMockQASuccessStore()
	signer, err := NewAuditorSigner("")
	require.NoError(t, err)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(store)
	svc := NewAuditorService(nil, store, signer, emitter, engine, gate)

	taskID := "ATK-DYN-004"
	store.objs[taskID] = map[string]any{
		objects.FieldKeyID:           taskID,
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyStatus:       objects.ObjectStatusComplete,
		objects.FieldKeyCommitHashes: []any{"abc1234"},
		objects.FieldKeyArtifacts:    []any{},
		objects.FieldKeyOutputs:      []any{cleanFile},
		objects.FieldKeyTitle:        "Completed agent task with outputs",
		objects.FieldKeyDescription:  "Agent task deliverable via outputs",
	}

	svc.performAudit(context.Background(), taskID, objects.KindAgentTask)

	// Assert no disparity interrupt was emitted for missing artifacts
	latest := latestCriticalInterrupt(t, tmpDir)
	if latest != nil && latest.DedupeKey == "qa-disparity-"+taskID {
		t.Fatalf("expected no disparity interrupt for agent task with valid outputs, got: %s", latest.Message)
	}
}

func TestAgentTaskDeliverables_NegativeAdversarialBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// Boundary 1: Empty artifacts AND empty outputs strictly fails closed
	taskEmpty := map[string]any{
		objects.FieldKeyID:        "ATK-ADV-EMPTY",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{},
	}
	_, err := ValidateDeliverableArtifacts(taskEmpty, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), ReasonMissingArtifacts)
	require.Contains(t, err.Error(), "ATK-ADV-EMPTY")

	// Boundary 2: Outputs with whitespace only or non-string garbage strictly fails closed
	taskGarbage := map[string]any{
		objects.FieldKeyID:        "ATK-ADV-GARBAGE",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{"   ", "", 99999, nil},
	}
	_, err = ValidateDeliverableArtifacts(taskGarbage, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), ReasonMissingArtifacts)

	// Boundary 3: Outputs with non-existent file strictly fails closed
	missingFile := filepath.Join(tmpDir, "pkg", "does_not_exist.go")
	taskMissing := map[string]any{
		objects.FieldKeyID:        "ATK-ADV-MISSING",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{missingFile},
	}
	_, err = ValidateDeliverableArtifacts(taskMissing, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist or cannot be read")

	// Boundary 4: Directory path instead of regular file strictly fails closed
	subDir := filepath.Join(tmpDir, "pkg", "somedir")
	require.NoError(t, fileutil.MkdirAll(subDir, paths.DirPerm755))
	taskDir := map[string]any{
		objects.FieldKeyID:        "ATK-ADV-DIR",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{subDir},
	}
	_, err = ValidateDeliverableArtifacts(taskDir, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist or cannot be read")

	// Boundary 5: Path escaping project root strictly fails closed
	taskEscape := map[string]any{
		objects.FieldKeyID:        "ATK-ADV-ESCAPE",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
		objects.FieldKeyOutputs:   []any{"../../outside.go"},
	}
	_, err = ValidateDeliverableArtifacts(taskEscape, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "artifact file escapes project root")
}
