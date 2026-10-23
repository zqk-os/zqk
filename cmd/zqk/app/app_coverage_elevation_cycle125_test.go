package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/policy"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

func TestWriteGoroutineProfile_Comprehensive(t *testing.T) {
	err := writeGoroutineProfile("")
	require.NoError(t, err)

	tmp := filepath.Join(t.TempDir(), "test_goroutine.pprof")
	err = writeGoroutineProfile(tmp)
	require.NoError(t, err)
	assert.FileExists(t, tmp)

	invalidPath := filepath.Join(t.TempDir(), "nonexistent", "sub", "test.pprof")
	err = writeGoroutineProfile(invalidPath)
	require.Error(t, err)
}

func TestRootPreRunInitFileLogging_SkipCases(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}

	rootPreRunInitFileLogging(cmd, "", false)
	rootPreRunInitFileLogging(cmd, t.TempDir(), true)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"zqk", "system", "init"}
	rootPreRunInitFileLogging(cmd, t.TempDir(), false)

	os.Args = []string{"zqk", "init"}
	rootPreRunInitFileLogging(cmd, t.TempDir(), false)
}

func TestRootPreRunInitFileLogging_Profiles(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	cmd := &cobra.Command{Use: "status"}
	cmd.Flags().String("context", "ai-agent", "")
	rootPreRunInitFileLogging(cmd, proj.Root, false)

	cmd2 := &cobra.Command{Use: "status"}
	cmd2.Flags().String("context", "debug", "")
	rootPreRunInitFileLogging(cmd2, proj.Root, false)

	cmd3 := &cobra.Command{Use: "status"}
	cmd3.Flags().String("verbose", "true", "")
	rootPreRunInitFileLogging(cmd3, proj.Root, false)
}

func TestRootPreRunInitFileLogging_McpAndConfig(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	cfg := map[string]any{
		"profile": "ai-agent",
		"logging": map[string]any{
			"profile": "debug",
			"level":   "warn",
		},
	}
	cfgData, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	cfgPath := filepath.Join(proj.Root, "config", paths.ZqkConfigFileName)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(cfgPath)))
	require.NoError(t, fileutil.WriteStandardFile(cfgPath, cfgData))

	cmd := &cobra.Command{Use: "status"}
	rootPreRunInitFileLogging(cmd, proj.Root, false)

	t.Setenv("ZQK_MCP_ACCOUNT_ID", "acc-mcp-test")
	rootPreRunInitFileLogging(cmd, proj.Root, false)

	parent := &cobra.Command{Use: "mcp"}
	serveCmd := &cobra.Command{Use: "serve"}
	parent.AddCommand(serveCmd)
	rootPreRunInitFileLogging(serveCmd, proj.Root, false)
}

func TestRootPersistentPreRunE_HelpAndCompletion(t *testing.T) {
	helpCmd := &cobra.Command{Use: "help"}
	err := rootPersistentPreRunE(helpCmd, nil)
	require.NoError(t, err)

	parent := &cobra.Command{Use: "completion"}
	subCmd := &cobra.Command{Use: "bash"}
	parent.AddCommand(subCmd)
	err = rootPersistentPreRunE(subCmd, nil)
	require.NoError(t, err)
}

func TestRootPersistentPreRunE_CPUProfileFlags(t *testing.T) {
	cpuProf := filepath.Join(t.TempDir(), "test_cpu.prof")
	cmd := &cobra.Command{Use: "help"}
	cmd.Flags().String("cpuprofile", cpuProf, "")

	err := rootPersistentPreRunE(cmd, nil)
	require.NoError(t, err)
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		err = cpuProfileFile.Close()
		require.NoError(t, err)
		cpuProfileFile = nil
	}

	invalidProf := filepath.Join(t.TempDir(), "nonexistent", "deep", "cpu.prof")
	cmd2 := &cobra.Command{Use: "help"}
	cmd2.Flags().String("cpu-profile", invalidProf, "")
	err = rootPersistentPreRunE(cmd2, nil)
	require.NoError(t, err)
	assert.Nil(t, cpuProfileFile)
}

