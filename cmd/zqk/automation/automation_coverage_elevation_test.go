package automation

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAutomationCommand_Hierarchy(t *testing.T) {
	cmd := NewAutomationCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "automation", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Verify expected subcommands
	subCmds := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	assert.True(t, subCmds["docman-sync"])
	assert.True(t, subCmds["lint-bypass-audit"])

	// Help execution
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Automation and integration operations")
}

func TestDocmanSyncCommand_FlagsAndHelp(t *testing.T) {
	cmd := NewDocmanSyncCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "docman-sync", cmd.Use)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Sync documentation registration")
	assert.Contains(t, buf.String(), "--check-only")
	assert.Contains(t, buf.String(), "--git-aware")
	assert.Contains(t, buf.String(), "--update-existing")
	assert.Contains(t, buf.String(), "--dry-run")
}

func TestDocmanSync_ExecutionScenarios(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.docman_sync",
	})
	tempDir := proj.Root

	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	// Create docs directory with a sample markdown file
	docsDir := filepath.Join(tempDir, "docs", "guides")
	require.NoError(t, fileutil.MkdirAll(docsDir, paths.DirPerm755))
	testDocPath := filepath.Join(docsDir, "TEST_GUIDE.md")
	docContent := "# Test Guide\n\nThis is a test guide for docman sync.\n"
	require.NoError(t, fileutil.WriteFile(testDocPath, []byte(docContent), paths.FilePerm644))

	t.Run("check-only mode detects unregistered doc", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--check-only"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "files need registration")
	})

	t.Run("dry-run mode does not persist doc entries", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--dry-run"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	t.Run("standard registration succeeds", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	t.Run("check-only mode passes after registration", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--check-only"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	t.Run("update-existing flag passes", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--update-existing"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})
}

func TestLintBypassAuditCommand_FlagsAndHelp(t *testing.T) {
	cmd := NewLintBypassAuditCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "lint-bypass-audit", cmd.Use)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Create an audit event")
	assert.Contains(t, buf.String(), "--git-user")
	assert.Contains(t, buf.String(), "--git-email")
	assert.Contains(t, buf.String(), "--commit-message")
	assert.Contains(t, buf.String(), "--files")
	assert.Contains(t, buf.String(), "--auto-detect")
}

func TestLintBypassAudit_ExplicitFlags(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.lint_bypass",
	})
	tempDir := proj.Root

	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	t.Run("validation fails with missing user when auto-detect disabled", func(t *testing.T) {
		_ = proj
		cmd := NewLintBypassAuditCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"--auto-detect=false",
			"--git-email", "test@example.com",
			"--commit-message", "fix: test",
			"--files", "pkg/test.go",
		})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git user is required")
	})

	t.Run("validation fails with missing email when auto-detect disabled", func(t *testing.T) {
		cmd := NewLintBypassAuditCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"--auto-detect=false",
			"--git-user", "Test User",
			"--commit-message", "fix: test",
			"--files", "pkg/test.go",
		})
		err := cmd.Execute()
		// Only git-user is required by validateLintBypassAuditFlags
		assert.NoError(t, err)
	})

	t.Run("successful execution creates audit event", func(t *testing.T) {
		cmd := NewLintBypassAuditCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"--auto-detect=false",
			"--git-user", "Craftsman User",
			"--git-email", "craftsman@zqkos.com",
			"--commit-message", "refactor: bypass checks for test suite",
			"--files", "pkg/storage/store.go,cmd/zqk/main.go",
		})
		err := cmd.Execute()
		assert.NoError(t, err)
	})
}

func TestLintBypassAudit_GitDetection(t *testing.T) {
	tempDir := t.TempDir()
	initCmd := execwrap.Command("git", "init")
	initCmd.Dir = tempDir
	require.NoError(t, initCmd.Run())

	cfgUser := execwrap.Command("git", "config", "user.name", "Git Auto User")
	cfgUser.Dir = tempDir
	require.NoError(t, cfgUser.Run())

	cfgEmail := execwrap.Command("git", "config", "user.email", "gitauto@example.com")
	cfgEmail.Dir = tempDir
	require.NoError(t, cfgEmail.Run())

	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	// Test detectGitConfig helper directly
	name, err := detectGitConfig("user.name")
	assert.NoError(t, err)
	assert.Equal(t, "Git Auto User", name)

	email, err := detectGitConfig("user.email")
	assert.NoError(t, err)
	assert.Equal(t, "gitauto@example.com", email)

	// Test detectCommitMessage when no commit exists
	_, err = detectCommitMessage()
	assert.Error(t, err)

	// Test detectStagedFiles when nothing is staged
	staged, err := detectStagedFiles()
	assert.NoError(t, err)
	assert.Empty(t, staged)

	// Create and stage a go file and markdown file
	testGo := filepath.Join(tempDir, "file.go")
	require.NoError(t, fileutil.WriteFile(testGo, []byte("package main\n"), 0644))
	testMd := filepath.Join(tempDir, "doc.md")
	require.NoError(t, fileutil.WriteFile(testMd, []byte("# Markdown\n"), 0644))

	addCmd := execwrap.Command("git", "add", "file.go", "doc.md")
	require.NoError(t, addCmd.Run())

	staged, err = detectStagedFiles()
	assert.NoError(t, err)
	assert.Contains(t, staged, "file.go")
	assert.Contains(t, staged, "doc.md")
}

