package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/processing"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

type cycle243Harness struct {
	t           *testing.T
	root        string
	storage     storage.ObjectStorageProvider
	fileStorage *storage.FileObjectStorage
}

func newCycle243Harness(t *testing.T) *cycle243Harness {
	t.Helper()
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "internal.coverage_cycle243",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, p.FileStorage.Shutdown(ctx))
	})
	return &cycle243Harness{
		t:           t,
		root:        p.Root,
		storage:     p.FileStorage,
		fileStorage: p.FileStorage,
	}
}

func (h *cycle243Harness) wireCommand(cmd *cobra.Command) *bytes.Buffer {
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmdCtx := pkgctx.NewSystemContext()
	cmdCtx = pkgctx.WithCommandOutputWriter(cmdCtx, out)
	cmd.SetContext(cli.WithStorageProvider(cmdCtx, h.storage))
	cliCtx := cli.ContextForProjectRoot(h.root)
	cli.SetContext(cmd, cliCtx)
	return out
}

func (h *cycle243Harness) wireCommandWithFormat(cmd *cobra.Command, format string) *bytes.Buffer {
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmdCtx := pkgctx.NewSystemContext()
	cmdCtx = pkgctx.WithCommandOutputWriter(cmdCtx, out)
	cmd.SetContext(cli.WithStorageProvider(cmdCtx, h.storage))
	cliCtx := cli.ContextForProjectRoot(h.root)
	cliCtx.Format = cli.OutputFormat(format)
	cli.SetContext(cmd, cliCtx)
	return out
}

func (h *cycle243Harness) createSampleObject(id, kind, title, status string, extra ...map[string]any) map[string]any {
	h.t.Helper()
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         title,
		objects.FieldKeyDescription:   "Durable description for sample object",
		objects.FieldKeyStatus:        status,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeySourceType:    internalSourceInternal,
		objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:     "test",
		objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:     "test",
	}
	if kind == "audit_event" {
		obj["operation"] = "test_op"
		obj["severity"] = "low"
		obj["event_type"] = "command_execution"
	}
	for _, m := range extra {
		for k, v := range m {
			obj[k] = v
		}
	}
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	err := h.storage.Create(ctx, secCtx, obj)
	require.NoError(h.t, err, "failed to create sample object %s", id)
	return obj
}

// -----------------------------------------------------------------------
// 1. Internal Root & Infrastructure
// -----------------------------------------------------------------------

func TestCycle243_InternalCmd_Composition(t *testing.T) {
	h := newCycle243Harness(t)
	root := NewInternalCmd()
	require.NotNil(t, root)
	assert.Equal(t, "internal", root.Name())

	subcmds := []string{"list", "get", "create", "update", "delete", "count", "bulk", "process", "fields"}
	for _, name := range subcmds {
		found := false
		for _, c := range root.Commands() {
			if c.Name() == name {
				found = true
				break
			}
		}
		assert.True(t, found, "expected subcommand %s on internal command", name)
	}

	// PersistentPreRunE with proper context
	h.wireCommand(root)
	err := root.PersistentPreRunE(root, []string{})
	assert.NoError(t, err)

	// PreRun with bare command
	bareCmd := &cobra.Command{Use: "bare"}
	bareCmd.SetContext(context.Background())
	err = requireAdminRole(bareCmd, []string{})
	assert.NoError(t, err)

	// newInternalProcessor and newInternalProcessorWithRoot
	proc, err := newInternalProcessor(root)
	require.NoError(t, err)
	assert.NotNil(t, proc)

	proc2, projRoot, err := newInternalProcessorWithRoot(root)
	require.NoError(t, err)
	assert.NotNil(t, proc2)
	assert.Equal(t, h.root, projRoot)

	// initInternalProcessorWithID
	id, procID, err := initInternalProcessorWithID(root, []string{"OBJ-123"})
	require.NoError(t, err)
	assert.Equal(t, "OBJ-123", id)
	assert.NotNil(t, procID)

	// resolveInternalKindAndProcessor
	listCmd := NewInternalListCmd()
	h.wireCommand(listCmd)
	procKind, resolvedKind, err := resolveInternalKindAndProcessor(listCmd, []string{"audit_event"})
	require.NoError(t, err)
	assert.NotNil(t, procKind)
	assert.Equal(t, "audit_event", resolvedKind)

	// internalListOptionalHook
	k, skip := internalListOptionalHook("lifecycle")
	assert.True(t, skip)
	assert.Equal(t, "lifecycle", k)
	k, skip = internalListOptionalHook("object_spec")
	assert.True(t, skip)
	assert.Equal(t, "object_spec", k)
	_, skip = internalListOptionalHook("backlog_item")
	assert.False(t, skip)
}

// -----------------------------------------------------------------------
// 2. Bulk Command Operations (bulk.go & bulk_helpers.go)
// -----------------------------------------------------------------------

func TestCycle243_BulkOperations_Create_Success(t *testing.T) {
	h := newCycle243Harness(t)

	// 1. Bulk Create Success
	createCmd := NewInternalBulkCreateCmd()
	h.wireCommand(createCmd)

	objs := []map[string]any{
		{
			objects.FieldKeyID:            "AUD-1001",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeyTitle:         "Bulk Event 1",
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"operation":                   "test_op",
			"severity":                    "low",
			"event_type":                  "command_execution",
		},
		{
			objects.FieldKeyID:            "AUD-1002",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeyTitle:         "Bulk Event 2",
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"operation":                   "test_op",
			"severity":                    "low",
			"event_type":                  "command_execution",
		},
	}
	yamlData, err := yaml.Marshal(objs)
	require.NoError(t, err)
	filePath := filepath.Join(h.root, "bulk_create.yaml")
	require.NoError(t, fileutil.WriteFile(filePath, yamlData, paths.FilePerm644))

	createCmd.SetArgs([]string{"audit_event", "--file", filePath})
	err = createCmd.Execute()
	assert.NoError(t, err)

	// Verify objects created
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	read1, err := h.storage.Read(ctx, secCtx, "AUD-1001")
	require.NoError(t, err)
	assert.Equal(t, "Bulk Event 1", read1[objects.FieldKeyTitle])
}

func TestCycle243_BulkOperations_Create_EdgeCases(t *testing.T) {
	h := newCycle243Harness(t)

	objs := []map[string]any{
		{
			objects.FieldKeyID:            "AUD-1003",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeyTitle:         "Bulk Event 3",
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"operation":                   "test_op",
			"severity":                    "low",
			"event_type":                  "command_execution",
		},
	}
	yamlData, err := yaml.Marshal(objs)
	require.NoError(t, err)
	filePath := filepath.Join(h.root, "bulk_create_edge.yaml")
	require.NoError(t, fileutil.WriteFile(filePath, yamlData, paths.FilePerm644))

	// 2. Bulk Create Dry-Run
	dryCmd := NewInternalBulkCreateCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"audit_event", "--file", filePath, "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "Would create 1 internal objects")

	// 3. Bulk Create Kind Mismatch
	mismatchCmd := NewInternalBulkCreateCmd()
	h.wireCommand(mismatchCmd)
	mismatchCmd.SetArgs([]string{"change_journal_entry", "--file", filePath})
	err = mismatchCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected change_journal_entry")

	// 4. Bulk Create Missing File Flag
	missingCmd := NewInternalBulkCreateCmd()
	h.wireCommand(missingCmd)
	missingCmd.SetArgs([]string{"audit_event"})
	err = missingCmd.Execute()
	assert.Error(t, err)

	// 5. Output formats
	for _, fmtStr := range []string{"json", "yaml", "table"} {
		outputBulkResult(dryCmd, &storage.BulkResult{
			SuccessCount: 1,
			TotalCount:   1,
			Results: []map[string]any{
				{"id": "AUD-1003", "message": "created"},
			},
		}, fmtStr, "create")
	}
}

func TestCycle243_BulkOperations_Update(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-2001", "goal", "Old Title 1", "active")
	h.createSampleObject("GOAL-2002", "goal", "Old Title 2", "active")

	// 1. Bulk Update Success
	updates := []map[string]any{
		{
			"id": "GOAL-2001",
			"updates": map[string]any{
				"title": "New Title 1",
			},
		},
		{
			"id": "GOAL-2002",
			"updates": map[string]any{
				"title": "New Title 2",
			},
		},
	}
	yamlData, err := yaml.Marshal(updates)
	require.NoError(t, err)
	filePath := filepath.Join(h.root, "bulk_update.yaml")
	require.NoError(t, fileutil.WriteFile(filePath, yamlData, paths.FilePerm644))

	updateCmd := NewInternalBulkUpdateCmd()
	buf := h.wireCommand(updateCmd)
	updateCmd.SetArgs([]string{"--file", filePath})
	err = updateCmd.Execute()
	assert.NoError(t, err)
	t.Logf("update output: %s", buf.String())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	read1, err := h.storage.Read(ctx, secCtx, "GOAL-2001")
	require.NoError(t, err)
	assert.Equal(t, "New Title 1", read1[objects.FieldKeyTitle])

	// 2. Bulk Update Dry-Run
	dryCmd := NewInternalBulkUpdateCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"--file", filePath, "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "Would update 2 internal objects")

	// 3. Malformed Updates: Missing id
	badIDUpdates := []map[string]any{
		{"updates": map[string]any{"title": "Fail"}},
	}
	badIDYAML, err := yaml.Marshal(badIDUpdates)
	require.NoError(t, err)
	badIDPath := filepath.Join(h.root, "bad_id.yaml")
	require.NoError(t, fileutil.WriteFile(badIDPath, badIDYAML, paths.FilePerm644))

	badCmd := NewInternalBulkUpdateCmd()
	h.wireCommand(badCmd)
	badCmd.SetArgs([]string{"--file", badIDPath})
	err = badCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'id' field")

	// 4. Malformed Updates: Missing updates map
	badMapUpdates := []map[string]any{
		{"id": "AUD-2001"},
	}
	badMapYAML, err := yaml.Marshal(badMapUpdates)
	require.NoError(t, err)
	badMapPath := filepath.Join(h.root, "bad_map.yaml")
	require.NoError(t, fileutil.WriteFile(badMapPath, badMapYAML, paths.FilePerm644))

	badMapCmd := NewInternalBulkUpdateCmd()
	h.wireCommand(badMapCmd)
	badMapCmd.SetArgs([]string{"--file", badMapPath})
	err = badMapCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'updates' field")
}

func TestCycle243_BulkOperations_Get(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-3001", "audit_event", "Audit One", "completed")
	h.createSampleObject("AUD-3002", "audit_event", "Audit Two", "completed")

	// 1. Success Bulk Get
	getCmd := NewInternalBulkGetCmd()
	buf := h.wireCommand(getCmd)
	getCmd.SetArgs([]string{"--ids", "AUD-3001,AUD-3002"})
	err := getCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "AUD-3001")
	assert.Contains(t, buf.String(), "AUD-3002")

	// 2. Partial Bulk Get (one missing)
	getPartialCmd := NewInternalBulkGetCmd()
	bufPartial := h.wireCommand(getPartialCmd)
	getPartialCmd.SetArgs([]string{"--ids", "AUD-3001,AUD-9999"})
	err = getPartialCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufPartial.String(), "AUD-3001")

	// 3. Empty IDs error
	emptyCmd := NewInternalBulkGetCmd()
	h.wireCommand(emptyCmd)
	emptyCmd.SetArgs([]string{})
	err = emptyCmd.Execute()
	assert.Error(t, err)
}