func TestRootPersistentPreRunE_GoroutineProfileFlags(t *testing.T) {
	goroutineProfilePath = ""
	cmd := &cobra.Command{Use: "help"}
	cmd.Flags().String("goroutine-profile", "/tmp/gr.prof", "")

	err := rootPersistentPreRunE(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/gr.prof", goroutineProfilePath)
	goroutineProfilePath = ""

	cmd2 := &cobra.Command{Use: "help"}
	cmd2.Flags().String("goroutines-profile", "/tmp/gr2.prof", "")
	err = rootPersistentPreRunE(cmd2, nil)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/gr2.prof", goroutineProfilePath)
	goroutineProfilePath = ""
}

func TestRootPersistentPostRunE_ProfilingAndBypass(t *testing.T) {
	cmdHelp := &cobra.Command{Use: "help"}
	err := rootPersistentPostRunE(cmdHelp, nil)
	require.NoError(t, err)

	cmdVer := &cobra.Command{Use: "version"}
	err = rootPersistentPostRunE(cmdVer, nil)
	require.NoError(t, err)

	cmdNilCtx := &cobra.Command{Use: "status"}
	err = rootPersistentPostRunE(cmdNilCtx, nil)
	require.NoError(t, err)

	cmdNoTracker := &cobra.Command{Use: "status"}
	cmdNoTracker.SetContext(context.Background())
	err = rootPersistentPostRunE(cmdNoTracker, nil)
	require.NoError(t, err)
}

func TestRootPersistentPostRunE_ProfileTeardown(t *testing.T) {
	profPath := filepath.Join(t.TempDir(), "post_goroutine.pprof")
	goroutineProfilePath = profPath

	cmd := &cobra.Command{Use: "help"}
	err := rootPersistentPostRunE(cmd, nil)
	require.NoError(t, err)
	assert.Empty(t, goroutineProfilePath)
	assert.FileExists(t, profPath)

	cpuTmp := filepath.Join(t.TempDir(), "post_cpu.prof")
	f, err := fileutil.Create(cpuTmp)
	require.NoError(t, err)
	err = pprof.StartCPUProfile(f)
	require.NoError(t, err)
	cpuProfileFile = f

	err = rootPersistentPostRunE(cmd, nil)
	require.NoError(t, err)
	assert.Nil(t, cpuProfileFile)
}

func TestRootPersistentPostRunE_WithSessionTouch(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("project-root", proj.Root, "")
	require.NoError(t, cmd.Flags().Set("project-root", proj.Root))

	tracker := clitool.NewCommandExecutionTracker()
	ctx := context.Background()
	ctx = WithZqkSessionID(ctx, "ZS-TEST-002")
	ctx = clitool.WithTracker(ctx, tracker)
	ctx = cli.WithStorageProvider(ctx, proj.FileStorage)
	cmd.SetContext(ctx)

	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	t.Setenv("ZQK_TEST_ROOT", proj.Root)
	err := rootPersistentPostRunE(cmd, nil)
	require.NoError(t, err)
}

func TestCompletionBuilder_Comprehensive(t *testing.T) {
	root := &cobra.Command{Use: "zqk"}
	sub := &cobra.Command{Use: "status"}
	root.AddCommand(sub)

	cmd := NewCompletionCmd(root)
	assert.Equal(t, "completion", cmd.Name())
	root.AddCommand(cmd)

	shells := DefaultShells()
	for _, s := range shells {
		var buf bytes.Buffer
		err := s.GenCompletion(root, &buf)
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	}

	var psBuf bytes.Buffer
	err := root.GenPowerShellCompletion(&psBuf)
	require.NoError(t, err)
	assert.NotEmpty(t, psBuf.String())
}

func TestCompletionArgsBuilder_Helpers(t *testing.T) {
	shells := DefaultShells()
	builder := NewCompletionArgsBuilder(shells)

	args := builder.ValidArgs()
	assert.Contains(t, args, "bash")
	assert.Contains(t, args, "zsh")
	assert.Contains(t, args, "fish")

	shellMap := builder.ShellByName()
	assert.Contains(t, shellMap, "bash")
	assert.Contains(t, shellMap, "zsh")
	assert.Contains(t, shellMap, "fish")
}

func TestOnStorageCreated_Hook(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	if cli.OnStorageCreated != nil {
		cli.OnStorageCreated(proj.FileStorage)
	}
}

func TestCacheOperationHandler_Operations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()
	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	t.Setenv("ZQK_TEST_ROOT", proj.Root)

	f1 := filepath.Join(proj.Root, "gol1.yaml")
	require.NoError(t, fileutil.WriteStandardFile(f1, []byte("title: gol1")))
	f2 := filepath.Join(proj.Root, "gol2.yaml")
	require.NoError(t, fileutil.WriteStandardFile(f2, []byte("title: gol2")))

	handler := storage.GetCacheOperationHandler()
	require.NotNil(t, handler)

	cUpdate := &pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationUpdate,
		NewID:     "GOL-1",
		Kind:      objects.KindGoal,
		FilePath:  f1,
	}
	err := handler(cUpdate)
	require.NoError(t, err)

	cInvalidate := &pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationInvalidate,
		OldID:     "GOL-1",
	}
	err = handler(cInvalidate)
	require.NoError(t, err)

	cInvUp := &pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationInvalidateAndUpdate,
		OldID:     "GOL-1",
		NewID:     "GOL-2",
		Kind:      objects.KindGoal,
		FilePath:  f2,
	}
	err = handler(cInvUp)
	require.NoError(t, err)

	cDef := &pkgctx.CacheContext{Operation: "unknown_op"}
	err = handler(cDef)
	require.NoError(t, err)
}

