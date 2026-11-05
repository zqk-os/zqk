package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestSyncHelpers_UnitBranchesAndCommitMessages(t *testing.T) {
	t.Parallel()

	// 1. generateBranchName
	assert.Equal(t, "feature/pri-123", generateBranchName("PRI-123"))
	assert.Equal(t, "feature/pri-plan-abc", generateBranchName("PRI-PLAN-ABC"))
	assert.Equal(t, "custom-branch", generateBranchName("custom-branch"))

	// 2. extractBacklogItems
	files := []string{
		"pkg/storage/store.go",
		"docs/BLI-100-design.md",
		"cmd/zqk/BLI-200.go",
		"internal/BLI-100_test.go",
	}
	items := extractBacklogItems(files)
	assert.Len(t, items, 2)
	assert.True(t, items["BLI-100"])
	assert.True(t, items["BLI-200"])
	assert.Empty(t, extractBacklogItems(nil))

	// 3. buildPriorityPlanMessage
	assert.Equal(t, "", buildPriorityPlanMessage(nil))
	assert.Equal(t, "", buildPriorityPlanMessage(map[string]any{"other": 123}))
	assert.Equal(t, "PRI-100", buildPriorityPlanMessage(map[string]any{
		objects.FieldKeyID: "PRI-100",
	}))
	assert.Equal(t, "PRI-100: Core Subsystem", buildPriorityPlanMessage(map[string]any{
		objects.FieldKeyID:    "PRI-100",
		objects.FieldKeyTitle: "Core Subsystem",
	}))

	// 4. buildBacklogItemsMessage
	assert.Equal(t, "", buildBacklogItemsMessage(nil))
	itemMsg := buildBacklogItemsMessage(map[string]bool{"BLI-2": true, "BLI-1": true})
	assert.Contains(t, itemMsg, "Related backlog items: BLI-1, BLI-2")

	// 5. buildCommitMessageParts
	plan := map[string]any{
		objects.FieldKeyID:    "PRI-001",
		objects.FieldKeyTitle: "Test Plan",
	}
	parts := buildCommitMessageParts(plan, map[string]bool{"BLI-1": true}, 3)
	assert.Contains(t, parts, "PRI-001: Test Plan")
	assert.Contains(t, parts, "Related backlog items: BLI-1")
	assert.Contains(t, parts, "Changed 3 file(s)")

	partsEmpty := buildCommitMessageParts(nil, nil, 0)
	assert.Contains(t, partsEmpty, "0 file(s) changed")
}

func TestSyncHelpers_ParseTableOutput(t *testing.T) {
	t.Parallel()

	logger := logging.NewEventLogger(context.Background())

	// Clean output
	cleanOutput := "System check passed.\nTier 1 (Blocking): 0\nTier 2: 2\n"
	err := parseTableOutput(cleanOutput, logger)
	assert.NoError(t, err)

	// Blocking violations output
	dirtyOutput := "System check failed.\nTier 1 (Blocking): 4\nTier 2: 0\n"
	err = parseTableOutput(dirtyOutput, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tier 1 (blocking) violations detected: 4")
}

func TestSyncHelpers_HandlePriorityPlanError(t *testing.T) {
	t.Parallel()

	logger := logging.NewEventLogger(context.Background())

	// When err does not contain "no active or in_progress priority plans found", returns nil, nil
	genericErr := errors.New("database unreachable")
	plan, err := handlePriorityPlanError(genericErr, false, "my commit", logger)
	assert.NoError(t, err)
	assert.Nil(t, plan)

	// When missing active plan and autoCommit is true without message, returns validation error
	missingPlanErr := errors.New("no active or in_progress priority plans found")
	_, err = handlePriorityPlanError(missingPlanErr, true, "", logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot auto-generate commit message without active priority plan")

	// When missing active plan but message is provided, returns nil, nil
	plan, err = handlePriorityPlanError(missingPlanErr, true, "custom message", logger)
	assert.NoError(t, err)
	assert.Nil(t, plan)
}

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmpDir := t.TempDir()
	initCmd := testkit.ManagedCommand(t, t.Context(), "git", "init")
	zqkenv.WireExecForIsolatedProject(initCmd, tmpDir)
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	testFile := filepath.Join(tmpDir, "README.md")
	if err := fileutil.WriteFile(testFile, []byte("# Test Repo"), paths.FilePerm644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	addCmd := testkit.ManagedCommand(t, t.Context(), "git", "add", "README.md")
	zqkenv.WireExecForIsolatedProject(addCmd, tmpDir)
	_ = addCmd.Run()

	commitCmd := testkit.ManagedCommand(t, t.Context(), "git", "commit", "-m", "Initial commit")
	zqkenv.WireExecForIsolatedProject(commitCmd, tmpDir)
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	_ = commitCmd.Run()

	return tmpDir
}

func TestSyncHelpers_DryRunOperations(t *testing.T) {
	tmpDir := setupTestGitRepo(t)
	logger := logging.NewEventLogger(context.Background())
	cmd := NewSyncCmd()

	// ensureBranch dry run
	err := ensureBranch(tmpDir, "feature/pri-1", logger, true)
	assert.NoError(t, err)

	// gitStage dry run
	err = gitStage(cmd, tmpDir, logger, true)
	assert.NoError(t, err)

	// gitCommit dry run
	err = gitCommit(cmd, tmpDir, "test commit", logger, true)
	assert.NoError(t, err)

	// gitPull dry run
	err = gitPull(tmpDir, false, logger, true)
	assert.NoError(t, err)

	// gitPush dry run
	err = gitPush(cmd, tmpDir, false, logger, true)
	assert.NoError(t, err)

	// runSystemHealthCheck dry run
	err = runSystemHealthCheck(tmpDir, logger, true)
	assert.NoError(t, err)

	// createPullRequest dry run
	err = createPullRequest(tmpDir, nil, context.Background(), map[string]any{"id": "PRI-1"}, "feature/pri-1", "PR Title", "PR Body", logger, true)
	assert.NoError(t, err)

	// handleHealthCheck
	assert.NoError(t, handleHealthCheck(false, false, false, false, tmpDir, logger, true))
	assert.NoError(t, handleHealthCheck(true, true, false, false, tmpDir, logger, true))

	// handleStaging
	assert.NoError(t, handleStaging(false, cmd, tmpDir, logger, true))
	assert.NoError(t, handleStaging(true, cmd, tmpDir, logger, true))

	// handlePulling
	assert.NoError(t, handlePulling(false, tmpDir, false, logger, true))
	assert.NoError(t, handlePulling(true, tmpDir, false, logger, true))

	// handlePushing
	assert.NoError(t, handlePushing(false, false, cmd, tmpDir, false, nil, nil, nil, "", "", logger, true))
	assert.NoError(t, handlePushing(true, false, cmd, tmpDir, false, nil, nil, nil, "", "", logger, true))

	// createPRForPlatform error on unsupported platform
	err = createPRForPlatform("unsupported_vcs", tmpDir, "feature/x", "Title", "Body", logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported platform")
}

func TestSyncHelpers_ValidateGitRepository_Negative(t *testing.T) {
	t.Parallel()

	logger := logging.NewEventLogger(context.Background())
	tmpDir := t.TempDir() // Not a git repo

	err := validateGitRepository(tmpDir, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}
