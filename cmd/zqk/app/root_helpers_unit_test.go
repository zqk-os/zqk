package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFormatReminderDuration(t *testing.T) {
	assert.Equal(t, "30s", formatReminderDuration(30*time.Second))
	assert.Equal(t, "15m", formatReminderDuration(15*time.Minute))
	assert.Equal(t, "2.0h", formatReminderDuration(2*time.Hour))
}

func TestArgsContainHelpOrVersion(t *testing.T) {
	assert.True(t, argsContainHelpOrVersion([]string{"help"}))
	assert.True(t, argsContainHelpOrVersion([]string{"--help"}))
	assert.True(t, argsContainHelpOrVersion([]string{"-h"}))
	assert.True(t, argsContainHelpOrVersion([]string{"version"}))
	assert.True(t, argsContainHelpOrVersion([]string{"--version"}))
	assert.True(t, argsContainHelpOrVersion([]string{"-v"}))
	assert.False(t, argsContainHelpOrVersion([]string{"status"}))
	assert.False(t, argsContainHelpOrVersion([]string{}))
}

func TestRootErrorClassifiers(t *testing.T) {
	// isNonBlockingWarningError
	assert.False(t, isNonBlockingWarningError(nil))
	assert.False(t, isNonBlockingWarningError(errors.New("regular error")))

	// isExpectedObjectNotFound
	assert.False(t, isExpectedObjectNotFound(nil))
	assert.True(t, isExpectedObjectNotFound(errors.New("account acc-system not found in storage")))
	assert.True(t, isExpectedObjectNotFound(errors.New("missing token in headers")))
	assert.False(t, isExpectedObjectNotFound(errors.New("generic fatal error")))
}

func TestCheckCLIReminderInRoot_Empty(t *testing.T) {
	checkCLIReminderInRoot("")
}

func TestCheckPolicyInterruptGateInRoot_Empty(t *testing.T) {
	assert.NoError(t, checkPolicyInterruptGateInRoot(""))
}

func TestFirstString(t *testing.T) {
	assert.Equal(t, "hello", firstString([]string{"", "  ", "hello", "world"}))
	assert.Equal(t, "", firstString([]string{"", "  "}))
	assert.Equal(t, "", firstString(nil))
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "apple", firstNonEmpty("", "  ", "apple", "banana"))
	assert.Equal(t, "", firstNonEmpty("", "  "))
	assert.Equal(t, "", firstNonEmpty())
}

func TestDefaultShells(t *testing.T) {
	shells := DefaultShells()
	assert.NotEmpty(t, shells)
	names := make(map[string]bool)
	for _, s := range shells {
		names[s.Name] = true
	}
	assert.True(t, names["bash"])
	assert.True(t, names["zsh"])
}

func TestResolveAndReadSessionAccountID_Branches(t *testing.T) {
	ctx := context.Background()
	cmd := &cobra.Command{}

	// 1. Storage not available
	_, err := readSessionAccountID(ctx, cmd, "/nonexistent/root", "ZS-1")
	assert.Error(t, err)

	// 2. Project with storage
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root
	sp := proj.FileStorage
	defer func() {
		if sp != nil {
			_ = sp.Shutdown(context.Background())
		}
	}()

	// Session not found
	_, err = readSessionAccountID(ctx, cmd, tmpDir, "ZS-NOTFOUND")
	assert.Error(t, err)

	// Create valid session using StartZqkSession
	sessionID := StartZqkSession(ctx, tmpDir, "Test Session", "acc-user-123", sp)
	require.NotEmpty(t, sessionID)

	accID, err := readSessionAccountID(ctx, cmd, tmpDir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "acc-user-123", accID)

	// ResolveSessionAccountID with explicit persona
	resolved, err := resolveSessionAccountID(ctx, cmd, tmpDir, sessionID+"|PER-TEST")
	require.NoError(t, err)
	assert.Equal(t, "acc-user-123|PER-TEST", resolved)
}

func TestCheckSchedulerDaemonStatus_Branches(t *testing.T) {
	cmd := &cobra.Command{}

	// Empty project root
	assert.NoError(t, checkSchedulerDaemonStatus("", cmd, nil))

	// Non-admin without permissions
	root := t.TempDir()
	guestCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewGuestSecurityContext())
	cmd.SetContext(guestCtx)
	assert.NoError(t, checkSchedulerDaemonStatus(root, cmd, nil))
}

func TestStartParentDeathWatcherIfSet(t *testing.T) {
	// With empty or zero parent pid
	t.Setenv("ZQK_PARENT_PID", "")
	startParentDeathWatcherIfSet()

	t.Setenv("ZQK_PARENT_PID", "0")
	startParentDeathWatcherIfSet()

	t.Setenv("ZQK_PARENT_PID", "invalid")
	startParentDeathWatcherIfSet()
}