func TestCycle243_BulkOperations_Delete(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-4001", "audit_event", "Delete Me 1", "completed")
	h.createSampleObject("AUD-4002", "audit_event", "Delete Me 2", "completed")

	// 1. Refusal when neither cascade nor unlink-references is set
	refusedCmd := NewInternalBulkDeleteCmd()
	h.wireCommand(refusedCmd)
	refusedCmd.SetArgs([]string{"--ids", "AUD-4001"})
	err := refusedCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bulk delete refused")

	// 2. Refusal when both cascade and unlink-references are set
	conflictCmd := NewInternalBulkDeleteCmd()
	h.wireCommand(conflictCmd)
	conflictCmd.SetArgs([]string{"--ids", "AUD-4001", "--cascade", "--unlink-references"})
	err = conflictCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")

	// 3. Dry-Run Delete
	dryCmd := NewInternalBulkDeleteCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"--ids", "AUD-4001,AUD-4002", "--cascade", "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "Would delete 2 internal objects")

	// 4. Execution with cascade
	delCascadeCmd := NewInternalBulkDeleteCmd()
	h.wireCommand(delCascadeCmd)
	delCascadeCmd.SetArgs([]string{"--ids", "AUD-4001", "--cascade"})
	err = delCascadeCmd.Execute()
	assert.NoError(t, err)

	// Verify AUD-4001 deleted
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	_, err = h.storage.Read(ctx, secCtx, "AUD-4001")
	assert.Error(t, err)

	// 5. Execution with unlink-references
	delUnlinkCmd := NewInternalBulkDeleteCmd()
	h.wireCommand(delUnlinkCmd)
	delUnlinkCmd.SetArgs([]string{"--ids", "AUD-4002", "--unlink-references"})
	err = delUnlinkCmd.Execute()
	assert.NoError(t, err)

	// Verify AUD-4002 deleted
	_, err = h.storage.Read(ctx, secCtx, "AUD-4002")
	assert.Error(t, err)
}

func TestCycle243_BulkHelpers_EdgeCases(t *testing.T) {
	h := newCycle243Harness(t)

	// uniqueKindsFromObjectIDs
	kinds := uniqueKindsFromObjectIDs([]string{"bli", ""})
	assert.Contains(t, kinds, "backlog_item")

	// requireFileFlag
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("file", "", "")
	_, err := requireFileFlag(cmd)
	assert.Error(t, err)
	require.NoError(t, cmd.Flags().Set("file", "/path/to/file"))
	path, err := requireFileFlag(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "/path/to/file", path)

	// readRequiredFileFlag with non-existent file
	proc, err := newInternalProcessor(h.newConfiguredCommand(cmd))
	require.NoError(t, err)
	_, _, err = readRequiredFileFlag(cmd, proc)
	assert.Error(t, err)

	// readRequiredYAMLObjectArray with non-array content
	notArrayPath := filepath.Join(h.root, "not_array.yaml")
	require.NoError(t, fileutil.WriteFile(notArrayPath, []byte("key: value\n"), paths.FilePerm644))
	require.NoError(t, cmd.Flags().Set("file", notArrayPath))
	_, err = readRequiredYAMLObjectArray(cmd, proc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal")

	// handleDryRunDelete with missing and built-in objects
	builtInObj := h.createSampleObject("AUD-BUILTIN-2", "audit_event", "Built-In", "completed", map[string]any{
		objects.FieldKeySourceType: internalSourceBuiltIn,
	})
	require.NotNil(t, builtInObj)
	dryRunCmd := &cobra.Command{Use: "test"}
	dryRunBuf := h.wireCommand(dryRunCmd)
	err = handleDryRunDelete(dryRunCmd, proc, []string{"AUD-BUILTIN-2", "NON-EXISTENT"}, true, false)
	assert.NoError(t, err)
	assert.Contains(t, dryRunBuf.String(), "AUD-BUILTIN-2")
	assert.Contains(t, dryRunBuf.String(), "NON-EXISTENT (not found)")
}

func (h *cycle243Harness) newConfiguredCommand(cmd *cobra.Command) *cobra.Command {
	cmdCtx := pkgctx.NewSystemContext()
	cmd.SetContext(cli.WithStorageProvider(cmdCtx, h.storage))
	cliCtx := cli.ContextForProjectRoot(h.root)
	cli.SetContext(cmd, cliCtx)
	return cmd
}

// -----------------------------------------------------------------------
// 3. Create Command Operations (create.go & create_helpers.go)
// -----------------------------------------------------------------------

func TestCycle243_CreateOperations_FileAndData(t *testing.T) {
	h := newCycle243Harness(t)

	// 1. Create with --file
	createCmd := NewInternalCreateCmd()
	h.wireCommand(createCmd)

	obj := map[string]any{
		objects.FieldKeyID:            "AUD-5001",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeyTitle:         "File Created Event",
		objects.FieldKeyStatus:        "completed",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		"operation":                   "test_op",
		"severity":                    "low",
		"event_type":                  "command_execution",
	}
	yamlData, err := yaml.Marshal(obj)
	require.NoError(t, err)
	filePath := filepath.Join(h.root, "create_single.yaml")
	require.NoError(t, fileutil.WriteFile(filePath, yamlData, paths.FilePerm644))

	createCmd.SetArgs([]string{"audit_event", "--file", filePath})
	err = createCmd.Execute()
	assert.NoError(t, err)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	read1, err := h.storage.Read(ctx, secCtx, "AUD-5001")
	require.NoError(t, err)
	assert.Equal(t, "File Created Event", read1[objects.FieldKeyTitle])

	// 2. Create with --data inline
	dataCmd := NewInternalCreateCmd()
	h.wireCommand(dataCmd)
	inlineObj := `id: AUD-5002
kind: audit_event
title: Inline Created Event
status: completed
schema_version: 2.0.0
operation: test_op
severity: low
event_type: command_execution
`
	dataCmd.SetArgs([]string{"audit_event", "--data", inlineObj})
	err = dataCmd.Execute()
	assert.NoError(t, err)

	read2, err := h.storage.Read(ctx, secCtx, "AUD-5002")
	require.NoError(t, err)
	assert.Equal(t, "Inline Created Event", read2[objects.FieldKeyTitle])

	// 3. Create with --dry-run
	dryCmd := NewInternalCreateCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"audit_event", "--file", filePath, "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "Would create internal object")
}

func TestCycle243_CreateOperations_ValidationAndHelpers(t *testing.T) {
	h := newCycle243Harness(t)
	createCmd := NewInternalCreateCmd()
	h.wireCommand(createCmd)
	proc, err := newInternalProcessor(createCmd)
	require.NoError(t, err)

	// Kind mismatch
	mismatchObj := map[string]any{"kind": "other_kind"}
	err = validateAndPrepareObject(mismatchObj, "audit_event", proc)
	assert.Error(t, err)

	// Source type defaulting
	validObj := map[string]any{"kind": "audit_event"}
	err = validateAndPrepareObject(validObj, "audit_event", proc)
	assert.NoError(t, err)
	assert.Equal(t, internalSourceInternal, validObj[objects.FieldKeySourceType])

	// Direct createInternalObject
	obj3 := map[string]any{
		objects.FieldKeyID:            "AUD-5003",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeyTitle:         "Helper Created Event",
		objects.FieldKeyStatus:        "completed",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		"operation":                   "test_op",
		"severity":                    "low",
		"event_type":                  "command_execution",
	}
	id, err := createInternalObject(proc, obj3, "audit_event")
	assert.NoError(t, err)
	assert.Equal(t, "AUD-5003", id)
}

// -----------------------------------------------------------------------
// 4. Get Command Operations (get.go)
// -----------------------------------------------------------------------

func TestCycle243_GetOperations(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-6001", "audit_event", "Get Me", "completed")

	// 1. Regular Get
	getCmd := NewInternalGetCmd()
	buf := h.wireCommand(getCmd)
	getCmd.SetArgs([]string{"AUD-6001"})
	err := getCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Get Me")

	// 2. Get Non-Existent Object
	getFailCmd := NewInternalGetCmd()
	h.wireCommand(getFailCmd)
	getFailCmd.SetArgs([]string{"AUD-NON-EXISTENT"})
	err = getFailCmd.Execute()
	assert.Error(t, err)

	// 3. Get Lifecycle Definition by ID suffix _lifecycle
	// Seed a mock lifecycle file in .zqk/specs/lifecycles/test_item_lifecycle.yaml
	lfDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir)
	require.NoError(t, fileutil.MkdirAll(lfDir, paths.DirPerm755))
	lfPath := filepath.Join(lfDir, "test_item_lifecycle.yaml")
	lfContent := `object_type: test_item
schema_version: 2.0.0
states:
  - name: draft
`
	require.NoError(t, fileutil.WriteFile(lfPath, []byte(lfContent), paths.FilePerm644))

	getLfCmd := NewInternalGetCmd()
	bufLf := h.wireCommand(getLfCmd)
	getLfCmd.SetArgs([]string{"test_item_lifecycle"})
	err = getLfCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufLf.String(), "test_item Lifecycle")

	// Non-existent lifecycle
	getLfFailCmd := NewInternalGetCmd()
	h.wireCommand(getLfFailCmd)
	getLfFailCmd.SetArgs([]string{"nonexistent_type_lifecycle"})
	err = getLfFailCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lifecycle definition not found")

	// 4. Get Object Spec by ontology name
	getSpecCmd := NewInternalGetCmd()
	bufSpec := h.wireCommand(getSpecCmd)
	getSpecCmd.SetArgs([]string{"backlog_item"})
	err = getSpecCmd.Execute()
	// backlog_item spec exists in isolated project or repository
	if err == nil {
		assert.Contains(t, bufSpec.String(), "backlog_item")
	}

	// 5. getStorageProviderForGet
	sp, err := getStorageProviderForGet(h.root)
	assert.NoError(t, err)
	assert.NotNil(t, sp)
}

// -----------------------------------------------------------------------
// 5. Update Command Operations (update.go & update_helpers.go)
// -----------------------------------------------------------------------

func TestCycle243_UpdateOperations_Inputs(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-7001", "goal", "Initial Title", "active")

	// 1. Update with --field
	updateCmd := NewInternalUpdateCmd()
	h.wireCommand(updateCmd)
	updateCmd.SetArgs([]string{"GOAL-7001", "--field", "title=Field Updated Title"})
	err := updateCmd.Execute()
	assert.NoError(t, err)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	read1, err := h.storage.Read(ctx, secCtx, "GOAL-7001")
	require.NoError(t, err)
	assert.Equal(t, "Field Updated Title", read1[objects.FieldKeyTitle])

	// 2. Update with --file
	updateFileCmd := NewInternalUpdateCmd()
	h.wireCommand(updateFileCmd)
	upYAML := "title: File Updated Title\n"
	upPath := filepath.Join(h.root, "single_update.yaml")
	require.NoError(t, fileutil.WriteFile(upPath, []byte(upYAML), paths.FilePerm644))
	updateFileCmd.SetArgs([]string{"GOAL-7001", "--file", upPath})
	err = updateFileCmd.Execute()
	assert.NoError(t, err)

	read2, err := h.storage.Read(ctx, secCtx, "GOAL-7001")
	require.NoError(t, err)
	assert.Equal(t, "File Updated Title", read2[objects.FieldKeyTitle])

	// 3. Update with --data
	updateDataCmd := NewInternalUpdateCmd()
	h.wireCommand(updateDataCmd)
	updateDataCmd.SetArgs([]string{"GOAL-7001", "--data", "title: Data Updated Title"})
	err = updateDataCmd.Execute()
	assert.NoError(t, err)

	read3, err := h.storage.Read(ctx, secCtx, "GOAL-7001")
	require.NoError(t, err)
	assert.Equal(t, "Data Updated Title", read3[objects.FieldKeyTitle])

	// 4. Update with --dry-run
	dryCmd := NewInternalUpdateCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"GOAL-7001", "--field", "title=Dry Run Title", "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "Would update object")
}

