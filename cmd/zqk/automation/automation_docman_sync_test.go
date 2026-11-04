package automation

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// chdirToTemp switches the working directory to a new temporary directory and registers a restore cleanup.
func chdirToTemp(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	wd, errWd := fileutil.Getwd()
	require.NoError(t, errWd)
	t.Cleanup(func() {
		chdirErr := fileutil.Chdir(wd)
		require.NoError(t, chdirErr)
	})
	chdirErr := fileutil.Chdir(tempDir)
	require.NoError(t, chdirErr)
	return tempDir
}

// corruptProjectDotZqk creates a regular file named .zqk to force storage directory initialization to fail.
func corruptProjectDotZqk(t *testing.T, dir string) {
	t.Helper()
	badFile := filepath.Join(dir, ".zqk")
	errWrite := fileutil.WriteFile(badFile, []byte("plain file blocking directory creation"), paths.FilePerm644)
	require.NoError(t, errWrite)
}

func TestDocmanSync_CheckOnlyModeUnregisteredAndRegistered(t *testing.T) {
	cmd := NewDocmanSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	// Mode 1: files need registration, normal output
	errNeeded := handleCheckOnlyMode(cmd, 3)
	require.Error(t, errNeeded)
	assert.Contains(t, errNeeded.Error(), "3 files need registration")
	assert.Contains(t, buf.String(), "3 files need registration")

	// Mode 2: all registered, normal output
	buf.Reset()
	errClean := handleCheckOnlyMode(cmd, 0)
	require.NoError(t, errClean)
	assert.Contains(t, buf.String(), "All documentation is registered")

	// Mode 3: quiet mode with files needed
	quietCmd := NewDocmanSyncCmd()
	quietBuf := new(bytes.Buffer)
	quietCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), quietBuf))
	setErr := quietCmd.Flags().Set("quiet", "true")
	require.NoError(t, setErr)
	errQuietNeeded := handleCheckOnlyMode(quietCmd, 2)
	require.Error(t, errQuietNeeded)
	assert.Empty(t, quietBuf.String())

	// Mode 4: quiet mode when clean
	errQuietClean := handleCheckOnlyMode(quietCmd, 0)
	require.NoError(t, errQuietClean)
	assert.Empty(t, quietBuf.String())
}

func TestDocmanSync_GitAwareNoGitRepo(t *testing.T) {
	tempDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(automationProfileAIAgent)

	// hasStagedMarkdownFiles returns error when directory is not a git repository
	hasMd, err := hasStagedMarkdownFiles(tempDir)
	require.Error(t, err)
	assert.False(t, hasMd)
	assert.Contains(t, err.Error(), "not a git repository")

	// checkGitAwareSync should catch error, log debug, and return true to avoid blocking
	cmd := NewDocmanSyncCmd()
	shouldProceed := checkGitAwareSync(cmd, tempDir, true, logger)
	assert.True(t, shouldProceed)

	// checkGitAwareSync with gitAware=false returns true immediately
	shouldProceedWithoutGit := checkGitAwareSync(cmd, tempDir, false, logger)
	assert.True(t, shouldProceedWithoutGit)
}

func TestDocmanSync_HasStagedMarkdownFilesCorruptedIndex(t *testing.T) {
	tempDir := t.TempDir()
	initCmd := execwrap.Command("git", "init")
	initCmd.Dir = tempDir
	errInit := initCmd.Run()
	require.NoError(t, errInit)

	// Corrupt git index by writing invalid binary bytes into .git/index
	indexPath := filepath.Join(tempDir, ".git", "index")
	errWrite := fileutil.WriteFile(indexPath, []byte("NOT_A_VALID_GIT_INDEX_DATA"), paths.FilePerm644)
	require.NoError(t, errWrite)

	hasMd, err := hasStagedMarkdownFiles(tempDir)
	require.Error(t, err)
	assert.False(t, hasMd)
	assert.Contains(t, err.Error(), "failed to get staged files")
}

