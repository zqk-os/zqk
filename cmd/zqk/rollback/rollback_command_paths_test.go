package rollback

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeElevationCmd(cmd *cobra.Command, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if cmd.Context() == nil {
		cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf))
	} else {
		cmd.SetContext(pkgctx.WithCommandOutputWriter(cmd.Context(), &buf))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestRollbackElevation_ResolveRollbackStorageAndContext_NilContext(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	cmd := newApplyCmd()
	cmd.SetContext(nil)

	root, provider, ctx, err := resolveRollbackStorageAndContext(cmd)
	require.NoError(t, err)
	assert.Equal(t, proj.Root, root)
	assert.NotNil(t, provider)
	assert.NotNil(t, ctx, "expected default system context when cmd.Context() is nil")
}

func TestRollbackElevation_NoProjectRootErrors(t *testing.T) {
	origDir, err := fileutil.Getwd()
	require.NoError(t, err)
	emptyDir := t.TempDir()
	require.NoError(t, fileutil.Chdir(emptyDir))
	t.Cleanup(func() {
		_ = fileutil.Chdir(origDir)
	})

	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")

	// 1. runList with no project root
	listCmd := newListCmd()
	errList := runList(listCmd, nil)
	require.Error(t, errList)
	assert.Contains(t, errList.Error(), "not a ZQK project (no project root found)")

	// 2. resolveRollbackStorageAndContext with no project root
	applyCmd := newApplyCmd()
	_, _, _, errResolve := resolveRollbackStorageAndContext(applyCmd)
	require.Error(t, errResolve)
	assert.Contains(t, errResolve.Error(), "not a ZQK project (no project root found)")

	// 3. runApply with no project root
	errApply := runApply(applyCmd, []string{"RBK-DEMO-001"})
	require.Error(t, errApply)
	assert.Contains(t, errApply.Error(), "not a ZQK project")

	// 4. runReconstruct with no project root (after supplying required flags)
	recCmd := newReconstructCmd()
	_ = recCmd.Flags().Set("scope-id", "lifecycle:backlog_item:BLI-DEMO-001:complete")
	_ = recCmd.Flags().Set("timestamp", time.Now().UTC().Format(time.RFC3339))
	errRec := runReconstruct(recCmd, nil)
	require.Error(t, errRec)
	assert.Contains(t, errRec.Error(), "not a ZQK project")

	// 5. runRetain with no project root
	retainCmd := newRetainCmd()
	errRetain := runRetain(retainCmd, nil)
	require.Error(t, errRetain)
	assert.Contains(t, errRetain.Error(), "not a ZQK project (no project root found)")
}

func TestRollbackElevation_StorageResolutionFailure(t *testing.T) {
	tempDir := t.TempDir()
	// Create .zqk as a regular file so ResolveProjectRoot (via env) uses tempDir,
	// but NewFileObjectStorage fails because it cannot create/stat the .zqk/process directory.
	dotZqk := filepath.Join(tempDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.WriteFile(dotZqk, []byte("blocking-file-not-a-dir"), paths.FilePerm644))

	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	applyCmd := newApplyCmd()
	_, _, _, errResolve := resolveRollbackStorageAndContext(applyCmd)
	require.Error(t, errResolve)
	assert.Contains(t, errResolve.Error(), "storage")

	errApply := runApply(applyCmd, []string{"RBK-DEMO-001"})
	require.Error(t, errApply)
	assert.Contains(t, errApply.Error(), "storage")

	recCmd := newReconstructCmd()
	_ = recCmd.Flags().Set("scope-id", "lifecycle:backlog_item:BLI-DEMO-001:planned")
	_ = recCmd.Flags().Set("timestamp", time.Now().UTC().Format(time.RFC3339))
	errRec := runReconstruct(recCmd, nil)
	require.Error(t, errRec)
	assert.Contains(t, errRec.Error(), "storage")
}

func TestRollbackElevation_StoreReadAndRetainFailure(t *testing.T) {
	tempDir := t.TempDir()
	dotZqk := filepath.Join(tempDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(dotZqk, paths.DirPerm755))
	// Create .zqk/wal as a regular file so NewStore's MkdirAll fails
	walPath := filepath.Join(dotZqk, paths.WalDir)
	require.NoError(t, fileutil.WriteFile(walPath, []byte("blocking directory creation"), paths.FilePerm644))

	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	listCmd := newListCmd()
	errList := runList(listCmd, nil)
	require.Error(t, errList)
	assert.Contains(t, errList.Error(), "rollback list")

	retainCmd := newRetainCmd()
	errRetain := runRetain(retainCmd, nil)
	require.Error(t, errRetain)
	assert.Contains(t, errRetain.Error(), "rollback retain")
}

func TestRollbackElevation_ListFormatsAndExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Capture sample points
	pointID1, err := rollback.Capture(proj.Root, "test_scope", "SCOPE-CYCLE103-001", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{ID: "BLI-ELEV-001", Kind: "backlog_item", State: map[string]any{"id": "BLI-ELEV-001", "status": "planned"}},
		}, nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, pointID1)

	pointID2, err := rollback.Capture(proj.Root, "test_scope", "SCOPE-CYCLE103-002", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{ID: "BLI-ELEV-002", Kind: "backlog_item", State: map[string]any{"id": "BLI-ELEV-002", "status": "planned"}},
		}, nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, pointID2)

	// Formats: JSON, YAML, Table
	rootCmd := NewRollbackCmd()
	outJSON, errJSON := executeElevationCmd(rootCmd, "list", "--format", "json")
	require.NoError(t, errJSON)
	assert.Contains(t, outJSON, "points")
	assert.Contains(t, outJSON, pointID1)

	rootCmdYAML := NewRollbackCmd()
	outYAML, errYAML := executeElevationCmd(rootCmdYAML, "list", "--format", "yaml")
	require.NoError(t, errYAML)
	assert.Contains(t, outYAML, pointID2)

	rootCmdTable := NewRollbackCmd()
	outTable, errTable := executeElevationCmd(rootCmdTable, "list", "--format", "table")
	require.NoError(t, errTable)
	assert.NotEmpty(t, outTable)

	// Filters: --last and --within
	rootCmdFilter := NewRollbackCmd()
	outFilter, errFilter := executeElevationCmd(rootCmdFilter, "list", "--last", "1", "--within", "24h")
	require.NoError(t, errFilter)
	assert.Contains(t, outFilter, pointID2)
}