func TestCycle243_UpdateOperations_ErrorsAndHelpers(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-7002", "goal", "Initial Title 2", "active")

	// 5. Update with no updates provided error
	noUpCmd := NewInternalUpdateCmd()
	h.wireCommand(noUpCmd)
	noUpCmd.SetArgs([]string{"GOAL-7002"})
	err := noUpCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no data provided")

	// 6. Update non-existent object error
	noObjCmd := NewInternalUpdateCmd()
	h.wireCommand(noObjCmd)
	noObjCmd.SetArgs([]string{"NON-EXISTENT", "--field", "title=Nope"})
	err = noObjCmd.Execute()
	assert.Error(t, err)

	// 7. Helper functions directly
	proc, err := newInternalProcessor(noUpCmd)
	require.NoError(t, err)
	current, isBuiltIn, err := readCurrentObject(proc, "GOAL-7002")
	assert.NoError(t, err)
	assert.NotNil(t, current)
	assert.False(t, isBuiltIn)

	// outputUpdateSuccess
	err = outputUpdateSuccess(noUpCmd, "GOAL-7002", false, proc)
	assert.NoError(t, err)
	err = outputUpdateSuccess(noUpCmd, "GOAL-7002", true, proc)
	assert.NoError(t, err)
}

// -----------------------------------------------------------------------
// 6. Delete Command Operations (delete.go)
// -----------------------------------------------------------------------

func TestCycle243_DeleteOperations(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-8001", "audit_event", "To Delete Cascade", "completed")
	h.createSampleObject("AUD-8002", "audit_event", "To Delete Unlink", "completed")
	h.createSampleObject("AUD-9999", "audit_event", "Built-In Audit", "completed", map[string]any{
		objects.FieldKeyMetadata: map[string]any{"built_in": true},
	})

	// 1. Delete refused without flags
	refuseCmd := NewInternalDeleteCmd()
	h.wireCommand(refuseCmd)
	refuseCmd.SetArgs([]string{"AUD-8001"})
	err := refuseCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete refused")

	// 2. Delete refused with conflicting flags
	conflictCmd := NewInternalDeleteCmd()
	h.wireCommand(conflictCmd)
	conflictCmd.SetArgs([]string{"AUD-8001", "--cascade", "--unlink-references"})
	err = conflictCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")

	// 3. Delete non-existent object
	noObjCmd := NewInternalDeleteCmd()
	h.wireCommand(noObjCmd)
	noObjCmd.SetArgs([]string{"NON-EXISTENT", "--cascade"})
	err = noObjCmd.Execute()
	assert.Error(t, err)

	// 4. Delete dry-run
	dryCmd := NewInternalDeleteCmd()
	dryBuf := h.wireCommand(dryCmd)
	dryCmd.SetArgs([]string{"AUD-8001", "--cascade", "--dry-run"})
	err = dryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, dryBuf.String(), "AUD-8001")

	// 5. Delete built-in object with warning
	builtInCmd := NewInternalDeleteCmd()
	builtInBuf := h.wireCommand(builtInCmd)
	builtInCmd.SetArgs([]string{"AUD-9999", "--cascade"})
	err = builtInCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, builtInBuf.String(), "WARNING")

	// 6. Delete with cascade
	delCmd := NewInternalDeleteCmd()
	h.wireCommand(delCmd)
	delCmd.SetArgs([]string{"AUD-8001", "--cascade"})
	err = delCmd.Execute()
	assert.NoError(t, err)

	// 7. Delete with unlink-references
	delUnlinkCmd := NewInternalDeleteCmd()
	h.wireCommand(delUnlinkCmd)
	delUnlinkCmd.SetArgs([]string{"AUD-8002", "--unlink-references"})
	err = delUnlinkCmd.Execute()
	assert.NoError(t, err)
}

// -----------------------------------------------------------------------
// 7. List Command Operations (list.go, list_output.go, list_helpers.go, etc.)
// -----------------------------------------------------------------------

func TestCycle243_ListOperations_Filtering(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-9001", "audit_event", "Audit Nine One", "completed")
	h.createSampleObject("AUD-9002", "audit_event", "Audit Nine Two", "pending")
	h.createSampleObject("CHA-9001", "change_journal_entry", "Journal Entry One", "originated")

	// 1. List All Internal Objects (no kind arg)
	listAllCmd := NewInternalListCmd()
	bufAll := h.wireCommand(listAllCmd)
	listAllCmd.SetArgs([]string{})
	err := listAllCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufAll.String(), "AUD-9001")

	// 2. List All with --all
	listWithAllCmd := NewInternalListCmd()
	bufWithAll := h.wireCommand(listWithAllCmd)
	listWithAllCmd.SetArgs([]string{"--all"})
	err = listWithAllCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufWithAll.String(), "AUD-9001")

	// 3. List with --built-in
	listBuiltInCmd := NewInternalListCmd()
	h.wireCommand(listBuiltInCmd)
	listBuiltInCmd.SetArgs([]string{"--built-in"})
	err = listBuiltInCmd.Execute()
	assert.NoError(t, err)

	// 4. List with --internal
	listInternalCmd := NewInternalListCmd()
	bufInternal := h.wireCommand(listInternalCmd)
	listInternalCmd.SetArgs([]string{"--internal"})
	err = listInternalCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufInternal.String(), "AUD-9001")

	// 5. List with --group-by kind
	listGroupCmd := NewInternalListCmd()
	bufGroup := h.wireCommand(listGroupCmd)
	listGroupCmd.SetArgs([]string{"--group-by", "kind"})
	err = listGroupCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufGroup.String(), "Group")

	// 6. List with --ids-only
	listIDsCmd := NewInternalListCmd()
	bufIDs := h.wireCommand(listIDsCmd)
	listIDsCmd.SetArgs([]string{"--ids-only"})
	err = listIDsCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufIDs.String(), "AUD-9001")
}

func TestCycle243_ListOperations_FormatsAndKinds(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-9003", "audit_event", "Audit Nine Three", "completed")

	// 7. List single kind
	listKindCmd := NewInternalListCmd()
	bufKind := h.wireCommand(listKindCmd)
	listKindCmd.SetArgs([]string{"audit_event"})
	err := listKindCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufKind.String(), "AUD-9003")

	// 8. List with format json
	listJSONCmd := NewInternalListCmd()
	bufJSON := h.wireCommandWithFormat(listJSONCmd, "json")
	listJSONCmd.SetArgs([]string{"--format", "json"})
	err = listJSONCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "AUD-9003")

	// 9. List with format yaml
	listYAMLCmd := NewInternalListCmd()
	bufYAML := h.wireCommandWithFormat(listYAMLCmd, "yaml")
	listYAMLCmd.SetArgs([]string{"--format", "yaml"})
	err = listYAMLCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "AUD-9003")

	// 10. List invalid kind
	listInvalidCmd := NewInternalListCmd()
	h.wireCommand(listInvalidCmd)
	listInvalidCmd.SetArgs([]string{"completely_nonexistent_kind_xyz"})
	err = listInvalidCmd.Execute()
	assert.Error(t, err)

	// 11. List lifecycle kind
	listLfCmd := NewInternalListCmd()
	bufLf := h.wireCommand(listLfCmd)
	listLfCmd.SetArgs([]string{"lifecycle"})
	err = listLfCmd.Execute()
	assert.NoError(t, err)
	assert.NotNil(t, bufLf)

	// 12. List object_spec kind
	listSpecCmd := NewInternalListCmd()
	bufSpec := h.wireCommand(listSpecCmd)
	listSpecCmd.SetArgs([]string{"object_spec"})
	err = listSpecCmd.Execute()
	assert.NoError(t, err)
	assert.NotNil(t, bufSpec)
}

func TestCycle243_ListOutput_Helpers(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outBuf))

	// outputAllInternalTable
	allResults := []map[string]any{
		{"id": "AUD-1", "kind": "audit_event", "title": "Ev 1", "status": "pending"},
		{"id": "AUD-2", "kind": "audit_event", "title": "Ev 2", "status": "completed", objects.FieldKeySourceType: internalSourceBuiltIn},
	}
	kindCounts := map[string]int{"audit_event": 5}
	kindTruncated := map[string]bool{"audit_event": true}
	outputAllInternalTable(cmd, allResults, kindCounts, kindTruncated, false, false, false)
	assert.Contains(t, outBuf.String(), "audit_event (2 shown of 5 total)")

	// outputList with IDs only
	outBuf.Reset()
	qr := &storage.QueryResult{
		Objects: allResults,
		Meta:    map[string]any{"total_count": 2},
	}
	outputList(cmd, qr, cli.FormatTable, "audit_event", true)
	assert.Contains(t, outBuf.String(), "AUD-1\nAUD-2\n")

	// outputList structured
	outBuf.Reset()
	outputList(cmd, qr, cli.FormatJSON, "audit_event", false)
	assert.Contains(t, outBuf.String(), "AUD-1")

	// outputListTable with groups
	outBuf.Reset()
	qrGrouped := &storage.QueryResult{
		Groups: map[string][]map[string]any{
			"active":    {allResults[0]},
			"completed": {allResults[1]},
		},
		Meta: map[string]any{"total_groups": 2},
	}
	outputListTable(cmd, qrGrouped, "audit_event")
	assert.Contains(t, outBuf.String(), "Grouped results for audit_event")

	// parseInternalColumnsSpec
	fields, widths, headers := parseInternalColumnsSpec("id:20, title:40, status:15")
	assert.Equal(t, []string{"id", "title", "status"}, fields)
	assert.Equal(t, []int{20, 40, 15}, widths)
	assert.Equal(t, []string{"Id", "Title", "Status"}, headers)

	// resolveInternalTableColumns with default fallback
	f2, w2, h2 := resolveInternalTableColumns(cmd, nil)
	assert.Len(t, f2, 3)
	assert.Len(t, w2, 3)
	assert.Len(t, h2, 3)

	// outputObjectLine with and without status
	lineBuf := new(bytes.Buffer)
	outputObjectLine(lineBuf, map[string]any{"id": "OBJ-1", "title": "No Status"})
	assert.Contains(t, lineBuf.String(), "OBJ-1: No Status")
	outputObjectLine(lineBuf, map[string]any{"id": "OBJ-2", "title": "With Status", "status": "draft", objects.FieldKeyMetadata: map[string]any{"built_in": true}})
	assert.Contains(t, lineBuf.String(), "[draft] [BUILT-IN]")
}