func TestDocmanSync_OutputRegistrationResults(t *testing.T) {
	// Quiet mode produces no output
	quietCmd := NewDocmanSyncCmd()
	quietBuf := new(bytes.Buffer)
	quietCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), quietBuf))
	errSet := quietCmd.Flags().Set("quiet", "true")
	require.NoError(t, errSet)
	outputRegistrationResults(quietCmd, 2, 1, false)
	outputRegistrationResults(quietCmd, 2, 1, true)
	assert.Empty(t, quietBuf.String())

	// Non-quiet dry-run mode
	normalCmd := NewDocmanSyncCmd()
	normalBuf := new(bytes.Buffer)
	normalCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), normalBuf))
	outputRegistrationResults(normalCmd, 4, 2, true)
	assert.Contains(t, normalBuf.String(), "Would create 4 doc_entry objects")
	assert.Contains(t, normalBuf.String(), "Would skip 2 files")

	// Non-quiet standard mode
	normalBuf.Reset()
	outputRegistrationResults(normalCmd, 3, 1, false)
	assert.Contains(t, normalBuf.String(), "Created 3 doc_entry objects")
	assert.Contains(t, normalBuf.String(), "Skipped 1 files")
}

func TestDocmanSync_GetCommandContext(t *testing.T) {
	// Nil context branch creates system background context
	cmdNil := &cobra.Command{}
	ctxNil := getCommandContext(cmdNil)
	require.NotNil(t, ctxNil)

	// Non-nil context branch preserves provided context
	type contextKey string
	const testKey contextKey = "automation-test-key"
	expectedCtx := context.WithValue(context.Background(), testKey, "custom-value")
	cmdWithCtx := &cobra.Command{}
	cmdWithCtx.SetContext(expectedCtx)
	actualCtx := getCommandContext(cmdWithCtx)
	assert.Equal(t, expectedCtx, actualCtx)
	assert.Equal(t, "custom-value", actualCtx.Value(testKey))
}

func TestDocmanSync_ParseFlagsDetailed(t *testing.T) {
	cmd := NewDocmanSyncCmd()
	errArgs := cmd.ParseFlags([]string{
		"--context", "custom-profile-v1",
		"--check-only",
		"--git-aware",
		"--update-existing",
		"--dry-run",
	})
	require.NoError(t, errArgs)

	flags, err := parseDocmanSyncFlags(cmd)
	require.NoError(t, err)
	require.NotNil(t, flags)
	assert.Equal(t, "custom-profile-v1", flags.Profile)
	assert.True(t, flags.CheckOnly)
	assert.True(t, flags.GitAware)
	assert.True(t, flags.UpdateExisting)
	assert.True(t, flags.DryRun)
}

func TestDocmanSync_ParseFlagsErrorBranches(t *testing.T) {
	// Flag type mismatch: check-only
	cmd1 := &cobra.Command{}
	cmd1.Flags().Int("check-only", 1, "")
	_, err1 := parseDocmanSyncFlags(cmd1)
	require.Error(t, err1)
	assert.Contains(t, err1.Error(), "check-only")

	// Flag type mismatch: git-aware
	cmd2 := &cobra.Command{}
	cmd2.Flags().Bool("check-only", false, "")
	cmd2.Flags().Int("git-aware", 1, "")
	_, err2 := parseDocmanSyncFlags(cmd2)
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "git-aware")

	// Flag type mismatch: update-existing
	cmd3 := &cobra.Command{}
	cmd3.Flags().Bool("check-only", false, "")
	cmd3.Flags().Bool("git-aware", false, "")
	cmd3.Flags().Int("update-existing", 1, "")
	_, err3 := parseDocmanSyncFlags(cmd3)
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "update-existing")

	// Flag type mismatch: dry-run
	cmd4 := &cobra.Command{}
	cmd4.Flags().Bool("check-only", false, "")
	cmd4.Flags().Bool("git-aware", false, "")
	cmd4.Flags().Bool("update-existing", false, "")
	cmd4.Flags().Int("dry-run", 1, "")
	_, err4 := parseDocmanSyncFlags(cmd4)
	require.Error(t, err4)
	assert.Contains(t, err4.Error(), "dry-run")

	// runDocmanSync returns parseDocmanSyncFlags error
	errRun := runDocmanSync(cmd1, []string{})
	require.Error(t, errRun)
	assert.Contains(t, errRun.Error(), "check-only")
}

