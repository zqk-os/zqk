package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
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