func TestEmitReferenceShockwaves_AllKinds(t *testing.T) {
	root := t.TempDir()

	// 1. TestCase
	tcData := map[string]any{
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
		objects.FieldKeyRequirementRefs: []any{"REQ-1"},
		objects.FieldKeyBacklogItemRefs: []any{"BLI-1"},
	}
	emitReferenceShockwaves(root, objects.KindTestCase, "TST-1", tcData)

	// 2. Criteria
	critData := map[string]any{
		objects.FieldKeyTestCaseRefs: []any{"TST-1"},
	}
	emitReferenceShockwaves(root, objects.KindCriteria, "CRIT-1", critData)

	// 3. BacklogItem
	bliData := map[string]any{
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
		objects.FieldKeyTestCaseRefs: []any{"TST-1"},
	}
	emitReferenceShockwaves(root, objects.KindBacklogItem, "BLI-1", bliData)

	// 4. Other kind (no-op)
	emitReferenceShockwaves(root, "goal", "GOL-1", nil)
}

func TestCheckCLIReminderInRoot_WithFile(t *testing.T) {
	root := t.TempDir()
	flagDir := filepath.Join(root, paths.ProjectDataDir)
	require.NoError(t, fileutil.EnsureDir(flagDir))
	flagFile := filepath.Join(flagDir, "cli_reminder.flag")
	require.NoError(t, fileutil.WriteStandardFile(flagFile, []byte("Important Reminder Message")))

	// Calling checkCLIReminderInRoot should read the file and log
	checkCLIReminderInRoot(root)
}

func TestCheckPolicyInterruptGateInRoot_ActorBranches(t *testing.T) {
	root := t.TempDir()

	// 1. System account is bypassed
	t.Setenv("ZQK_ACCOUNT_ID", pkgctx.SystemAccountID)
	assert.NoError(t, checkPolicyInterruptGateInRoot(root))

	// 2. Non-system actor with no pending interrupts
	t.Setenv("ZQK_ACCOUNT_ID", "acc-user-999")
	assert.NoError(t, checkPolicyInterruptGateInRoot(root))
}

func TestIsMutatingCommand(t *testing.T) {
	assert.True(t, isMutatingCommand(&cobra.Command{Use: "create"}))
	assert.True(t, isMutatingCommand(&cobra.Command{Use: "update"}))
	assert.True(t, isMutatingCommand(&cobra.Command{Use: "delete"}))
	assert.True(t, isMutatingCommand(&cobra.Command{Use: "workflow"}))
	assert.True(t, isMutatingCommand(&cobra.Command{Use: "system"}))

	parent := &cobra.Command{Use: "workflow"}
	child := &cobra.Command{Use: "next"}
	parent.AddCommand(child)
	assert.True(t, isMutatingCommand(child))

	readCmd := &cobra.Command{Use: "get"}
	assert.False(t, isMutatingCommand(readCmd))
	assert.False(t, isMutatingCommand(nil))
}

func TestIsInformationalCommandError_ExtraBranches(t *testing.T) {
	assert.False(t, isInformationalCommandError(nil))
	assert.True(t, isInformationalCommandError(errors.New("semantic routing failed for plan")))
	assert.True(t, isInformationalCommandError(errors.New("could not semantically resolve target")))
	assert.True(t, isInformationalCommandError(errors.New("unknown command 'foo'")))
	assert.True(t, isInformationalCommandError(errors.New("unknown flag: --bar")))
	assert.True(t, isInformationalCommandError(errors.New("flag provided but not defined: -x")))
	assert.False(t, isInformationalCommandError(errors.New("internal server crash")))
}

func TestCheckSchedulerDaemonStatus_WithAdminAndMutating(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", "") // allow check to proceed

	cmd := &cobra.Command{Use: "create"}
	adminSec := &pkgctx.SecurityContext{
		AccountID: "acc-admin",
		Roles:     []string{"admin"},
	}
	ctx := pkgctx.WithSecurityContext(context.Background(), adminSec)
	cmd.SetContext(ctx)

	err := checkSchedulerDaemonStatus(root, cmd, nil)
	assert.NoError(t, err)
}

func TestRoot_StartParentDeathWatcherIfSet(t *testing.T) {
	// Empty PID
	t.Setenv("ZQK_PARENT_PID", "")
	startParentDeathWatcherIfSet()

	// Invalid PID
	t.Setenv("ZQK_PARENT_PID", "invalid-pid")
	startParentDeathWatcherIfSet()

	// Negative PID
	t.Setenv("ZQK_PARENT_PID", "-1")
	startParentDeathWatcherIfSet()
}

func TestRoot_MCPInitTrace(t *testing.T) {
	// Without MCP account ID
	t.Setenv("ZQK_MCP_ACCOUNT_ID", "")
	mcpInitTrace(time.Now(), "test-label")

	// With MCP account ID
	t.Setenv("ZQK_MCP_ACCOUNT_ID", "acc-test-trace")
	mcpInitTrace(time.Now().Add(-100*time.Millisecond), "test-label")
}