func TestRollbackElevation_ApplyCommandPaths(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// 1. Missing point ID argument
	rootCmdNoArgs := NewRollbackCmd()
	_, errNoArgs := executeElevationCmd(rootCmdNoArgs, "apply")
	require.Error(t, errNoArgs)
	assert.Contains(t, errNoArgs.Error(), "point id is required")

	// 2. Point not found (pruned or invalid) provides canonical reconstruction hint
	rootCmdNotFound := NewRollbackCmd()
	_, errNotFound := executeElevationCmd(rootCmdNotFound, "apply", "RBK-NONEXISTENT-001")
	require.Error(t, errNotFound)
	assert.Contains(t, errNotFound.Error(), "zqk rollback reconstruct")

	// 3. Storage update failure (non-not-found error returned directly on line 109)
	pointID, captureErr := rollback.Capture(proj.Root, "test_scope", "SCOPE-CYCLE103-FAIL", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{ID: "BLI-FAIL-001", Kind: "backlog_item", State: map[string]any{"id": "BLI-FAIL-001", "status": "planned"}},
		}, nil
	})
	require.NoError(t, captureErr)

	failingStorage := storage.NewNoopObjectStorage()
	cmdFailing := NewRollbackCmd()
	ctxWithStorage := cliapp.WithStorageProvider(pkgctx.NewSystemContext(), failingStorage)
	cmdFailing.SetContext(ctxWithStorage)
	_, errFailing := executeElevationCmd(cmdFailing, "apply", pointID)
	require.Error(t, errFailing)
	assert.Contains(t, errFailing.Error(), "noop object storage")
	assert.NotContains(t, errFailing.Error(), "zqk rollback reconstruct", "must return raw error when failure is not not-found")

	// 4. Successful apply with JSON and YAML formats
	secCtx := pkgctx.NewSystemSecurityContext()
	createErr := proj.FileStorage.Create(pkgctx.NewSystemContext(), secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-OK-001",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyTitle:        "Apply Baseline Test",
		objects.FieldKeyStatus:       "planned",
		objects.FieldKeyPriorityTier: "P1",
	})
	require.NoError(t, createErr)

	validPointID, captureValidErr := rollback.Capture(proj.Root, "scope", "SCOPE-OK", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{
				ID:   "BLI-OK-001",
				Kind: objects.KindBacklogItem,
				State: map[string]any{
					objects.FieldKeyID:           "BLI-OK-001",
					objects.FieldKeyKind:         objects.KindBacklogItem,
					objects.FieldKeyTitle:        "Updated State In Point",
					objects.FieldKeyStatus:       "planned",
					objects.FieldKeyPriorityTier: "P1",
				},
			},
		}, nil
	})
	require.NoError(t, captureValidErr)

	cmdValidJSON := NewRollbackCmd()
	outJSON, errValidJSON := executeElevationCmd(cmdValidJSON, "apply", validPointID, "--format", "json")
	require.NoError(t, errValidJSON)
	assert.Contains(t, outJSON, validPointID)

	cmdValidYAML := NewRollbackCmd()
	outYAML, errValidYAML := executeElevationCmd(cmdValidYAML, "apply", validPointID, "--format", "yaml")
	require.NoError(t, errValidYAML)
	assert.Contains(t, outYAML, validPointID)
}