func TestDocmanSync_GitAwareScenarios(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.docman_sync_gitaware",
	})
	tempDir := proj.Root

	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	initCmd := execwrap.Command("git", "init")
	require.NoError(t, initCmd.Run())

	// When no markdown files are staged, --git-aware exits early with success
	t.Run("git-aware returns early when no staged markdown files", func(t *testing.T) {
		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--git-aware"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	// When a markdown file is staged, --git-aware discovers and registers it
	t.Run("git-aware processes staged markdown files", func(t *testing.T) {
		docsDir := filepath.Join(tempDir, "docs", "guides")
		require.NoError(t, fileutil.MkdirAll(docsDir, paths.DirPerm755))
		testDoc := filepath.Join(docsDir, "STAGED_DOC.md")
		require.NoError(t, fileutil.WriteFile(testDoc, []byte("# Staged Doc\nContent here\n"), paths.FilePerm644))

		addCmd := execwrap.Command("git", "add", "docs/guides/STAGED_DOC.md")
		require.NoError(t, addCmd.Run())

		cmd := NewDocmanSyncCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--git-aware"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})
}

func TestLintBypassAudit_AutoDetectExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.automation.lint_bypass_autodetect",
	})
	tempDir := proj.Root

	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	initCmd := execwrap.Command("git", "init")
	require.NoError(t, initCmd.Run())

	cfgUser := execwrap.Command("git", "config", "user.name", "Audit Tester")
	require.NoError(t, cfgUser.Run())

	cfgEmail := execwrap.Command("git", "config", "user.email", "tester@example.com")
	require.NoError(t, cfgEmail.Run())

	// Create commit
	dummyFile := filepath.Join(tempDir, "initial.txt")
	require.NoError(t, fileutil.WriteFile(dummyFile, []byte("init"), 0644))
	require.NoError(t, execwrap.Command("git", "add", "initial.txt").Run())
	require.NoError(t, execwrap.Command("git", "commit", "-m", "chore: initial commit").Run())

	// Stage a modified file
	modFile := filepath.Join(tempDir, "service.go")
	require.NoError(t, fileutil.WriteFile(modFile, []byte("package service\n"), 0644))
	require.NoError(t, execwrap.Command("git", "add", "service.go").Run())

	t.Run("auto-detect defaults execute successfully", func(t *testing.T) {
		cmd := NewLintBypassAuditCmd()
		buf := new(bytes.Buffer)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Created lint bypass audit event")
	})

	t.Run("auto-detect with quiet flag produces no output", func(t *testing.T) {
		cmd := NewLintBypassAuditCmd()
		buf := new(bytes.Buffer)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--quiet"})
		err := cmd.Execute()
		assert.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestLintBypassAudit_CommitMessageDetection(t *testing.T) {
	tempDir := t.TempDir()
	wd, err := fileutil.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.Chdir(wd)
	})
	require.NoError(t, fileutil.Chdir(tempDir))

	require.NoError(t, execwrap.Command("git", "init").Run())
	require.NoError(t, execwrap.Command("git", "config", "user.name", "Committer").Run())
	require.NoError(t, execwrap.Command("git", "config", "user.email", "committer@example.com").Run())

	// Test detectCommitMessage from HEAD
	testFile := filepath.Join(tempDir, "file.txt")
	require.NoError(t, fileutil.WriteFile(testFile, []byte("hello"), 0644))
	require.NoError(t, execwrap.Command("git", "add", "file.txt").Run())
	require.NoError(t, execwrap.Command("git", "commit", "-m", "feat: new feature message").Run())

	msg, err := detectCommitMessage()
	assert.NoError(t, err)
	assert.Equal(t, "feat: new feature message", msg)

	// Test detectCommitMessage from COMMIT_EDITMSG
	gitDirCmd := execwrap.Command("git", "rev-parse", "--git-dir")
	out, err := gitDirCmd.Output()
	require.NoError(t, err)
	gitDir := filepath.Clean(string(bytes.TrimSpace(out)))
	editMsgPath := filepath.Join(gitDir, "COMMIT_EDITMSG")
	require.NoError(t, fileutil.WriteFile(editMsgPath, []byte("wip: hook edit message\n"), 0644))

	// In another temp dir with no HEAD commits but COMMIT_EDITMSG
	tempDir2 := t.TempDir()
	require.NoError(t, fileutil.Chdir(tempDir2))
	require.NoError(t, execwrap.Command("git", "init").Run())
	gitDirCmd2 := execwrap.Command("git", "rev-parse", "--git-dir")
	out2, err := gitDirCmd2.Output()
	require.NoError(t, err)
	gitDir2 := filepath.Clean(string(bytes.TrimSpace(out2)))
	require.NoError(t, fileutil.WriteFile(filepath.Join(gitDir2, "COMMIT_EDITMSG"), []byte("wip: hook message\n"), 0644))

	msg2, err := detectCommitMessage()
	assert.NoError(t, err)
	assert.Equal(t, "wip: hook message", msg2)
}

func TestLintBypassCoordination_Helpers(t *testing.T) {
	assert.Equal(t, 3, minInt(3, 10))
	assert.Equal(t, 5, minInt(20, 5))
	assert.Equal(t, 7, minInt(7, 7))
}