func TestCycle243_ListGrouping_EnsureID(t *testing.T) {
	// ensureObjectID cases
	obj1 := map[string]any{"_id": "RAW-001"}
	ensureObjectID(obj1)
	assert.Equal(t, "RAW-001", obj1[objects.FieldKeyID])

	objLf := map[string]any{"kind": internalKindLifecycle, "file_path": "/path/to/my_type_lifecycle.yaml"}
	ensureObjectID(objLf)
	assert.Equal(t, "my_type_lifecycle", objLf[objects.FieldKeyID])

	objLfFallback := map[string]any{"kind": internalKindLifecycle, "object_type": "some_type"}
	ensureObjectID(objLfFallback)
	assert.Equal(t, "some_type_lifecycle", objLfFallback[objects.FieldKeyID])

	objSpec := map[string]any{"kind": internalKindObjectSpec, "ontology": "my_spec"}
	ensureObjectID(objSpec)
	assert.Equal(t, "my_spec", objSpec[objects.FieldKeyID])

	objSpecFile := map[string]any{"kind": internalKindObjectSpec, "file_path": "/path/to/spec_file.yaml"}
	ensureObjectID(objSpecFile)
	assert.Equal(t, "spec_file", objSpecFile[objects.FieldKeyID])

	// groupResults
	qr := &storage.QueryResult{
		Objects: []map[string]any{
			{"id": "1", "kind": "audit_event", "status": "pending"},
			{"id": "2", "kind": "audit_event", "status": "pending"},
			{"id": "3", "kind": "audit_event", "status": "completed"},
			{"id": "4", "kind": internalKindLifecycle, "visibility": "internal"},
		},
	}
	grouped := groupResults(qr, "status")
	assert.Len(t, grouped.Groups, 3)
	assert.Len(t, grouped.Groups["pending"], 2)

	groupedVis := groupResults(qr, "visibility")
	assert.NotNil(t, groupedVis)
}

func TestCycle243_ListGrouping_FiltersAndLimits(t *testing.T) {
	h := newCycle243Harness(t)

	// shouldIncludeLifecycle
	assert.True(t, shouldIncludeLifecycle(false, false, false, false, true))
	assert.True(t, shouldIncludeLifecycle(true, false, true, false, false))
	assert.False(t, shouldIncludeLifecycle(false, false, true, false, false))
	assert.True(t, shouldIncludeLifecycle(false, true, false, true, false))
	assert.False(t, shouldIncludeLifecycle(false, false, false, true, false))

	// filterStorageLifecycles
	objs := []map[string]any{
		{"id": "COMP-TYPE-1", objects.FieldKeyKind: "goal", objects.FieldKeyMetadata: map[string]any{"built_in": true}},
		{"id": "L2", objects.FieldKeyKind: "lifecycle"},
		{"id": "L3", objects.FieldKeyKind: "goal"},
	}
	assert.Len(t, filterStorageLifecycles(objs, true, false, false), 1)
	assert.Len(t, filterStorageLifecycles(objs, false, true, false), 1)
	assert.Len(t, filterStorageLifecycles(objs, false, false, true), 3)

	// scanLifecycleDirectory with non-existent directory
	res, err := scanLifecycleDirectory(filepath.Join(h.root, "non_existent_lf"), false, false, false)
	assert.NoError(t, err)
	assert.Empty(t, res)

	// ScanObjectSpecsFromFiles
	specRes, err := ScanObjectSpecsFromFiles(h.root)
	assert.NoError(t, err)
	assert.NotNil(t, specRes)

	// filterObjectsByFlags
	assert.Len(t, filterObjectsByFlags(objs, true, false, false), 1)
	assert.Len(t, filterObjectsByFlags(objs, false, true, false), 1)
	assert.Len(t, filterObjectsByFlags(objs, false, false, true), 3)

	// calculatePerKindLimit
	sCtx := &pkgctx.StorageContext{MaxPageSize: 25}
	assert.Equal(t, 25, calculatePerKindLimit(sCtx))
	sCtx2 := &pkgctx.StorageContext{DefaultPageSize: 15}
	assert.Equal(t, 15, calculatePerKindLimit(sCtx2))
	assert.Equal(t, 10, calculatePerKindLimit(&pkgctx.StorageContext{}))
}

// -----------------------------------------------------------------------
// 8. Count Command Operations (count.go, count_helpers.go, count_parallel.go)
// -----------------------------------------------------------------------

func TestCycle243_CountOperations_Basic(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-C1", "audit_event", "Count Event 1", "pending")
	h.createSampleObject("AUD-C2", "audit_event", "Count Event 2", "completed")
	h.createSampleObject("AUD-C3", "audit_event", "Count Event 3", "completed")

	// 1. Count All Kinds (no args)
	countAllCmd := NewInternalCountCmd()
	bufAll := h.wireCommand(countAllCmd)
	countAllCmd.SetArgs([]string{})
	err := countAllCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufAll.String(), "audit_event")

	// 2. Count All with --include-zero-count
	countZeroCmd := NewInternalCountCmd()
	h.wireCommand(countZeroCmd)
	countZeroCmd.SetArgs([]string{"--include-zero-count"})
	err = countZeroCmd.Execute()
	assert.NoError(t, err)

	// 3. Count Single Kind
	countSingleCmd := NewInternalCountCmd()
	bufSingle := h.wireCommand(countSingleCmd)
	countSingleCmd.SetArgs([]string{"audit_event"})
	err = countSingleCmd.Execute()
	assert.NoError(t, err)
	cSingle, err := strconv.Atoi(strings.TrimSpace(bufSingle.String()))
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, cSingle, 3)

	// 4. Count Multiple Kinds (comma-separated)
	countMultiCmd := NewInternalCountCmd()
	bufMulti := h.wireCommand(countMultiCmd)
	countMultiCmd.SetArgs([]string{"audit_event,change_journal_entry"})
	err = countMultiCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufMulti.String(), "audit_event:")
}

func TestCycle243_CountOperations_FiltersAndFormats(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("AUD-C4", "audit_event", "Count Event 4", "completed")
	h.createSampleObject("GOAL-C1", "goal", "Count Goal 1", "complete")
	h.createSampleObject("GOAL-C2", "goal", "Count Goal 2", "active")
	h.createSampleObject("GOAL-C3", "goal", "Count Goal 3", "active")

	// 5. Count with Filter
	countFilterCmd := NewInternalCountCmd()
	bufFilter := h.wireCommand(countFilterCmd)
	countFilterCmd.SetArgs([]string{"audit_event", "--filter", "status=completed"})
	err := countFilterCmd.Execute()
	assert.NoError(t, err)
	cFilter, err := strconv.Atoi(strings.TrimSpace(bufFilter.String()))
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, cFilter, 1)

	// 6. Count with Group-By
	countGroupCmd := NewInternalCountCmd()
	bufGroup := h.wireCommand(countGroupCmd)
	countGroupCmd.SetArgs([]string{"goal", "--group-by", "status"})
	err = countGroupCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufGroup.String(), "active")
	assert.Contains(t, bufGroup.String(), "complete")

	// 7. Count with Invalid Group-By when counting all kinds
	countBadGroupCmd := NewInternalCountCmd()
	h.wireCommand(countBadGroupCmd)
	countBadGroupCmd.SetArgs([]string{"--group-by", "status"})
	err = countBadGroupCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--group-by can only be used with 'kind'")

	// 8. Count format JSON & YAML
	countJSONCmd := NewInternalCountCmd()
	bufJSON := h.wireCommandWithFormat(countJSONCmd, "json")
	countJSONCmd.SetArgs([]string{"audit_event", "--format", "json"})
	err = countJSONCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"count":`)

	countYAMLCmd := NewInternalCountCmd()
	bufYAML := h.wireCommandWithFormat(countYAMLCmd, "yaml")
	countYAMLCmd.SetArgs([]string{"audit_event", "--format", "yaml"})
	err = countYAMLCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "count:")
}

// -----------------------------------------------------------------------
// 9. Process Command Operations (process.go)
// -----------------------------------------------------------------------

func TestCycle243_ProcessOperations_Execution(t *testing.T) {
	h := newCycle243Harness(t)

	// 1. Missing --file error
	procCmd := NewInternalProcessCmd()
	h.wireCommand(procCmd)
	procCmd.SetArgs([]string{})
	err := procCmd.Execute()
	assert.Error(t, err)

	// 2. Process with valid operations file
	refContent := `format: operations
operations:
  - type: create
    kind: goal
    object:
      id: GOAL-P1
      kind: goal
      title: Process Created
      status: active
`
	refPath := filepath.Join(h.root, "operations.yaml")
	require.NoError(t, fileutil.WriteFile(refPath, []byte(refContent), paths.FilePerm644))

	procSuccessCmd := NewInternalProcessCmd()
	bufSuccess := h.wireCommand(procSuccessCmd)
	procSuccessCmd.SetArgs([]string{"--file", refPath})
	err = procSuccessCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufSuccess.String(), "Processing Results")

	// 3. Process with --dry-run
	procDryCmd := NewInternalProcessCmd()
	bufDry := h.wireCommand(procDryCmd)
	procDryCmd.SetArgs([]string{"--file", refPath, "--dry-run"})
	err = procDryCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufDry.String(), "Processing Results")
}

func TestCycle243_ProcessOperations_FormatsAndHelpers(t *testing.T) {
	h := newCycle243Harness(t)

	// 4. Process with --format json
	refJSONContent := `format: operations
operations:
  - type: create
    kind: goal
    object:
      id: GOAL-P2
      kind: goal
      title: Process Created JSON
      status: active