func TestRollbackElevation_ReconstructCommandPaths(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// 1. Direct call with empty flags hitting line 131 validation check
	cmdDirect := newReconstructCmd()
	errDirect := runReconstruct(cmdDirect, nil)
	require.Error(t, errDirect)
	assert.Contains(t, errDirect.Error(), "--scope-id and --timestamp are required")

	// 2. Direct call with scope-id present but empty timestamp
	cmdDirectScope := newReconstructCmd()
	_ = cmdDirectScope.Flags().Set("scope-id", "lifecycle:backlog_item:BLI-001:complete")
	errDirectTS := runReconstruct(cmdDirectScope, nil)
	require.Error(t, errDirectTS)
	assert.Contains(t, errDirectTS.Error(), "--scope-id and --timestamp are required")

	// 3. Invalid RFC3339 timestamp
	cmdInvalidTS := newReconstructCmd()
	_ = cmdInvalidTS.Flags().Set("scope-id", "lifecycle:backlog_item:BLI-001:complete")
	_ = cmdInvalidTS.Flags().Set("timestamp", "not-a-valid-timestamp")
	errInvalidTS := runReconstruct(cmdInvalidTS, nil)
	require.Error(t, errInvalidTS)
	assert.Contains(t, errInvalidTS.Error(), "invalid --timestamp (use RFC3339)")

	// 4. No refs found for scope
	nowStr := time.Now().UTC().Format(time.RFC3339)
	cmdNoRefs := newReconstructCmd()
	_ = cmdNoRefs.Flags().Set("scope-type", "non_existent_scope")
	_ = cmdNoRefs.Flags().Set("scope-id", "unknown:ID:status")
	_ = cmdNoRefs.Flags().Set("timestamp", nowStr)
	errNoRefs := runReconstruct(cmdNoRefs, nil)
	require.Error(t, errNoRefs)
	assert.Contains(t, errNoRefs.Error(), "no refs for scope non_existent_scope:unknown:ID:status")

	// 5. ApplyReconstruct error when object not found or storage read fails
	cmdApplyReconstructFail := newReconstructCmd()
	_ = cmdApplyReconstructFail.Flags().Set("scope-type", rollback.ScopeTypeLifecycle)
	_ = cmdApplyReconstructFail.Flags().Set("scope-id", "backlog_item:BLI-NONEXISTENT:complete")
	_ = cmdApplyReconstructFail.Flags().Set("timestamp", nowStr)
	errReconstructFail := runReconstruct(cmdApplyReconstructFail, nil)
	require.Error(t, errReconstructFail)
	assert.Contains(t, errReconstructFail.Error(), "not found")

	// 6. Successful reconstruct execution
	secCtx := pkgctx.NewSystemSecurityContext()
	createErr := proj.FileStorage.Create(pkgctx.NewSystemContext(), secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-REC-OK-001",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyTitle:        "Reconstruct Success Item",
		objects.FieldKeyStatus:       "planned",
		objects.FieldKeyPriorityTier: "P1",
	})
	require.NoError(t, createErr)

	futureTimestamp := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339)

	cmdSuccess := NewRollbackCmd()
	outSuccess, errSuccess := executeElevationCmd(cmdSuccess, "reconstruct",
		"--scope-type", rollback.ScopeTypeLifecycle,
		"--scope-id", "backlog_item:BLI-REC-OK-001:planned",
		"--timestamp", futureTimestamp,
		"--format", "json",
	)
	require.NoError(t, errSuccess)
	assert.Contains(t, outSuccess, `"reconstructed": true`)
	assert.Contains(t, outSuccess, `"object_count": 1`)
	assert.Contains(t, outSuccess, "BLI-REC-OK-001")

	// Also execute in YAML format
	cmdSuccessYAML := NewRollbackCmd()
	outSuccessYAML, errSuccessYAML := executeElevationCmd(cmdSuccessYAML, "reconstruct",
		"--scope-type", rollback.ScopeTypeLifecycle,
		"--scope-id", "backlog_item:BLI-REC-OK-001:planned",
		"--timestamp", futureTimestamp,
		"--format", "yaml",
	)
	require.NoError(t, errSuccessYAML)
	assert.Contains(t, outSuccessYAML, "reconstructed: true")
}