func TestMcpInitTrace_Logging(t *testing.T) {
	mcpInitTrace(time.Now(), "test_label_noop")

	t.Setenv("ZQK_MCP_ACCOUNT_ID", "acc-test-mcp")
	assert.Equal(t, "acc-test-mcp", zqkenv.MCPAccountID().Get())
	mcpInitTrace(time.Now(), "test_label_logged")
}

func TestPolicyDocLinksGate_Comprehensive(t *testing.T) {
	gate := &policy.DocLinksGate{}
	assert.Equal(t, "doc-links", gate.Name())
	assert.NotEmpty(t, gate.Description())

	tmpRoot := t.TempDir()
	opts := policy.RunOptions{ProjectRoot: tmpRoot}
	res, err := gate.Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, res.Passed)
	assert.NotEmpty(t, res.Violations)

	docsDir := filepath.Join(tmpRoot, "docs")
	require.NoError(t, fileutil.EnsureDir(docsDir))
	indexPath := filepath.Join(docsDir, "INDEX.md")
	indexContent := "# Index\n[Tutorial](tutorials/sample.md)\n[Ext](https://example.com)\n[Anchor](#sec)"
	require.NoError(t, fileutil.WriteStandardFile(indexPath, []byte(indexContent)))

	quadrants := []string{"tutorials", "howto", "manual", "explanation"}
	for _, q := range quadrants {
		qDir := filepath.Join(docsDir, q)
		require.NoError(t, fileutil.EnsureDir(qDir))
		require.NoError(t, fileutil.WriteStandardFile(filepath.Join(qDir, "sample.md"), []byte("# Sample")))
	}

	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(tmpRoot, "CONTRIBUTING.md"), []byte("# Contrib")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(tmpRoot, "CODE_OF_CONDUCT.md"), []byte("# CoC")))
	ghDir := filepath.Join(tmpRoot, ".github")
	require.NoError(t, fileutil.EnsureDir(ghDir))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(ghDir, "PULL_REQUEST_TEMPLATE.md"), []byte("# PR")))

	res2, err := gate.Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, res2.Passed)
	assert.Empty(t, res2.Violations)
}

func TestErrorClassifiers_Extended(t *testing.T) {
	assert.False(t, isInformationalCommandError(nil))
	assert.True(t, isInformationalCommandError(errors.New("semantic routing failed to match")))
	assert.True(t, isInformationalCommandError(errors.New("could not semantically resolve target")))
	assert.True(t, isInformationalCommandError(errors.New("unknown command status")))
	assert.True(t, isInformationalCommandError(errors.New("unknown flag --foo")))
	assert.True(t, isInformationalCommandError(errors.New("flag provided but not defined: -x")))
	assert.False(t, isInformationalCommandError(errors.New("corrupt disk block")))
}