func TestDocmanSync_UpdateExistingBranches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.docman_update_existing",
	})
	registry, err := initializeDocmanRegistry(proj.Root)
	require.NoError(t, err)
	require.NotNil(t, registry)

	cmd := NewDocmanSyncCmd()
	cmdCtx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(automationProfileAIAgent)

	// Branch 1: updateExisting=false returns early with (false, nil)
	returnedNoOp, errNoOp := updateExistingDocEntries(
		cmd, registry, cmdCtx, automationProfileAIAgent,
		false, false, false, logger,
	)
	require.NoError(t, errNoOp)
	assert.False(t, returnedNoOp)

	// Branch 2: updateExisting=true with dryRun=true hits line 141 (false, nil)
	returnedDry, errDry := updateExistingDocEntries(
		cmd, registry, cmdCtx, automationProfileAIAgent,
		true, true, false, logger,
	)
	require.NoError(t, errDry)
	assert.False(t, returnedDry)

	// Branch 3: updateExisting=true with checkOnly=true hits line 141 (false, nil)
	returnedCheck, errCheck := updateExistingDocEntries(
		cmd, registry, cmdCtx, automationProfileAIAgent,
		true, false, true, logger,
	)
	require.NoError(t, errCheck)
	assert.False(t, returnedCheck)
}