func TestRollbackElevation_RetainCommandPaths(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Capture multiple points to retain
	for i := 1; i <= 3; i++ {
		_, err := rollback.Capture(proj.Root, "scope", fmt.Sprintf("SCOPE-RETAIN-%d", i), func() ([]rollback.ObjectState, error) {
			return []rollback.ObjectState{
				{ID: fmt.Sprintf("BLI-RET-%d", i), Kind: "backlog_item", State: map[string]any{"id": "BLI-RET", "status": "planned"}},
			}, nil
		})
		require.NoError(t, err)
	}

	rootCmdJSON := NewRollbackCmd()
	outJSON, errJSON := executeElevationCmd(rootCmdJSON, "retain", "--format", "json")
	require.NoError(t, errJSON)
	assert.Contains(t, outJSON, `"retained": true`)

	rootCmdYAML := NewRollbackCmd()
	outYAML, errYAML := executeElevationCmd(rootCmdYAML, "retain", "--format", "yaml")
	require.NoError(t, errYAML)
	assert.Contains(t, outYAML, "retained: true")

	rootCmdTable := NewRollbackCmd()
	outTable, errTable := executeElevationCmd(rootCmdTable, "retain", "--format", "table")
	require.NoError(t, errTable)
	assert.NotEmpty(t, outTable)
}

func TestRollbackElevation_RootCommandAndLoggerProfiles(t *testing.T) {
	rootCmd := NewRollbackCmd()
	require.NotNil(t, rootCmd)
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("format"))

	// Test GetLoggerFromProfile with various profiles
	defaultLogger := GetLoggerFromProfile(rootCmd)
	assert.NotNil(t, defaultLogger)

	cmdCustomProfile := NewRollbackCmd()
	customCtx := cliapp.ContextForProjectRoot(".")
	customCtx.Context.Profile = "human"
	cliapp.SetContext(cmdCustomProfile, customCtx)
	humanLogger := GetLoggerFromProfile(cmdCustomProfile)
	assert.NotNil(t, humanLogger)

	customCtxAgent := cliapp.ContextForProjectRoot(".")
	customCtxAgent.Context.Profile = "ai-agent"
	cmdAgentProfile := NewRollbackCmd()
	cliapp.SetContext(cmdAgentProfile, customCtxAgent)
	agentLogger := GetLoggerFromProfile(cmdAgentProfile)
	assert.NotNil(t, agentLogger)
}