func TestReadSessionAccountID_FallbackAndInvalid(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(cli.WithStorageProvider(context.Background(), proj.FileStorage))
	cli.RegisterStorageForProjectRoot(proj.Root, proj.FileStorage)

	validSID := StartZqkSession(context.Background(), proj.Root, "Fallback", "acc-fb-001", proj.FileStorage)
	require.NotEmpty(t, validSID)
	WritePersistedSessionID(context.Background(), proj.Root, validSID, "acc-fb-001", proj.FileStorage)

	accID, err := readSessionAccountID(context.Background(), cmd, proj.Root, "ZS-MISSING")
	require.NoError(t, err)
	assert.Equal(t, "acc-fb-001", accID)

	emptyAccObj := map[string]any{
		objects.FieldKeyID:     "ZS-NOACC",
		objects.FieldKeyKind:   objects.KindZqkSession,
		objects.FieldKeyStatus: "active",
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	err = proj.FileStorage.Create(context.Background(), secCtx, emptyAccObj)
	require.NoError(t, err)

	ClearPersistedSession(proj.Root)
	_, err = readSessionAccountID(context.Background(), cmd, proj.Root, "ZS-NOACC")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no account_id")
}

func TestCheckPolicyInterruptGate_CriticalUnacked(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	t.Setenv("ZQK_ACCOUNT_ID", "acc-human-999")
	rec := policyinterrupt.InterruptRecord{
		DedupeKey:   "crit-dedupe-1",
		PolicyID:    "POL-TEST-001",
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		Message:     "Critical security boundary violated",
	}
	err := policyinterrupt.AppendInterrupt(proj.Root, rec)
	require.NoError(t, err)

	err = checkPolicyInterruptGateInRoot(proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "critical policy interrupt requires acknowledgement")

	rec2 := policyinterrupt.InterruptRecord{
		DedupeKey:   "crit-dedupe-2",
		PolicyID:    "POL-TEST-002",
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		Message:     "",
	}
	err = policyinterrupt.AppendInterrupt(proj.Root, rec2)
	require.NoError(t, err)

	err = checkPolicyInterruptGateInRoot(proj.Root)
	require.Error(t, err)
}

func TestValidateAndWriteAuditEvent_Branches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	cmd := &cobra.Command{Use: "status"}
	metric := &clitool.CommandMetric{Command: "status"}

	err := validateAndWriteAuditEvent(cmd, proj.Root, "op", "info", nil, metric, "human")
	require.NoError(t, err)

	cmd.Flags().String("project-root", proj.Root, "")
	require.NoError(t, cmd.Flags().Set("project-root", proj.Root))
	err = validateAndWriteAuditEvent(cmd, proj.Root, "op", "info", nil, metric, "human")
	require.NoError(t, err)

	cmd.SetContext(cli.WithStorageProvider(context.Background(), proj.FileStorage))
	err = validateAndWriteAuditEvent(cmd, proj.Root, "op", "info", nil, metric, "human")
	require.NoError(t, err)
}

func TestChangeNotificationHandler_ShockwavesAndWake(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()
	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	t.Setenv("ZQK_TEST_ROOT", proj.Root)

	handler := storage.GetChangeNotificationHandler()
	require.NotNil(t, handler)

	goalID := "GOL-1790000000000000000-11112222"
	obj := map[string]any{
		objects.FieldKeyID:       goalID,
		objects.FieldKeyKind:     objects.KindGoal,
		objects.FieldKeyTitle:    "Test Goal",
		objects.FieldKeyStatus:   "active",
		objects.FieldKeyDeadline: time.Now().UTC().Add(time.Hour).Format("2006-01-02T15:04:05Z"),
	}
	err := handler(context.Background(), storage.OpCreate, objects.KindGoal, goalID, obj)
	require.NoError(t, err)

	updateObj := map[string]any{
		objects.FieldKeyID:       goalID,
		objects.FieldKeyKind:     objects.KindGoal,
		objects.FieldKeyTitle:    "Test Goal Updated",
		objects.FieldKeyStatus:   "completed",
		objects.FieldKeyDeadline: time.Now().UTC().Add(time.Hour).Format("2006-01-02T15:04:05Z"),
	}
	err = handler(context.Background(), storage.OpUpdate, objects.KindGoal, goalID, updateObj)
	require.NoError(t, err)

	err = handler(context.Background(), storage.OpDelete, objects.KindGoal, goalID, nil)
	require.NoError(t, err)

	err = handler(context.Background(), "unknown_op", objects.KindGoal, goalID, nil)
	require.NoError(t, err)
}