func TestDocmanSync_RootNotFound(t *testing.T) {
	tempDir := chdirToTemp(t)
	require.NotEmpty(t, tempDir)

	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")

	oldCLICommand := paths.CLICommandName
	paths.CLICommandName = ""
	t.Cleanup(func() {
		paths.CLICommandName = oldCLICommand
	})

	cmd := NewDocmanSyncCmd()
	err := runDocmanSync(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
	assert.Contains(t, err.Error(), paths.CLICommandNameDefault)
}

func TestDocmanSync_InitializeRegistryFailure(t *testing.T) {
	tempDir := t.TempDir()
	corruptProjectDotZqk(t, tempDir)

	registry, err := initializeDocmanRegistry(tempDir)
	require.Error(t, err)
	assert.Nil(t, registry)
	assert.Contains(t, err.Error(), "failed to initialize storage factory")
}

func TestDocmanSync_RunDocmanSyncRegistryFailure(t *testing.T) {
	tempDir := t.TempDir()
	corruptProjectDotZqk(t, tempDir)

	t.Setenv("ZQK_PROJECT_ROOT", tempDir)
	cmd := NewDocmanSyncCmd()
	errRun := runDocmanSync(cmd, []string{})
	require.Error(t, errRun)
	assert.Contains(t, errRun.Error(), "failed to initialize storage factory")
}

func TestLintBypass_RootNotFound(t *testing.T) {
	tempDir := chdirToTemp(t)
	require.NotEmpty(t, tempDir)

	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")

	oldCLICommand := paths.CLICommandName
	paths.CLICommandName = ""
	t.Cleanup(func() {
		paths.CLICommandName = oldCLICommand
	})

	cmd := NewLintBypassAuditCmd()
	err := runLintBypassAudit(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
	assert.Contains(t, err.Error(), paths.CLICommandNameDefault)
}

func TestLintBypass_DetectCommitMessageMergeMsg(t *testing.T) {
	tempDir := chdirToTemp(t)
	require.NotEmpty(t, tempDir)

	initCmd := execwrap.Command("git", "init")
	errInit := initCmd.Run()
	require.NoError(t, errInit)

	// Write MERGE_MSG file inside .git directory without committing anything
	gitDirCmd := execwrap.Command("git", "rev-parse", "--git-dir")
	gitDirOut, errGitDir := gitDirCmd.Output()
	require.NoError(t, errGitDir)
	gitDir := strings.TrimSpace(string(gitDirOut))

	mergeMsgPath := filepath.Join(gitDir, "MERGE_MSG")
	errWrite := fileutil.WriteFile(mergeMsgPath, []byte("Merge branch 'feature-114' into main\n"), paths.FilePerm644)
	require.NoError(t, errWrite)

	msg, errDetect := detectCommitMessage()
	require.NoError(t, errDetect)
	assert.Equal(t, "Merge branch 'feature-114' into main", msg)
}

func TestLintBypass_DetectGitConfigAndStagedErrors(t *testing.T) {
	tempDir := chdirToTemp(t)
	require.NotEmpty(t, tempDir)

	// detectGitConfig returns error on invalid config key
	_, errCfg := detectGitConfig("invalid.nonexistent.key.12345")
	require.Error(t, errCfg)

	// detectStagedFiles returns error when not in a git repo
	_, errStaged := detectStagedFiles()
	require.Error(t, errStaged)
}

func TestLintBypass_CoordinationEmissionEdges(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.lint_coordination_edges",
	})
	factory, errFact := initializeDocmanRegistry(proj.Root)
	require.NoError(t, errFact)
	require.NotNil(t, factory)

	ctx := context.Background()

	// Edge 1: projectRoot == "" or "." returns early safely
	emitLintBypassAuditEventViaCoordinator(ctx, "", nil, "User", "user@test.com", "msg", nil, "")
	emitLintBypassAuditEventViaCoordinator(ctx, ".", nil, "User", "user@test.com", "msg", nil, "")

	// Edge 2: empty gitUser defaults actor to "unknown", commit message > 100 truncated, operationID > 50
	longCommitMsg := strings.Repeat("A very long commit message explanation that exceeds one hundred chars. ", 3)
	longFiles := []string{
		"path/to/an_extremely_long_nested_file_path_for_testing_truncation_1.go",
		"path/to/an_extremely_long_nested_file_path_for_testing_truncation_2.go",
	}

	// Call with empty user to trigger actor == "unknown" and long strings to trigger truncation
	emitLintBypassAuditEventViaCoordinator(
		ctx,
		proj.Root,
		nil,
		"", // empty user triggers "unknown" actor
		"committer@zqkos.com",
		longCommitMsg,
		longFiles,
		automationProfileAIAgent,
	)
	time.Sleep(150 * time.Millisecond)
}

func TestLintBypass_CreateAuditEventFallback(t *testing.T) {
	tempDir := t.TempDir()
	corruptProjectDotZqk(t, tempDir)

	flags := &LintBypassAuditFlags{
		GitUser:       "Craftsman",
		GitEmail:      "craftsman@example.com",
		CommitMessage: "chore: test fallback",
		Files:         []string{"pkg/test.go"},
		AutoDetect:    false,
	}

	ctx := pkgctx.NewSystemContext()
	err := createLintBypassAuditEvent(ctx, tempDir, flags)
	// Fallback executes CreateLintBypassAuditEvent without panic
	assert.True(t, err == nil || strings.Contains(err.Error(), "storage"))
}

func TestLintBypass_ParseFlagsDetailed(t *testing.T) {
	cmd := NewLintBypassAuditCmd()
	errArgs := cmd.ParseFlags([]string{
		"--git-user", "Alice Developer",
		"--git-email", "alice@example.com",
		"--commit-message", "feat: automation feature",
		"--files", "cmd/zqk/main.go",
		"--files", "pkg/storage/store.go",
		"--auto-detect=false",
	})
	require.NoError(t, errArgs)

	flags, errParse := parseLintBypassAuditFlags(cmd)
	require.NoError(t, errParse)
	require.NotNil(t, flags)
	assert.Equal(t, "Alice Developer", flags.GitUser)
	assert.Equal(t, "alice@example.com", flags.GitEmail)
	assert.Equal(t, "feat: automation feature", flags.CommitMessage)
	assert.Equal(t, []string{"cmd/zqk/main.go", "pkg/storage/store.go"}, flags.Files)
	assert.False(t, flags.AutoDetect)
}

