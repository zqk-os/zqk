package object

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestObjectMutation_DottedFieldExpansion verifies nesting and deep-merge logic.
func TestObjectMutation_DottedFieldExpansion(t *testing.T) {
	t.Parallel()

	// 1. expandDottedFieldKeys
	flat := map[string]any{
		"title":                "sample task",
		"meta.tags":            []string{"backend", "go"},
		"spec.runtime.timeout": 30,
		"deep.a.b.c":           "nested_value",
	}

	expanded := expandDottedFieldKeys(flat)
	assert.Equal(t, "sample task", expanded["title"])

	metaMap, ok := expanded["meta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"backend", "go"}, metaMap["tags"])

	specMap, ok := expanded["spec"].(map[string]any)
	require.True(t, ok)
	runtimeMap, ok := specMap["runtime"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 30, runtimeMap["timeout"])

	// 2. setNestedMap edge cases
	m := make(map[string]any)
	setNestedMap(m, []string{}, "ignored")
	assert.Empty(t, m)

	setNestedMap(m, []string{"level1"}, "val1")
	assert.Equal(t, "val1", m["level1"])

	// Overwrite non-map intermediate with map
	setNestedMap(m, []string{"level1", "child"}, "val2")
	childMap, ok := m["level1"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "val2", childMap["child"])
}

// TestObjectMutation_ValidateFileHash verifies content-addressable storage hash checks.
func TestObjectMutation_ValidateFileHash(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// 1. Non-CAS filename is ignored
	plainFile := filepath.Join(tmpDir, "plain.yaml")
	err := fileutil.WriteFile(plainFile, []byte("content"), 0644)
	require.NoError(t, err)
	err = validateFileHash(plainFile)
	assert.NoError(t, err)

	// 2. Non-hex 64-char filename is ignored
	nonHexFile := filepath.Join(tmpDir, "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz.yaml")
	err = fileutil.WriteFile(nonHexFile, []byte("content"), 0644)
	require.NoError(t, err)
	err = validateFileHash(nonHexFile)
	assert.NoError(t, err)

	// 3. Valid 64-char hex filename with matching content hash
	content := []byte("title: Valid Content\n")
	hasher := sha256.New()
	_, writeErr := hasher.Write(content)
	require.NoError(t, writeErr)
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	matchFile := filepath.Join(tmpDir, contentHash+".yaml")
	err = fileutil.WriteFile(matchFile, content, 0644)
	require.NoError(t, err)
	err = validateFileHash(matchFile)
	assert.NoError(t, err)

	// 4. Valid 64-char hex filename with mismatched content hash
	mismatchFile := filepath.Join(tmpDir, contentHash+".yaml")
	err = fileutil.WriteFile(mismatchFile, []byte("corrupted payload"), 0644)
	require.NoError(t, err)
	err = validateFileHash(mismatchFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")

	// 5. Valid 64-char hex filename that does not exist
	missingFile := filepath.Join(tmpDir, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.yaml")
	err = validateFileHash(missingFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

// TestObjectMutation_ReferenceFlagParsing verifies --add-ref / --remove-ref argument parsing.
func TestObjectMutation_ReferenceFlagParsing(t *testing.T) {
	t.Parallel()

	// Equals separator
	field, val, err := parseRefFlag("goal_refs=GOAL-123")
	require.NoError(t, err)
	assert.Equal(t, "goal_refs", field)
	assert.Equal(t, "GOAL-123", val)

	// Colon separator
	field, val, err = parseRefFlag("requirement_ref:REQ-456")
	require.NoError(t, err)
	assert.Equal(t, "requirement_ref", field)
	assert.Equal(t, "REQ-456", val)

	// Whitespace trimming
	field, val, err = parseRefFlag("  milestone_refs  =  MIL-789  ")
	require.NoError(t, err)
	assert.Equal(t, "milestone_refs", field)
	assert.Equal(t, "MIL-789", val)

	// Missing separator
	_, _, err = parseRefFlag("invalid_string_without_separator")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")

	// Empty field or value
	_, _, err = parseRefFlag("=GOAL-123")
	assert.Error(t, err)
	_, _, err = parseRefFlag("goal_refs=")
	assert.Error(t, err)
}

// TestObjectMutation_UnsetAndOptimisticLockFlags verifies field unsetting and optimistic locking.
func TestObjectMutation_UnsetAndOptimisticLockFlags(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().StringArray("unset-field", nil, "")
	cmd.Flags().String("expected-updated-at", "", "")

	// 1. applyUnsetFieldFlags ignores immutable system fields and unsets user fields
	fieldsToUnset := []string{"id", "kind", "created_at", "created_by", "expected_updated_at", "description", "notes"}
	for _, f := range fieldsToUnset {
		err := cmd.Flags().Set("unset-field", f)
		require.NoError(t, err)
	}

	updates := make(map[string]any)
	applyUnsetFieldFlags(cmd, updates)

	assert.NotContains(t, updates, "id")
	assert.NotContains(t, updates, "kind")
	assert.NotContains(t, updates, "created_at")
	assert.NotContains(t, updates, "created_by")
	assert.NotContains(t, updates, "expected_updated_at")
	assert.Equal(t, storage.FieldUnset, updates["description"])
	assert.Equal(t, storage.FieldUnset, updates["notes"])

	// 2. addOptimisticLocking
	err := cmd.Flags().Set("expected-updated-at", "2026-11-05T12:00:00Z")
	require.NoError(t, err)
	addOptimisticLocking(cmd, updates)
	assert.Equal(t, "2026-11-05T12:00:00Z", updates["expected_updated_at"])
}

// TestObjectMutation_LifecycleDerivationAndBulkHelpers tests bulk update heuristics and derivations.
func TestObjectMutation_LifecycleDerivationAndBulkHelpers(t *testing.T) {
	t.Parallel()

	// 1. deriveNextLifecycleStatus
	next, err := deriveNextLifecycleStatus(map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: "planned",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, next)

	// 2. shouldUseBulkInvalidation
	assert.False(t, shouldUseBulkInvalidation(50, 100, true), "dryRun should never use bulk invalidation")
	assert.False(t, shouldUseBulkInvalidation(50, 0, false), "zero total objects should return false")
	assert.True(t, shouldUseBulkInvalidation(35, 100, false), "35% > 30% threshold")
	assert.False(t, shouldUseBulkInvalidation(25, 100, false), "25% <= 30% threshold")

	// 3. formatUpdateAllResults
	res := formatUpdateAllResults("task", 5, 1, []string{"validation error"}, true)
	str := string(res)
	assert.Contains(t, str, "Success: 5")
	assert.Contains(t, str, "Errors: 1")
	assert.Contains(t, str, "validation error")
	assert.Contains(t, str, "Dry-run mode")

	// 4. getMissingFields namespace derivation
	objWithoutNS := map[string]any{
		objects.FieldKeyID: "BLI-1793906394453581000-61a6a086",
	}
	missing := getMissingFields(objWithoutNS, objects.KindBacklogItem, nil)
	assert.Contains(t, missing, objects.FieldKeyNamespaceID)
}

// TestObjectMutation_NegativeAdversarialStatusGuardrails verifies protection against direct status manipulation.
func TestObjectMutation_NegativeAdversarialStatusGuardrails(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("override", false, "")
	cmd.Flags().String("reason-code", "", "")
	cmd.Flags().Bool("auto-status", false, "")

	// 1. No status update -> allowed
	err := guardManualStatusUpdate(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"title": "new"})
	assert.NoError(t, err)

	// 2. Direct status update without override -> rejected
	err = guardManualStatusUpdate(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"status": "completed"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manual status updates are restricted")

	// 3. Direct status update with auto-status -> allowed
	err = cmd.Flags().Set("auto-status", "true")
	require.NoError(t, err)
	err = guardManualStatusUpdate(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"status": "completed"})
	assert.NoError(t, err)

	// Reset auto-status
	err = cmd.Flags().Set("auto-status", "false")
	require.NoError(t, err)

	// 4. Override without reason-code -> rejected
	err = cmd.Flags().Set("override", "true")
	require.NoError(t, err)
	err = guardManualStatusUpdate(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"status": "completed"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required")
}

// TestObjectMutation_NegativeAdversarialRefGuardrails verifies graph poison pill protections.
func TestObjectMutation_NegativeAdversarialRefGuardrails(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("override", false, "")
	cmd.Flags().String("reason-code", "", "")
	cmd.Flags().StringArray("add-ref", nil, "")
	cmd.Flags().StringArray("remove-ref", nil, "")
	cmd.Flags().StringArray("unset-field", nil, "")

	// 1. No ref fields -> allowed
	err := guardManualRefFieldUpdates(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"title": "update"})
	assert.NoError(t, err)

	// 2. Direct ref update without override -> rejected with poison pill warning
	err = guardManualRefFieldUpdates(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"milestone_refs": []string{"MIL-1"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AGENT POISON PILL")

	// 3. Direct ref update with override but missing reason-code -> rejected
	err = cmd.Flags().Set("override", "true")
	require.NoError(t, err)
	err = guardManualRefFieldUpdates(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"milestone_refs": []string{"MIL-1"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required")

	// 4. Ref update present in --add-ref is allowed
	err = cmd.Flags().Set("override", "false")
	require.NoError(t, err)
	err = cmd.Flags().Set("add-ref", "milestone_refs=MIL-1")
	require.NoError(t, err)
	err = guardManualRefFieldUpdates(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"milestone_refs": []string{"MIL-1"}})
	assert.NoError(t, err)
}

// TestObjectMutation_NegativeAdversarialProvenanceGuardrails verifies system provenance protection.
func TestObjectMutation_NegativeAdversarialProvenanceGuardrails(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("override", false, "")
	cmd.Flags().String("reason-code", "", "")

	// 1. No provenance fields -> allowed
	err := guardManualSystemProvenanceFields(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"title": "safe"})
	assert.NoError(t, err)

	// 2. Manual created_by without override -> rejected
	err = guardManualSystemProvenanceFields(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"created_by": "hacker"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system provenance field(s)")

	// 3. With override but missing reason-code -> rejected
	err = cmd.Flags().Set("override", "true")
	require.NoError(t, err)
	err = guardManualSystemProvenanceFields(cmd, nil, "BLI-001", objects.KindBacklogItem, map[string]any{"created_at": "fake-time"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required")
}

// TestObjectMutation_BreakGlassAndDryRunHelpers verifies DECIDE break-glass and dry run utilities.
func TestObjectMutation_BreakGlassAndDryRunHelpers(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("override", false, "")
	cmd.Flags().String("reason-code", "", "")

	// 1. withUpdateBreakGlass non-critical kind without flags
	ctx := context.Background()
	outCtx, err := withUpdateBreakGlass(cmd, ctx, "custom_noncritical_kind")
	require.NoError(t, err)
	assert.False(t, pkgctx.IsLifecycleBreakGlass(outCtx))

	// 2. withUpdateBreakGlass with force on non-critical kind succeeds without reason-code
	err = cmd.Flags().Set("force", "true")
	require.NoError(t, err)
	outCtx, err = withUpdateBreakGlass(cmd, ctx, "custom_noncritical_kind")
	require.NoError(t, err)
	assert.True(t, pkgctx.IsLifecycleBreakGlass(outCtx))

	// 3. withUpdateBreakGlass critical kind requires reason-code
	err = cmd.Flags().Set("reason-code", "")
	require.NoError(t, err)
	_, err = withUpdateBreakGlass(cmd, ctx, objects.KindBacklogItem)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required when using --force/--override on critical kinds")

	// Critical kind with reason-code (>=30 chars for core kernel kinds) succeeds
	err = cmd.Flags().Set("reason-code", "POL-ADMIN-EMERGENCY-MAINTENANCE-CYCLE45-ACTIVE")
	require.NoError(t, err)
	outCtx, err = withUpdateBreakGlass(cmd, ctx, objects.KindBacklogItem)
	require.NoError(t, err)
	assert.True(t, pkgctx.IsLifecycleBreakGlass(outCtx))

	// 4. processDryRunResult
	handled, err := processDryRunResult(cmd, false, nil, errors.New("permission denied"))
	assert.Error(t, err)
	assert.False(t, handled)

	handled, err = processDryRunResult(cmd, false, nil, nil)
	assert.NoError(t, err)
	assert.False(t, handled)

	// 5. logSlowCLIObjectMutationFlush safety
	eventLogger := logging.NewEventLogger(context.Background())
	logSlowCLIObjectMutationFlush(nil, "update", "OBJ-1", []string{"goal"}, time.Second, time.Second)
	logSlowCLIObjectMutationFlush(eventLogger, "update", "OBJ-1", []string{"goal"}, 500*time.Millisecond, 500*time.Millisecond)
	logSlowCLIObjectMutationFlush(eventLogger, "update", "", []string{}, 2*time.Second, 1*time.Second)
}