func TestResolveSessionAccountID_CacheInvalidate(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()
	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	t.Setenv("ZQK_TEST_ROOT", proj.Root)

	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(cli.WithStorageProvider(context.Background(), proj.FileStorage))
	cli.RegisterStorageForProjectRoot(proj.Root, proj.FileStorage)

	sid1 := StartZqkSession(context.Background(), proj.Root, "S1", "acc-one", proj.FileStorage)
	require.NotEmpty(t, sid1)
	sid2 := StartZqkSession(context.Background(), proj.Root, "S2", "acc-two", proj.FileStorage)
	require.NotEmpty(t, sid2)

	accID1, err := resolveSessionAccountID(context.Background(), cmd, proj.Root, sid1)
	require.NoError(t, err)
	assert.Equal(t, "acc-one", accID1)

	accID2, err := resolveSessionAccountID(context.Background(), cmd, proj.Root, sid2)
	require.NoError(t, err)
	assert.Equal(t, "acc-two", accID2)

	_, err = resolveSessionAccountID(context.Background(), cmd, proj.Root, "ZS-NONEXISTENT")
	require.Error(t, err)
}

func TestDraftSweepAuth_Helpers(t *testing.T) {
	assert.True(t, containsAnyMarker("sweep-test", []string{"sweep"}))
	assert.False(t, containsAnyMarker("other-test", []string{"sweep", "cleanup"}))

	secWithPerm := &pkgctx.SecurityContext{
		Permissions: []string{"other:perm", PermDraftSweep},
	}
	assert.True(t, hasDraftSweepPermission(secWithPerm))

	secWithoutPerm := &pkgctx.SecurityContext{
		Permissions: []string{"read:object"},
	}
	assert.False(t, hasDraftSweepPermission(secWithoutPerm))

	secVocab := &pkgctx.SecurityContext{
		ActiveVocabularySchemes: []string{"custom-scheme", "vocabulary_schemes.sweep"},
	}
	assert.True(t, hasDraftSweepVocabularyMarker(secVocab))

	secPersona := &pkgctx.SecurityContext{
		PersonaID: "persona:tpm_sweeper",
	}
	assert.True(t, hasDraftSweepPersonaMarker(secPersona))
}

func TestIsExpectedObjectNotFound_GetMiss(t *testing.T) {
	assert.True(t, isExpectedObjectNotFound(storage.ErrObjectNotFound))
	assert.True(t, isExpectedObjectNotFound(errors.New("account ACC-system not found")))
	assert.True(t, isExpectedObjectNotFound(errors.New("missing token in headers")))
	assert.False(t, isExpectedObjectNotFound(errors.New("random internal error")))
	assert.False(t, isExpectedObjectNotFound(nil))
}

func TestStartParentDeathWatcherIfSet_ValidPID(t *testing.T) {
	t.Setenv("ZQK_PARENT_PID", strconv.Itoa(os.Getppid()))
	startParentDeathWatcherIfSet()
}

type mockWarningExitCoder struct {
	code int
}

func (m mockWarningExitCoder) Error() string { return "mock warning" }
func (m mockWarningExitCoder) ExitCode() int { return m.code }

func TestIsNonBlockingWarningError_ExitCode3(t *testing.T) {
	assert.True(t, isNonBlockingWarningError(mockWarningExitCoder{code: 3}))
	assert.False(t, isNonBlockingWarningError(mockWarningExitCoder{code: 1}))
	assert.False(t, isNonBlockingWarningError(nil))
	assert.False(t, isNonBlockingWarningError(errors.New("other error")))
}

func TestCheckCLIReminderInRoot_Branches(t *testing.T) {
	checkCLIReminderInRoot("")

	tmpDir := t.TempDir()
	checkCLIReminderInRoot(tmpDir)

	flagDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	err := fileutil.MkdirAll(flagDir, paths.DirPerm755)
	require.NoError(t, err)

	flagFile := filepath.Join(flagDir, "cli_reminder.flag")
	err = fileutil.WriteFile(flagFile, []byte("test reminder message"), paths.FilePerm644)
	require.NoError(t, err)

	checkCLIReminderInRoot(tmpDir)

	oldTime := time.Now().Add(-2 * time.Hour)
	err = os.Chtimes(flagFile, oldTime, oldTime)
	require.NoError(t, err)

	checkCLIReminderInRoot(tmpDir)
}