func TestLintBypass_ParseFlagsErrorBranches(t *testing.T) {
	// Flag type mismatch: git-user
	cmd1 := &cobra.Command{}
	cmd1.Flags().Int("git-user", 1, "")
	_, err1 := parseLintBypassAuditFlags(cmd1)
	require.Error(t, err1)
	assert.Contains(t, err1.Error(), "git-user")

	// Flag type mismatch: git-email
	cmd2 := &cobra.Command{}
	cmd2.Flags().String("git-user", "test", "")
	cmd2.Flags().Int("git-email", 1, "")
	_, err2 := parseLintBypassAuditFlags(cmd2)
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "git-email")

	// Flag type mismatch: commit-message
	cmd3 := &cobra.Command{}
	cmd3.Flags().String("git-user", "test", "")
	cmd3.Flags().String("git-email", "test@test.com", "")
	cmd3.Flags().Int("commit-message", 1, "")
	_, err3 := parseLintBypassAuditFlags(cmd3)
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "commit-message")

	// Flag type mismatch: files
	cmd4 := &cobra.Command{}
	cmd4.Flags().String("git-user", "test", "")
	cmd4.Flags().String("git-email", "test@test.com", "")
	cmd4.Flags().String("commit-message", "test", "")
	cmd4.Flags().Int("files", 1, "")
	_, err4 := parseLintBypassAuditFlags(cmd4)
	require.Error(t, err4)
	assert.Contains(t, err4.Error(), "files")

	// Flag type mismatch: auto-detect
	cmd5 := &cobra.Command{}
	cmd5.Flags().String("git-user", "test", "")
	cmd5.Flags().String("git-email", "test@test.com", "")
	cmd5.Flags().String("commit-message", "test", "")
	cmd5.Flags().StringArray("files", nil, "")
	cmd5.Flags().Int("auto-detect", 1, "")
	_, err5 := parseLintBypassAuditFlags(cmd5)
	require.Error(t, err5)
	assert.Contains(t, err5.Error(), "auto-detect")

	// runLintBypassAudit returns parseLintBypassAuditFlags error
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.lint_parse_err",
	})
	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	errRun := runLintBypassAudit(cmd1, []string{})
	require.Error(t, errRun)
	assert.Contains(t, errRun.Error(), "git-user")
}

func TestLintBypass_AutoDetectDisabledAndBranches(t *testing.T) {
	// When auto-detect is false, values are untouched
	flagsDisabled := &LintBypassAuditFlags{
		AutoDetect: false,
	}
	autoDetectContext(flagsDisabled)
	assert.Empty(t, flagsDisabled.GitUser)
	assert.Empty(t, flagsDisabled.GitEmail)
	assert.Empty(t, flagsDisabled.CommitMessage)
	assert.Empty(t, flagsDisabled.Files)

	// When values already present and auto-detect is true, values are preserved
	flagsFilled := &LintBypassAuditFlags{
		GitUser:       "Pre-set User",
		GitEmail:      "preset@example.com",
		CommitMessage: "pre-set msg",
		Files:         []string{"existing.go"},
		AutoDetect:    true,
	}
	autoDetectContext(flagsFilled)
	assert.Equal(t, "Pre-set User", flagsFilled.GitUser)
	assert.Equal(t, "preset@example.com", flagsFilled.GitEmail)
	assert.Equal(t, "pre-set msg", flagsFilled.CommitMessage)
	assert.Equal(t, []string{"existing.go"}, flagsFilled.Files)
}