func TestRoot_PreRunInitFileLogging(t *testing.T) {
	// Empty project root
	rootPreRunInitFileLogging(nil, "", false)

	// Is help command
	rootPreRunInitFileLogging(nil, "/some/root", true)

	// Valid root, non-help command with context flag
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("context", "human", "")
	rootPreRunInitFileLogging(cmd, t.TempDir(), false)
}

func TestRootPersistentPostRunE_Branches(t *testing.T) {
	// 1. Help/version command
	helpCmd := &cobra.Command{Use: "help"}
	assert.NoError(t, rootPersistentPostRunE(helpCmd, nil))

	versionCmd := &cobra.Command{Use: "version"}
	assert.NoError(t, rootPersistentPostRunE(versionCmd, nil))

	// 2. Nil context
	regularCmd := &cobra.Command{Use: "status"}
	assert.NoError(t, rootPersistentPostRunE(regularCmd, nil))

	// 3. With tracker in context
	ctx := context.Background()
	tracker := clitool.NewCommandExecutionTracker()
	ctx = clitool.WithTracker(ctx, tracker)
	regularCmd.SetContext(ctx)
	assert.NoError(t, rootPersistentPostRunE(regularCmd, nil))

	// 4. With cpuProfileFile mock cleanup
	tmpFile, err := os.CreateTemp("", "cpu-prof-*.pprof")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	cpuProfileFile = tmpFile
	assert.NoError(t, rootPersistentPostRunE(helpCmd, nil))
	assert.Nil(t, cpuProfileFile)

	// 5. With goroutineProfilePath
	goroutineProfilePath = filepath.Join(t.TempDir(), "goroutine.pprof")
	assert.NoError(t, rootPersistentPostRunE(helpCmd, nil))
	assert.Empty(t, goroutineProfilePath)
}

func TestCheckPolicyInterruptGateInRoot_WithUnacked(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_ACCOUNT_ID", "acc-developer-99")
	t.Setenv("ZQK_MCP_ACCOUNT_ID", "")

	err := policyinterrupt.AppendInterrupt(root, policyinterrupt.InterruptRecord{
		DedupeKey:   "PI-CRIT-001",
		PolicyID:    "POL-BREAKING-CHANGE-001",
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		Message:     "Breaking migration in progress",
	})
	require.NoError(t, err)

	err = checkPolicyInterruptGateInRoot(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires acknowledgement")
}

func TestCheckCLIReminderInRoot_WithRecentFlagFile(t *testing.T) {
	root := t.TempDir()
	flagDir := filepath.Join(root, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(flagDir, 0755))
	flagFile := filepath.Join(flagDir, "cli_reminder.flag")
	require.NoError(t, fileutil.WriteFile(flagFile, []byte("REMINDER CONTENT"), 0644))

	checkCLIReminderInRoot(root)
}

func TestCheckSchedulerDaemonStatus_RoleAndPermVariations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", "")

	// 1. With manage:scheduler perm
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().Bool("allow-degraded", false, "")
	sec := &pkgctx.SecurityContext{
		Permissions: []string{"manage:scheduler"},
	}
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), sec))
	assert.NoError(t, checkSchedulerDaemonStatus(root, cmd, nil))

	// 2. With read:* perm and allow-degraded=true
	cmd2 := &cobra.Command{Use: "create"}
	cmd2.Flags().Bool("allow-degraded", true, "")
	_ = cmd2.Flags().Set("allow-degraded", "true")
	sec2 := &pkgctx.SecurityContext{
		Permissions: []string{"read:*"},
	}
	cmd2.SetContext(pkgctx.WithSecurityContext(context.Background(), sec2))
	assert.NoError(t, checkSchedulerDaemonStatus(root, cmd2, nil))

	// 3. Without permissions
	cmd3 := &cobra.Command{Use: "create"}
	sec3 := &pkgctx.SecurityContext{
		Roles: []string{"guest"},
	}
	cmd3.SetContext(pkgctx.WithSecurityContext(context.Background(), sec3))
	assert.NoError(t, checkSchedulerDaemonStatus(root, cmd3, nil))
}

func TestRootPersistentPostRunE_WithStorageAndSession(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	defer func() {
		if proj.FileStorage != nil {
			_ = proj.FileStorage.Shutdown(context.Background())
		}
	}()

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("project-root", proj.Root, "")
	_ = cmd.Flags().Set("project-root", proj.Root)

	ctx := context.Background()
	ctx = WithZqkSessionID(ctx, "ZS-TEST-001")
	ctx = clitool.WithTracker(ctx, clitool.NewCommandExecutionTracker())
	ctx = cli.WithStorageProvider(ctx, proj.FileStorage)
	cmd.SetContext(ctx)

	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	assert.NoError(t, rootPersistentPostRunE(cmd, nil))
}