func TestCheckPolicyInterruptGateInRoot_Branches(t *testing.T) {
	err := checkPolicyInterruptGateInRoot("")
	require.NoError(t, err)

	tmpDir := t.TempDir()
	t.Setenv(zqkenv.AccountID().Name(), "")
	t.Setenv(zqkenv.MCPAccountID().Name(), "")
	err = checkPolicyInterruptGateInRoot(tmpDir)
	require.NoError(t, err)

	t.Setenv(zqkenv.AccountID().Name(), pkgctx.SystemAccountID)
	err = checkPolicyInterruptGateInRoot(tmpDir)
	require.NoError(t, err)

	t.Setenv(zqkenv.AccountID().Name(), "ACC-USER-1")
	err = checkPolicyInterruptGateInRoot(tmpDir)
	require.NoError(t, err)
}

func TestApplyDefaultCLIGoroutineBudget_ThreadExceeds(t *testing.T) {
	prev := goroutinelabels.DefaultBudget()
	t.Cleanup(func() { goroutinelabels.SetDefaultBudget(prev) })
	goroutinelabels.SetDefaultBudget(nil)
	t.Setenv(zqkenv.MaxOSThreads().Name(), "1000")
	applyDefaultCLIGoroutineBudget()
	got := goroutinelabels.DefaultBudget()
	require.NotNil(t, got)
	assert.Equal(t, 1000, got.MaxTotal())
}

func TestAuditCoordination_EmitCommandExecutionEvent(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	ctx := context.Background()
	mSuccess := &clitool.CommandMetric{
		Command:       "create",
		NormalizedCmd: "create",
		Success:       true,
		Duration:      50 * time.Millisecond,
		StartTime:     time.Now(),
		EndTime:       time.Now().Add(50 * time.Millisecond),
	}
	emitCommandExecutionEventViaCoordinator(ctx, proj.Root, proj.FileStorage, "op-success", auditSeverityLow, map[string]any{"k": "v"}, mSuccess, profileMCP)

	mFail := &clitool.CommandMetric{
		Command:       "delete",
		NormalizedCmd: "delete",
		Success:       false,
		Error:         "disk write error",
		Duration:      10 * time.Millisecond,
		StartTime:     time.Now(),
		EndTime:       time.Now().Add(10 * time.Millisecond),
	}
	emitCommandExecutionEventViaCoordinator(ctx, proj.Root, proj.FileStorage, "op-fail", auditSeverityHigh, nil, mFail, profileDebug)

	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(cli.WithStorageProvider(ctx, proj.FileStorage))
	cli.RegisterStorageForProjectRoot(proj.Root, proj.FileStorage)

	createCommandAuditEvent(cmd, proj.Root, mSuccess, profileHuman)
	createCommandAuditEvent(cmd, "", mSuccess, profileHuman)
	createCommandAuditEvent(nil, proj.Root, mSuccess, profileHuman)
}

func TestAuth_RunLoginAndLogout_Direct(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	cmd := NewLoginCmd()
	cmd.SetContext(cli.WithStorageProvider(context.Background(), proj.FileStorage))
	cli.RegisterStorageForProjectRoot(proj.Root, proj.FileStorage)

	err := runLogin(cmd, nil)
	require.NoError(t, err)

	err = runLogin(cmd, nil)
	require.NoError(t, err)

	logoutCmd := NewLogoutCmd()
	logoutCmd.SetContext(cli.WithStorageProvider(context.Background(), proj.FileStorage))
	err = runLogout(logoutCmd, nil)
	require.NoError(t, err)

	err = runLogout(logoutCmd, nil)
	require.NoError(t, err)
}

func TestValidationTriggerBatch_AdditionalEdgeCases(t *testing.T) {
	AddValidationTrigger("", "task", "ATK-1", "create")
	AddValidationTrigger("/tmp/dummy", "session", "S-1", "create")

	flushValidationBatch("")

	var triggered bool
	restore := SetValidationTriggerFuncForTest(func(ctx context.Context, evtType, evtKind string, data map[string]any) error {
		triggered = true
		return nil
	})
	defer restore()

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		ForceRemoveRootOnCleanup: true,
	})
	defer func() {
		if proj.FileStorage != nil {
			err := proj.FileStorage.Shutdown(context.Background())
			require.NoError(t, err)
		}
	}()

	AddValidationTrigger(proj.Root, "task", "ATK-2", "create")
	flushValidationBatch(proj.Root)
	assert.True(t, triggered)

	flushValidationBatch(proj.Root)
}