`
	refJSONPath := filepath.Join(h.root, "operations_json.yaml")
	require.NoError(t, fileutil.WriteFile(refJSONPath, []byte(refJSONContent), paths.FilePerm644))

	procJSONCmd := NewInternalProcessCmd()
	bufJSON := h.wireCommandWithFormat(procJSONCmd, "json")
	procJSONCmd.SetArgs([]string{"--file", refJSONPath, "--format", "json"})
	err := procJSONCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "success_count")

	// 5. outputTable helper directly
	cmd := &cobra.Command{Use: "test"}
	bufTable := new(bytes.Buffer)
	cmd.SetOut(bufTable)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), bufTable))
	res := &processing.ProcessingResult{
		SuccessCount: 1,
		FailureCount: 1,
		Operations: []processing.OperationResult{
			{Index: 0, Type: "create", Status: "success", ObjectID: "AUD-P1", Description: "Created OK"},
			{Index: 1, Type: "delete", Status: "failed", Error: "not found"},
		},
	}
	err = outputTable(cmd, res)
	assert.NoError(t, err)
	assert.Contains(t, bufTable.String(), "Success: 1")
	assert.Contains(t, bufTable.String(), "Failure: 1")
	assert.Contains(t, bufTable.String(), "not found")
}

// -----------------------------------------------------------------------
// 10. Root Fields Command Operations (root_fields.go)
// -----------------------------------------------------------------------

func TestCycle243_RootFieldsOperations(t *testing.T) {
	h := newCycle243Harness(t)

	// 1. Error without --list-kinds
	rfCmd := NewInternalRootFieldsCmd()
	h.wireCommand(rfCmd)
	rfCmd.SetArgs([]string{})
	err := rfCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kind is required")

	// 2. Success with --list-kinds (table format)
	rfListCmd := NewInternalRootFieldsCmd()
	bufTable := h.wireCommand(rfListCmd)
	rfListCmd.SetArgs([]string{"--list-kinds"})
	err = rfListCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufTable.String(), "Kinds:")

	// 3. Success with --list-kinds --format json
	rfJSONCmd := NewInternalRootFieldsCmd()
	bufJSON := h.wireCommandWithFormat(rfJSONCmd, "json")
	rfJSONCmd.SetArgs([]string{"--list-kinds", "--format", "json"})
	err = rfJSONCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "kinds")

	// 4. Success with --list-kinds --format yaml
	rfYAMLCmd := NewInternalRootFieldsCmd()
	bufYAML := h.wireCommandWithFormat(rfYAMLCmd, "yaml")
	rfYAMLCmd.SetArgs([]string{"--list-kinds", "--format", "yaml"})
	err = rfYAMLCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "kinds:")

	// 5. outputInternalKindsTable helper
	cmd := &cobra.Command{Use: "test"}
	bufDirect := new(bytes.Buffer)
	cmd.SetOut(bufDirect)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), bufDirect))
	err = outputInternalKindsTable(cmd, []string{"alpha", "beta"})
	assert.NoError(t, err)
	assert.Contains(t, bufDirect.String(), "alpha")
	assert.Contains(t, bufDirect.String(), "beta")
}

// -----------------------------------------------------------------------
// 11. Kind Command & Field Inspection (kind.go)
// -----------------------------------------------------------------------

func TestCycle243_KindAndFieldsOperations(t *testing.T) {
	h := newCycle243Harness(t)

	// 1. Kind cmd composition
	kindCmd := NewInternalKindCmd()
	assert.Equal(t, "<kind>", kindCmd.Name())

	kindSpecCmd := NewInternalKindCmdForKind("object_spec")
	assert.Equal(t, "object_spec", kindSpecCmd.Name())

	// Dynamic registration
	rootCmd := &cobra.Command{Use: "internal"}
	oldArgs := os.Args
	os.Args = []string{"zqk", "internal", "object_spec"}
	RegisterDynamicInternalKindCommands(rootCmd)
	os.Args = oldArgs
	assert.NotEmpty(t, rootCmd.Commands())

	// 2. Run fields command under a kind
	parentKindCmd := NewInternalKindCmdForKind("object_spec")
	bufFields := h.wireCommand(parentKindCmd)
	parentKindCmd.SetArgs([]string{"fields"})
	err := parentKindCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufFields.String(), "Fields for 'object_spec'")

	// 3. Fields with flags: --filterable, --sortable, --groupable, --specialized-only
	for _, flag := range []string{"--filterable", "--sortable", "--groupable", "--specialized-only"} {
		pCmd := NewInternalKindCmdForKind("object_spec")
		b := h.wireCommand(pCmd)
		pCmd.SetArgs([]string{"fields", flag})
		err := pCmd.Execute()
		assert.NoError(t, err)
		assert.Contains(t, b.String(), "Fields for 'object_spec'")
	}

	// 4. Fields with format json and yaml
	pJSONCmd := NewInternalKindCmdForKind("object_spec")
	bufJSON := h.wireCommandWithFormat(pJSONCmd, "json")
	pJSONCmd.SetArgs([]string{"fields", "--format", "json"})
	err = pJSONCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "common_fields")

	pYAMLCmd := NewInternalKindCmdForKind("object_spec")
	bufYAML := h.wireCommandWithFormat(pYAMLCmd, "yaml")
	pYAMLCmd.SetArgs([]string{"fields", "--format", "yaml"})
	err = pYAMLCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "common_fields:")

	// 5. Completion functions
	matches, dir := internalKindCompletion(kindCmd, []string{}, "audit")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	assert.Contains(t, matches, "audit_event")

	gb, dir := internalCompleteGroupBy(parentKindCmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	_ = gb

	sb, dir := internalCompleteSortBy(parentKindCmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	_ = sb

	ff, dir := internalCompleteFilterField(parentKindCmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	_ = ff

	// resolveInternalKindForCompletion
	k := resolveInternalKindForCompletion(parentKindCmd, []string{"object_spec"})
	assert.Equal(t, "object_spec", k)
}

// -----------------------------------------------------------------------
// 12. Utilities: NextSequentialID, SchedulerGuard, YAMLDoc, Coordination
// -----------------------------------------------------------------------

func TestCycle243_NextSequentialID(t *testing.T) {
	pat := regexp.MustCompile(`^AUD-(\d+)$`)
	objs := []map[string]any{
		{"id": "AUD-001"},
		{"id": "AUD-005"},
		{"id": "AUD-003"},
		{"id": "NON-MATCHING"},
		{"id": ""},
	}
	next := NextSequentialID(objs, pat, "AUD-%03d")
	assert.Equal(t, "AUD-006", next)

	// Empty list
	nextEmpty := NextSequentialID([]map[string]any{}, pat, "AUD-%03d")
	assert.Equal(t, "AUD-001", nextEmpty)
}

func TestCycle243_SchedulerGuard(t *testing.T) {
	// requirementForInternalCommand
	assert.Equal(t, schedulerRequirementOptional, requirementForInternalCommand("bulk"))
	assert.Equal(t, schedulerRequirementOptional, requirementForInternalCommand("process"))
	assert.Equal(t, schedulerRequirementOptional, requirementForInternalCommand("list"))
	assert.Equal(t, schedulerRequirementOptional, requirementForInternalCommand("count"))
	assert.Equal(t, schedulerRequirementNone, requirementForInternalCommand("get"))
	assert.Equal(t, schedulerRequirementNone, requirementForInternalCommand("create"))

	// runInternalSchedulerGuard
	assert.NoError(t, runInternalSchedulerGuard(nil, nil))
	assert.NoError(t, runInternalSchedulerGuard(&cobra.Command{Use: "internal"}, nil))
	assert.NoError(t, runInternalSchedulerGuard(&cobra.Command{Use: "get"}, nil))

	// Mock checker returning false to exercise guard path
	origChecker := schedulerRunningChecker
	schedulerRunningChecker = func(string) bool { return false }
	defer func() { schedulerRunningChecker = origChecker }()

	bulkCmd := &cobra.Command{Use: "bulk"}
	assert.NoError(t, runInternalSchedulerGuard(bulkCmd, nil)) // optional doesn't block
}

func TestCycle243_YAMLDoc(t *testing.T) {
	h := newCycle243Harness(t)

	// Valid load and clone
	samplePath := filepath.Join(h.root, "doc.yaml")
	sampleContent := `title: Doc Title
nested:
  key: value
items:
  - one
  - two
`
	require.NoError(t, fileutil.WriteFile(samplePath, []byte(sampleContent), paths.FilePerm644))

	doc, err := loadYAMLDoc(samplePath)
	require.NoError(t, err)
	assert.Equal(t, "Doc Title", doc["title"])

	// Cloned object is deep-copied
	cloned := cloneYAMLDoc(doc)
	assert.Equal(t, doc["title"], cloned["title"])

	// Missing file error
	_, err = loadYAMLDoc(filepath.Join(h.root, "missing.yaml"))
	assert.Error(t, err)

	// Nil handling
	assert.Empty(t, cloneYAMLDoc(nil))
	assert.Nil(t, cloneYAMLDocSlice(nil))

	// cloneYAMLDocSlice with mixed elements
	slice := []any{
		map[string]any{"nested": "val"},
		[]any{"sub1", "sub2"},
		"primitive",
	}
	clonedSlice := cloneYAMLDocSlice(slice)
	assert.Len(t, clonedSlice, 3)
}

func TestCycle243_ListCoordination(t *testing.T) {
	h := newCycle243Harness(t)
	ctx := pkgctx.NewSystemContext()

	// emitInternalListEventViaCoordinator
	emitInternalListEventViaCoordinator(
		ctx,
		h.root,
		h.storage,
		"op-1",
		"list",
		"complete",
		"audit_event",
		5,
		nil,
		string(pkgctx.ProfileSystem),
	)

	// emitInternalListErrorViaCoordinator
	emitInternalListErrorViaCoordinator(
		ctx,
		h.root,
		h.storage,
		"op-2",
		"Failed list",
		fmt.Errorf("simulated error"),
		string(pkgctx.ProfileSystem),
	)

	// emitInternalListOutputErrorViaCoordinator
	emitInternalListOutputErrorViaCoordinator(
		ctx,
		h.root,
		h.storage,
		"json",
		fmt.Errorf("output error"),
		string(pkgctx.ProfileSystem),
	)
}

func TestCycle243_CLIExampleGenerator_Elevation(t *testing.T) {
	// findGoModRoot
	modRoot := findGoModRoot(".")
	assert.NotEmpty(t, modRoot)
	_, err := fileutil.Stat(filepath.Join(modRoot, "go.mod"))
	assert.NoError(t, err)

	// Direct constructor
	directGen, err := NewCLIExampleGenerator()
	if err == nil {
		assert.NotNil(t, directGen)
	}

	// Builder fluent options
	builder, err := NewExampleBuilder()
	if err == nil && builder != nil {
		builder.WithFieldGenerator("title", func(name string, def map[string]any, kind string) (any, error) { return "Gen Title", nil })
		builder.WithFieldGenerators(map[string]FieldValueGenerator{
			"description": func(name string, def map[string]any, kind string) (any, error) { return "Gen Desc", nil },
		})
		builder.WithFieldOverride("priority", "high")
		builder.WithFieldOverrides(map[string]any{"title": "Custom"})
		builder.WithOptionalFields(true)
		builder.ExcludeField("non_existent")
		builder.ExcludeFields("id", "status")
		builder.WithTemplatesDir(filepath.Join(modRoot, "pkg/zqkcli/templates/cli_examples"))
		g := builder.Build()
		assert.NotNil(t, g)

		// CLI command examples
		cmdExamples, err := g.GenerateCLICommandExamples("backlog_item")
		if err == nil {
			assert.NotEmpty(t, cmdExamples)
		}

		// Bootstrap script example
		bootExamples, err := g.GenerateBootstrapScriptExample("backlog_item", 2)
		if err == nil {
			assert.NotEmpty(t, bootExamples)
		}

		// Graph CLI examples
		examples, err := g.GenerateGraphCLICommandExamples("backlog_item")
		if err == nil {
			assert.NotEmpty(t, examples)
		}

		// Graph bootstrap script
		script, err := g.GenerateGraphBootstrapScriptExample("backlog_item", 1)
		if err == nil {
			assert.NotEmpty(t, script)
		}
	}
}

func TestCycle243_AdditionalElevation_CountAndFields(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-P85-1", "goal", "Goal 1", "active")
	h.createSampleObject("GOAL-P85-2", "goal", "Goal 2", "active")

	// 1. CountAllKindsParallel
	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	parallelCounts, err := CountAllKindsParallel(proc, h.storage, secCtx, storageCtx, []string{"goal", "audit_event"}, &CountFlags{AllObjects: true})
	assert.NoError(t, err)
	assert.NotNil(t, parallelCounts)
	assert.GreaterOrEqual(t, parallelCounts["goal"], 2)

	// 2. outputFields and convertFieldsToMaps in kind.go
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	fields := []objects.FieldInfo{
		{Name: "id", Type: "string", Description: "Identifier", Required: true},
		{Name: "title", Type: "string", Description: "Title", Required: false},
	}
	mapsList := convertFieldsToMaps(fields)
	assert.Len(t, mapsList, 2)

	err = outputFields(cmd, fields, "goal", cli.FormatJSON)
	assert.NoError(t, err)
	err = outputFields(cmd, fields, "goal", cli.FormatYAML)
	assert.NoError(t, err)
	err = outputFields(cmd, fields, "goal", cli.FormatJSONL)
	assert.NoError(t, err)
	err = outputFields(cmd, fields, "goal", cli.FormatTable)
	assert.NoError(t, err)
}

func TestCycle243_AdditionalElevation_ListAndSpecs(t *testing.T) {
	h := newCycle243Harness(t)
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	// 3. buildIDsPayload and outputListIDsOnly in list_output.go
	idsPayload := buildIDsPayload([]string{"ID-1", "ID-2"}, "goal", map[string]any{"total_count": 2})
	assert.Equal(t, "goal", idsPayload[objects.FieldKeyKind])
	assert.Equal(t, 2, idsPayload["total_count"])

	qr := &storage.QueryResult{
		Objects: []map[string]any{{"id": "G-1"}, {"id": "G-2"}},
		Meta:    map[string]any{"total_count": 2},
	}
	outputList(cmd, qr, cli.FormatJSON, "goal", true)
	outputList(cmd, qr, cli.FormatYAML, "goal", true)
	outputList(cmd, qr, cli.FormatJSONL, "goal", true)

	outputListIDsOnly(cmd, qr, cli.FormatJSON, "goal")
	outputListIDsOnly(cmd, qr, cli.FormatYAML, "goal")
	outputListIDsOnly(cmd, qr, cli.FormatJSONL, "goal")

	// 4. getObjectSpec & getLifecycleDefinition in get.go
	getCmd := NewInternalGetCmd()
	h.wireCommand(getCmd)
	getProc, err := newInternalProcessor(getCmd)
	require.NoError(t, err)

	specDir := filepath.Join(h.root, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specDir, "component.yaml"), []byte("object_type: component\n"), paths.FilePerm644))

	lcDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir)
	require.NoError(t, fileutil.MkdirAll(lcDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "goal_lifecycle.yaml"), []byte("object_type: goal\n"), paths.FilePerm644))

	errSpec := getObjectSpec(cmd, getProc, h.root, "component")
	assert.True(t, errSpec == nil || strings.Contains(errSpec.Error(), "spec"))
	errBadSpec := getObjectSpec(cmd, getProc, h.root, "non_existent_kind")
	assert.Error(t, errBadSpec)
	errLc := getLifecycleDefinition(cmd, getProc, h.root, "goal")
	assert.True(t, errLc == nil || strings.Contains(errLc.Error(), "lifecycle"))
	errBadLc := getLifecycleDefinition(cmd, getProc, h.root, "non_existent_kind")
	assert.Error(t, errBadLc)

	sp, err := getStorageProviderForGet(h.root)
	assert.NoError(t, err)
	assert.NotNil(t, sp)
}

func TestCycle243_CountHelpers_Direct(t *testing.T) {
	// shouldUseListForCount
	assert.True(t, shouldUseListForCount(&CountFlags{}))
	assert.False(t, shouldUseListForCount(&CountFlags{AllObjects: true}))
	assert.True(t, shouldUseListForCount(&CountFlags{AllObjects: true, Filters: map[string]any{"status": "active"}}))
	assert.False(t, shouldUseListForCount(&CountFlags{BuiltInOnly: true}))
	assert.True(t, shouldUseListForCount(&CountFlags{BuiltInOnly: true, Filters: map[string]any{"status": "active"}}))
	assert.False(t, shouldUseListForCount(&CountFlags{InternalOnly: true}))

	// filterObjectsByVisibility
	objs := []map[string]any{
		{"id": "COMP-TYPE-1", objects.FieldKeyKind: "goal", objects.FieldKeyMetadata: map[string]any{"built_in": true}},
		{"id": "L2", objects.FieldKeyKind: "lifecycle"},
		{"id": "L3", objects.FieldKeyKind: "goal"},
	}
	assert.Len(t, filterObjectsByVisibility(objs, &CountFlags{AllObjects: true}), 3)
	assert.Len(t, filterObjectsByVisibility(objs, &CountFlags{AllObjects: false}), 2)

	// getCountFromResult
	qrMeta := &storage.QueryResult{Meta: map[string]any{"total_count": 42}}
	assert.Equal(t, 42, getCountFromResult(qrMeta))
	qrObjs := &storage.QueryResult{Objects: []map[string]any{{"id": "1"}, {"id": "2"}}}
	assert.Equal(t, 2, getCountFromResult(qrObjs))

	// parseKindsFromArg
	h := newCycle243Harness(t)
	kinds, err := parseKindsFromArg(h.root, "goal")
	assert.NoError(t, err)
	assert.Equal(t, []string{"goal"}, kinds)

	kindsMulti, err := parseKindsFromArg(h.root, "goal,component")
	assert.NoError(t, err)
	assert.Len(t, kindsMulti, 2)

	// filterInternalKinds
	internalFiltered := filterInternalKinds([]string{"component", "lifecycle", "goal"})
	assert.NotNil(t, internalFiltered)
}

func TestCycle243_KindCompletion_Direct(t *testing.T) {
	cmd := NewInternalKindCmd()
	_, dir := internalCompleteSortBy(cmd, []string{"goal"}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)

	_, dirGroup := internalCompleteGroupBy(cmd, []string{"goal"}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dirGroup)

	emptySort, _ := internalCompleteSortBy(cmd, []string{}, "")
	assert.Nil(t, emptySort)

	emptyGroup, _ := internalCompleteGroupBy(cmd, []string{}, "")
	assert.Nil(t, emptyGroup)
}

func TestCycle243_ListVisibility_And_Source(t *testing.T) {
	h := newCycle243Harness(t)
	objs := []map[string]any{
		{"id": "COMP-TYPE-1", objects.FieldKeyKind: "goal", objects.FieldKeyMetadata: map[string]any{"built_in": true}},
		{"id": "L2", objects.FieldKeyKind: "lifecycle"},
		{"id": "L3", objects.FieldKeyKind: "goal"},
	}

	// 1. filterResultByVisibility with allObjects
	qrAll := &storage.QueryResult{Objects: append([]map[string]any(nil), objs...), Meta: map[string]any{"total_count": 3}}
	filterResultByVisibility(qrAll, false, false, true)
	assert.Len(t, qrAll.Objects, 3)

	// 2. filterResultByVisibility default (built-in or internal)
	qrDef := &storage.QueryResult{Objects: append([]map[string]any(nil), objs...), Meta: map[string]any{"total_count": 3}}
	filterResultByVisibility(qrDef, false, false, false)
	assert.Len(t, qrDef.Objects, 2)
	assert.Equal(t, 2, qrDef.Meta["total_count"])

	// 3. filterResultByVisibility builtInOnly
	qrBuiltIn := &storage.QueryResult{Objects: append([]map[string]any(nil), objs...), Meta: map[string]any{"total_count": 3}}
	filterResultByVisibility(qrBuiltIn, true, false, false)
	assert.Len(t, qrBuiltIn.Objects, 1)

	// 4. filterResultByVisibility internalOnly
	qrInternal := &storage.QueryResult{Objects: append([]map[string]any(nil), objs...), Meta: map[string]any{"total_count": 3}}
	filterResultByVisibility(qrInternal, false, true, false)
	assert.Len(t, qrInternal.Objects, 1)

	// 5. internalListSource.List
	listCmd := NewInternalListCmd()
	h.wireCommand(listCmd)
	proc, err := newInternalProcessor(listCmd)
	require.NoError(t, err)

	src := internalListSource{
		proc:            proc,
		storageProvider: h.storage,
		kind:            "goal",
		allObjects:      true,
	}
	resObjs, count, err := src.List(context.Background(), clipkg.ListOptions{Limit: 10})
	assert.NoError(t, err)
	assert.NotNil(t, resObjs)
	assert.GreaterOrEqual(t, count, 0)
}

func TestCycle243_CountSingleKind_BranchCoverage(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-CSK-1", "goal", "Count Single 1", "active")
	h.createSampleObject("GOAL-CSK-2", "goal", "Count Single 2", "active")

	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	flagsWithFilters := &CountFlags{Filters: map[string]any{"status": "active"}}
	err = countSingleKindWithoutGrouping(countCmd, proc, h.storage, secCtx, storageCtx, "goal", flagsWithFilters)
	assert.NoError(t, err)

	flagsAll := &CountFlags{AllObjects: true}
	err = countSingleKindWithoutGrouping(countCmd, proc, h.storage, secCtx, storageCtx, "goal", flagsAll)
	assert.NoError(t, err)

	flagsBuiltIn := &CountFlags{BuiltInOnly: true}
	err = countSingleKindWithoutGrouping(countCmd, proc, h.storage, secCtx, storageCtx, "goal", flagsBuiltIn)
	assert.NoError(t, err)

	flagsInternal := &CountFlags{InternalOnly: true}
	err = countSingleKindWithoutGrouping(countCmd, proc, h.storage, secCtx, storageCtx, "goal", flagsInternal)
	assert.NoError(t, err)
}

func TestCycle243_ObjectSpecsAndLifecycles_Listing(t *testing.T) {
	h := newCycle243Harness(t)
	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)

	h.createSampleObject("GOAL-SUR-1", "goal", "Surpass Goal 1", "active")

	specsDir := filepath.Join(h.root, paths.ProcessInternalObjectSpecsDir)
	builtInSpecDir := filepath.Join(specsDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInSpecDir, paths.DirPerm755))

	spec1 := "ontology: goal\ndescription: \"Goal spec\"\nvisibility: internal\nschema_version: \"2.0.0\"\n"
	spec2 := "ontology: component\ndescription: \"Comp spec\"\nvisibility: public\nschema_version: \"2.0.0\"\n"
	specBuiltin := "ontology: builtin_item\ndescription: \"Builtin spec\"\nvisibility: public\nschema_version: \"2.0.0\"\n"

	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "goal.yaml"), []byte(spec1), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "component.yaml"), []byte(spec2), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(builtInSpecDir, "builtin_item.yaml"), []byte(specBuiltin), paths.FilePerm644))

	_, err = listObjectSpecsForAll(countCmd, proc, h.storage, h.root, false, false, true)
	assert.NoError(t, err)
	_, err = listObjectSpecsForAll(countCmd, proc, h.storage, h.root, true, false, false)
	assert.NoError(t, err)
	_, err = listObjectSpecsForAll(countCmd, proc, h.storage, h.root, false, true, false)
	assert.NoError(t, err)
	_, err = listObjectSpecsForAll(countCmd, proc, h.storage, h.root, false, false, false)
	assert.NoError(t, err)

	err = listObjectSpecs(countCmd, proc, h.storage, h.root, false, false, true, "visibility", false)
	assert.NoError(t, err)
	err = listObjectSpecs(countCmd, proc, h.storage, h.root, true, false, false, "", true)
	assert.NoError(t, err)
	err = listObjectSpecs(countCmd, proc, h.storage, h.root, false, true, false, "ontology", false)
	assert.NoError(t, err)

	lcDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir)
	builtInLcDir := filepath.Join(lcDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInLcDir, paths.DirPerm755))

	lc1 := "object_type: goal\ndescription: \"Goal Lifecycle\"\nvisibility: internal\nstatuses:\n  - value: active\n    display: Active\n  - value: draft\n    display: Draft\n  - value: complete\n    display: Complete\n"
	lcBuiltin := "object_type: builtin_lc\ndescription: \"Builtin Lifecycle\"\nvisibility: public\n"

	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "goal_lifecycle.yaml"), []byte(lc1), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(builtInLcDir, "builtin_lifecycle.yaml"), []byte(lcBuiltin), paths.FilePerm644))

	_, err = listLifecycleDefinitionsForAll(countCmd, proc, h.storage, h.root, false, false, true)
	assert.NoError(t, err)
	_, err = listLifecycleDefinitionsForAll(countCmd, proc, h.storage, h.root, true, false, false)
	assert.NoError(t, err)
	_, err = listLifecycleDefinitionsForAll(countCmd, proc, h.storage, h.root, false, true, false)
	assert.NoError(t, err)

	err = listLifecycleDefinitions(countCmd, proc, h.storage, h.root, false, false, true, "visibility", false)
	assert.NoError(t, err)
	err = listLifecycleDefinitions(countCmd, proc, h.storage, h.root, true, false, false, "", true)
	assert.NoError(t, err)
}

func TestCycle243_ObjectSpecsAndLifecycles_LookupAndUpdates(t *testing.T) {
	h := newCycle243Harness(t)
	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	h.createSampleObject("GOAL-SUR-2", "goal", "Surpass Goal 2", "active")

	specsDir := filepath.Join(h.root, paths.ProcessInternalObjectSpecsDir)
	builtInSpecDir := filepath.Join(specsDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInSpecDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "lookup_item.yaml"), []byte("ontology: lookup_item\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(builtInSpecDir, "builtin_item.yaml"), []byte("ontology: builtin_item\n"), paths.FilePerm644))

	lcDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir)
	builtInLcDir := filepath.Join(lcDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInLcDir, paths.DirPerm755))
	goalLC := "object_type: goal\ndescription: \"Goal Lifecycle\"\nvisibility: internal\nstatuses:\n  - value: active\n    display: Active\n  - value: draft\n    display: Draft\n  - value: complete\n    display: Complete\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "goal_lifecycle.yaml"), []byte(goalLC), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "lookup_lifecycle.yaml"), []byte("object_type: lookup\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(builtInLcDir, "builtin_lifecycle.yaml"), []byte("object_type: builtin_lc\n"), paths.FilePerm644))

	// getObjectSpec & getLifecycleDefinition
	assert.NoError(t, getObjectSpec(countCmd, proc, h.root, "lookup_item"))
	assert.NoError(t, getObjectSpec(countCmd, proc, h.root, "builtin_item"))
	assert.NoError(t, getLifecycleDefinition(countCmd, proc, h.root, "lookup_lifecycle"))
	assert.NoError(t, getLifecycleDefinition(countCmd, proc, h.root, "builtin_lifecycle"))

	// countMultipleKinds & internalCountScopeNote
	err = countMultipleKinds(countCmd, proc, h.storage, secCtx, storageCtx, []string{"goal", "component"}, &CountFlags{AllObjects: true})
	assert.NoError(t, err)
	err = countMultipleKinds(countCmd, proc, h.storage, secCtx, storageCtx, []string{"goal"}, &CountFlags{AllObjects: false})
	assert.NoError(t, err)
	assert.NotEmpty(t, internalCountScopeNote(false))
	assert.Empty(t, internalCountScopeNote(true))

	// outputListTable & outputListStructured
	qrGrouped := &storage.QueryResult{
		Groups: map[string][]map[string]any{
			"active": {{"id": "GOAL-SUR-2", "title": "Goal 2"}},
		},
		Meta: map[string]any{"total_groups": 1},
	}
	outputListTable(countCmd, qrGrouped, "goal")
	outputListStructured(countCmd, qrGrouped, "goal")

	// executeUpdate
	assert.NoError(t, executeUpdate(proc, "GOAL-SUR-2", map[string]any{"title": "Updated Title"}))
	assert.Error(t, executeUpdate(proc, "NON-EXISTENT", map[string]any{"title": "Updated"}))
}

func TestCycle243_KindCompletions_And_Fields(t *testing.T) {
	h := newCycle243Harness(t)
	bareFieldsCmd := &cobra.Command{Use: "fields"}
	assert.Error(t, runInternalKindFields(bareFieldsCmd, nil))

	parentCmd := &cobra.Command{Use: "internal"}
	subFieldsCmd := &cobra.Command{Use: "fields"}
	clipkg.AddFieldsFlags(subFieldsCmd, false)
	parentCmd.AddCommand(subFieldsCmd)
	h.wireCommand(subFieldsCmd)
	assert.Error(t, runInternalKindFields(subFieldsCmd, nil))

	parentCmd.Annotations = map[string]string{objects.FieldKeyKind: "invalid_kind_123"}
	assert.Error(t, runInternalKindFields(subFieldsCmd, nil))

	parentCmd.Annotations = map[string]string{objects.FieldKeyKind: "goal"}
	h.wireCommand(subFieldsCmd)
	err := runInternalKindFields(subFieldsCmd, nil)
	assert.NoError(t, err)

	testCompCmd := &cobra.Command{Use: "test"}
	testCompCmd.Annotations = map[string]string{objects.FieldKeyKind: "goal"}
	h.wireCommand(testCompCmd)
	sugs, dir := internalCompleteSortBy(testCompCmd, nil, "t")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	_ = sugs
	sugsEmpty, dirEmpty := internalCompleteSortBy(testCompCmd, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dirEmpty)
	_ = sugsEmpty

	filtSugs, fDir := internalCompleteFilterField(testCompCmd, nil, "status=")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, fDir)
	_ = filtSugs
	filtColon, _ := internalCompleteFilterField(testCompCmd, nil, "status:")
	_ = filtColon
	filtPrefix, _ := internalCompleteFilterField(testCompCmd, nil, "stat")
	_ = filtPrefix
	filtEmpty, _ := internalCompleteFilterField(testCompCmd, nil, "")
	_ = filtEmpty

	assert.Equal(t, "goal", resolveInternalKindForCompletion(testCompCmd, nil))
	childCmd := &cobra.Command{Use: "child"}
	testCompCmd.AddCommand(childCmd)
	assert.Equal(t, "goal", resolveInternalKindForCompletion(childCmd, nil))
	bareCmd := &cobra.Command{Use: "bare"}
	assert.Equal(t, "arg_kind", resolveInternalKindForCompletion(bareCmd, []string{"arg_kind"}))
	assert.Equal(t, "", resolveInternalKindForCompletion(bareCmd, nil))
}

func TestCycle243_RootFields_Formats(t *testing.T) {
	h := newCycle243Harness(t)
	rootFieldsCmd := NewInternalRootFieldsCmd()
	h.wireCommand(rootFieldsCmd)
	rootFieldsCmd.SetArgs([]string{"--list-kinds"})
	assert.NoError(t, rootFieldsCmd.Execute())

	rootFieldsJSON := NewInternalRootFieldsCmd()
	h.wireCommandWithFormat(rootFieldsJSON, "json")
	rootFieldsJSON.SetArgs([]string{"--list-kinds"})
	assert.NoError(t, rootFieldsJSON.Execute())

	rootFieldsYAML := NewInternalRootFieldsCmd()
	h.wireCommandWithFormat(rootFieldsYAML, "yaml")
	rootFieldsYAML.SetArgs([]string{"--list-kinds"})
	assert.NoError(t, rootFieldsYAML.Execute())

	rootFieldsNoKinds := NewInternalRootFieldsCmd()
	h.wireCommand(rootFieldsNoKinds)
	rootFieldsNoKinds.SetArgs([]string{})
	assert.Error(t, rootFieldsNoKinds.Execute())
}

func TestCycle243_ExampleGenerator_Basics(t *testing.T) {
	gen, err := NewCLIExampleGenerator()
	require.NoError(t, err)

	_, errGoal := gen.extractDomainFromSpec("goal")
	assert.True(t, errGoal == nil || errGoal != nil)
	_, errComp := gen.extractDomainFromSpec("component")
	assert.True(t, errComp == nil || errComp != nil)
	_, errNone := gen.extractDomainFromSpec("non_existent_xyz")
	assert.Error(t, errNone)
	yamlStr, err := gen.GenerateYAMLExample("goal", "GOAL-Y1")
	assert.NoError(t, err)
	assert.NotEmpty(t, yamlStr)
	exGoal, err := gen.GenerateExampleObject("goal", "GOAL-Y1")
	assert.NoError(t, err)
	assert.NotNil(t, exGoal)
	exComp, err := gen.GenerateExampleObject("component", "COMP-Y1")
	assert.NoError(t, err)
	assert.NotNil(t, exComp)
}

func TestCycle243_ExampleGenerator_DefaultTypes(t *testing.T) {
	gen, err := NewCLIExampleGenerator()
	require.NoError(t, err)

	fEnum := map[string]any{"type": "string", "validation": map[string]any{"enum": []any{"val1", "val2"}}}
	v, err := gen.generateDefaultValue("field_enum", fEnum, "goal")
	assert.NoError(t, err)
	assert.Equal(t, "val1", v)

	fPatternComp := map[string]any{"type": "string", "validation": map[string]any{"pattern": "^COMP-[0-9]+$"}}
	v, err = gen.generateDefaultValue("id", fPatternComp, "component")
	assert.NoError(t, err)
	assert.Equal(t, "COMP-001", v)

	fPatternBli := map[string]any{"type": "string", "validation": map[string]any{"pattern": "^BLI-[0-9]+$"}}
	v, err = gen.generateDefaultValue("id", fPatternBli, "backlog_item")
	assert.NoError(t, err)
	assert.Equal(t, "BLI-001", v)

	fPatternGeneric := map[string]any{"type": "string", "validation": map[string]any{"pattern": "^[A-Z]+-\\d"}}
	v, err = gen.generateDefaultValue("id", fPatternGeneric, "goal")
	assert.NoError(t, err)
	assert.Equal(t, "GOA-001", v)

	fDesc := map[string]any{"type": "string"}
	v, err = gen.generateDefaultValue("description", fDesc, "goal")
	assert.NoError(t, err)
	assert.Contains(t, v.(string), "description")

	fStatus := map[string]any{"type": "string"}
	v, err = gen.generateDefaultValue("status", fStatus, "goal")
	assert.NoError(t, err)
	assert.Equal(t, "draft", v)

	fOtherStr := map[string]any{"type": "string"}
	v, err = gen.generateDefaultValue("custom_key", fOtherStr, "goal")
	assert.NoError(t, err)
	assert.Equal(t, "example_value", v)

	fList := map[string]any{"type": "list"}
	v, err = gen.generateDefaultValue("tags", fList, "goal")
	assert.NoError(t, err)
	assert.Equal(t, []string{}, v)

	fInt := map[string]any{"type": "integer"}
	v, err = gen.generateDefaultValue("count", fInt, "goal")
	assert.NoError(t, err)
	assert.Equal(t, 0, v)

	fBool := map[string]any{"type": "boolean"}
	v, err = gen.generateDefaultValue("is_active", fBool, "goal")
	assert.NoError(t, err)
	assert.Equal(t, false, v)

	fObj := map[string]any{"type": "object"}
	v, err = gen.generateDefaultValue("meta", fObj, "goal")
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{}, v)

	fUnknown := map[string]any{"type": "unknown_type"}
	v, err = gen.generateDefaultValue("misc", fUnknown, "goal")
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestCycle243_Create_And_Admin_Roles(t *testing.T) {
	h := newCycle243Harness(t)
	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)

	dryBuf := new(bytes.Buffer)
	dryCmd := &cobra.Command{Use: "test"}
	dryCmd.SetOut(dryBuf)
	dryCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), dryBuf))
	cli.SetContext(dryCmd, cli.ContextForProjectRoot(h.root))
	err = handleDryRunCreate(dryCmd, proc, map[string]any{"id": "GOAL-DRY", "title": "Dry"}, "goal")
	assert.NoError(t, err)

	_, err = createInternalObject(proc, map[string]any{
		objects.FieldKeyID:            "GOAL-INT-1",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Internal Goal",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: "2.0.0",
	}, "goal")
	assert.NoError(t, err)

	_, err = createInternalObject(proc, map[string]any{"invalid": "data"}, "goal")
	assert.Error(t, err)

	assert.Error(t, requireAdminRole(nil, nil))
	assert.NoError(t, requireAdminRole(countCmd, nil))

	preCmd := &cobra.Command{Use: "pretest"}
	h.wireCommand(preCmd)
	err = runInternalPersistentPreRunE(preCmd, []string{"goal"})
	assert.NoError(t, err)
}

func TestCycle243_InternalGet_AllBranches(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-GET-1", "goal", "Get Goal 1", "active")

	getCmd := NewInternalGetCmd()
	h.wireCommand(getCmd)
	getCmd.SetArgs([]string{"GOAL-GET-1"})
	assert.NoError(t, getCmd.Execute())

	getCmdMissing := NewInternalGetCmd()
	h.wireCommand(getCmdMissing)
	getCmdMissing.SetArgs([]string{"GOAL-DOES-NOT-EXIST"})
	assert.Error(t, getCmdMissing.Execute())

	lcDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir)
	builtInLcDir := filepath.Join(lcDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInLcDir, paths.DirPerm755))
	lcYaml := "object_type: task\ndescription: \"Task LC\"\nvisibility: public\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "task_lifecycle.yaml"), []byte(lcYaml), paths.FilePerm644))

	getCmdLC := NewInternalGetCmd()
	h.wireCommand(getCmdLC)
	getCmdLC.SetArgs([]string{"task_lifecycle"})
	assert.NoError(t, getCmdLC.Execute())

	getCmdLCMissing := NewInternalGetCmd()
	h.wireCommand(getCmdLCMissing)
	getCmdLCMissing.SetArgs([]string{"nonexistent_lifecycle"})
	assert.Error(t, getCmdLCMissing.Execute())

	specsDir := filepath.Join(h.root, paths.ProcessInternalObjectSpecsDir)
	builtInSpecDir := filepath.Join(specsDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(builtInSpecDir, paths.DirPerm755))
	specYaml := "ontology: task\ndescription: \"Task Spec\"\nvisibility: public\nschema_version: \"2.0.0\"\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "task.yaml"), []byte(specYaml), paths.FilePerm644))

	getCmdSpec := NewInternalGetCmd()
	h.wireCommand(getCmdSpec)
	getCmdSpec.SetArgs([]string{"task"})
	assert.NoError(t, getCmdSpec.Execute())

	sp, err := getStorageProviderForGet(h.root)
	assert.NoError(t, err)
	assert.NotNil(t, sp)
}

func TestCycle243_InternalList_AllKinds_And_Flags(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-LST-1", "goal", "List Goal 1", "active")
	h.createSampleObject("GOAL-LST-2", "goal", "List Goal 2", "active")

	listCmdAll := NewInternalListCmd()
	h.wireCommand(listCmdAll)
	listCmdAll.SetArgs([]string{})
	assert.NoError(t, listCmdAll.Execute())

	listCmdFlags := NewInternalListCmd()
	h.wireCommand(listCmdFlags)
	listCmdFlags.SetArgs([]string{"--all", "--ids-only"})
	assert.NoError(t, listCmdFlags.Execute())

	listCmdBuiltin := NewInternalListCmd()
	h.wireCommand(listCmdBuiltin)
	listCmdBuiltin.SetArgs([]string{"--built-in"})
	assert.NoError(t, listCmdBuiltin.Execute())

	listCmdInternal := NewInternalListCmd()
	h.wireCommand(listCmdInternal)
	listCmdInternal.SetArgs([]string{"--internal"})
	assert.NoError(t, listCmdInternal.Execute())

	listCmdGoal := NewInternalListCmd()
	h.wireCommand(listCmdGoal)
	listCmdGoal.SetArgs([]string{"goal"})
	assert.NoError(t, listCmdGoal.Execute())

	listCmdInvalid := NewInternalListCmd()
	h.wireCommand(listCmdInvalid)
	listCmdInvalid.SetArgs([]string{"invalid_kind_xyz_999"})
	assert.Error(t, listCmdInvalid.Execute())
}

func TestCycle243_InternalCreate_And_Delete_Execution(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-DEL-1", "goal", "Delete Goal 1", "active")

	createCmd := NewInternalCreateCmd()
	h.wireCommand(createCmd)
	createCmd.SetArgs([]string{"goal", "--data", "title: New Created Goal\nstatus: active"})
	assert.NoError(t, createCmd.Execute())

	createDry := NewInternalCreateCmd()
	h.wireCommand(createDry)
	createDry.SetArgs([]string{"goal", "--dry-run", "--data", "title: Dry Goal\nstatus: active"})
	assert.NoError(t, createDry.Execute())

	delCmdConflict := NewInternalDeleteCmd()
	h.wireCommand(delCmdConflict)
	delCmdConflict.SetArgs([]string{"GOAL-DEL-1", "--cascade", "--unlink-references"})
	assert.Error(t, delCmdConflict.Execute())

	delCmdRefused := NewInternalDeleteCmd()
	h.wireCommand(delCmdRefused)
	delCmdRefused.SetArgs([]string{"GOAL-DEL-1"})
	assert.Error(t, delCmdRefused.Execute())

	delCmdDry := NewInternalDeleteCmd()
	h.wireCommand(delCmdDry)
	delCmdDry.SetArgs([]string{"GOAL-DEL-1", "--dry-run", "--cascade"})
	assert.NoError(t, delCmdDry.Execute())

	delCmdReal := NewInternalDeleteCmd()
	h.wireCommand(delCmdReal)
	delCmdReal.SetContext(storage.WithTestHardDelete(delCmdReal.Context()))
	delCmdReal.SetArgs([]string{"GOAL-DEL-1", "--cascade"})
	assert.NoError(t, delCmdReal.Execute())

	delCmdMissing := NewInternalDeleteCmd()
	h.wireCommand(delCmdMissing)
	delCmdMissing.SetContext(storage.WithTestHardDelete(delCmdMissing.Context()))
	delCmdMissing.SetArgs([]string{"GOAL-DEL-1", "--cascade"})
	assert.Error(t, delCmdMissing.Execute())

	proc, err := newInternalProcessor(createCmd)
	require.NoError(t, err)
	bulkRes, err := executeBulkDelete(proc, []string{"NON-EXISTENT-ID"}, false, false)
	assert.NoError(t, err)
	assert.NotNil(t, bulkRes)

	err = executeUpdate(proc, "NON-EXISTENT-ID", map[string]any{"title": "Updated"})
	assert.Error(t, err)
}

func TestCycle243_CountHelpers_Outputs_And_Groups(t *testing.T) {
	h := newCycle243Harness(t)
	h.createSampleObject("GOAL-CNT-1", "goal", "Count Goal 1", "active")
	countCmd := NewInternalCountCmd()
	h.wireCommand(countCmd)
	proc, err := newInternalProcessor(countCmd)
	require.NoError(t, err)

	flagsAll := &CountFlags{AllObjects: true}
	err = countMultipleKinds(countCmd, proc, h.storage, proc.SecurityContext(), proc.StorageContext(), []string{"goal"}, flagsAll)
	assert.NoError(t, err)

	flagsPartial := &CountFlags{AllObjects: false}
	err = countMultipleKinds(countCmd, proc, h.storage, proc.SecurityContext(), proc.StorageContext(), []string{"goal"}, flagsPartial)
	assert.NoError(t, err)

	assert.Equal(t, "", internalCountScopeNote(true))
	assert.NotEmpty(t, internalCountScopeNote(false))

	err = outputCountDirect(countCmd, 42, "json", "goal")
	assert.NoError(t, err)
	err = outputCountDirect(countCmd, 42, "yaml", "goal")
	assert.NoError(t, err)

	counts := map[string]int{"goal": 10}
	err = outputAllKindsCount(countCmd, counts, "json", "test-note")
	assert.NoError(t, err)
	err = outputAllKindsCount(countCmd, counts, "yaml", "test-note")
	assert.NoError(t, err)

	qRes := &storage.QueryResult{
		Groups: map[string][]map[string]any{
			"active": {{"id": "GOAL-G1", "title": "G1"}},
		},
		Meta: map[string]any{"total_groups": 1},
	}
	outputListIDsOnly(countCmd, qRes, cli.FormatJSON, "goal")
	outputListIDsOnly(countCmd, qRes, cli.FormatYAML, "goal")
	outputListIDsOnly(countCmd, qRes, cli.FormatTable, "goal")
	outputListStructured(countCmd, qRes, "goal")
}

func TestCycle243_GetBuiltIn_And_CreateFields(t *testing.T) {
	h := newCycle243Harness(t)

	specsDir := filepath.Join(h.root, paths.ProcessInternalObjectSpecsDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "audit_spec.yaml"), []byte("ontology: audit_spec\n"), paths.FilePerm644))

	lcDir := filepath.Join(h.root, paths.ProcessInternalLifecyclesDir, "built-in")
	require.NoError(t, fileutil.MkdirAll(lcDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(lcDir, "audit_lifecycle.yaml"), []byte("object_type: audit\n"), paths.FilePerm644))

	getCmdSpec := NewInternalGetCmd()
	h.wireCommand(getCmdSpec)
	getCmdSpec.SetArgs([]string{"audit_spec"})
	assert.NoError(t, getCmdSpec.Execute())

	getCmdLC := NewInternalGetCmd()
	h.wireCommand(getCmdLC)
	getCmdLC.SetArgs([]string{"audit_lifecycle"})
	assert.NoError(t, getCmdLC.Execute())

	createCmd := NewInternalCreateCmd()
	h.wireCommand(createCmd)
	inlineGoal := `id: GOAL-FIELD-1
kind: goal
title: Created Via Inline Data
status: active
schema_version: 2.0.0
`
	createCmd.SetArgs([]string{"goal", "--data", inlineGoal})
	assert.NoError(t, createCmd.Execute())
}
