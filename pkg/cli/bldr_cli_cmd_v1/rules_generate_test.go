package bldr_cli_cmd_v1_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestRulesGenerate_AllDirectivesContent(t *testing.T) {
	directives := bldr.KnownDirectives()
	require.Len(t, directives, 4)

	expectedFiles := map[string]bldr.TargetHost{
		".cursorrules":      bldr.HostCursor,
		"CLAUDE.md":         bldr.HostClaude,
		".windsurfrules":    bldr.HostWindsurf,
		".agents/AGENTS.md": bldr.HostAgents,
	}

	for _, d := range directives {
		host, ok := expectedFiles[d.RelPath]
		require.True(t, ok, "unexpected directive path %s", d.RelPath)
		assert.Equal(t, host, d.Host)

		// Verification: sub-15ms zgrep AST search queries
		assert.Contains(t, d.Content, "zgrep", "must reference zgrep in %s", d.RelPath)
		assert.Contains(t, d.Content, "sub-15ms", "must mention sub-15ms in %s", d.RelPath)
		assert.Contains(t, d.Content, "--ast --kind struct", "must mention AST struct search in %s", d.RelPath)
		assert.Contains(t, d.Content, "--ast --kind func", "must mention AST func search in %s", d.RelPath)
		assert.Contains(t, d.Content, "--ast --recv", "must mention AST recv search in %s", d.RelPath)
		assert.Contains(t, d.Content, "90%", "must mention 90% token savings in %s", d.RelPath)

		// Verification: Persistent Knowledge Kernel rules & done-gates & anti-hallucination
		assert.Contains(t, d.Content, "Persistent Knowledge Kernel", "must mention Persistent Knowledge Kernel in %s", d.RelPath)
		assert.Contains(t, d.Content, "Done-Gates", "must mention Done-Gates in %s", d.RelPath)
		assert.Contains(t, d.Content, "Anti-Hallucination Protocol", "must mention Anti-Hallucination Protocol in %s", d.RelPath)
		assert.Contains(t, d.Content, "zqk test run", "must reference zqk test run in %s", d.RelPath)
	}
}

func TestRulesGenerate_ResolveDirectives(t *testing.T) {
	// Default / empty / all
	all := bldr.ResolveDirectives(nil)
	assert.Len(t, all, 4)

	allExplicit := bldr.ResolveDirectives([]string{"all"})
	assert.Len(t, allExplicit, 4)

	// Single host
	cursorOnly := bldr.ResolveDirectives([]string{"cursor"})
	require.Len(t, cursorOnly, 1)
	assert.Equal(t, ".cursorrules", cursorOnly[0].RelPath)

	claudeOnly := bldr.ResolveDirectives([]string{"claude"})
	require.Len(t, claudeOnly, 1)
	assert.Equal(t, "CLAUDE.md", claudeOnly[0].RelPath)

	windsurfOnly := bldr.ResolveDirectives([]string{"windsurf"})
	require.Len(t, windsurfOnly, 1)
	assert.Equal(t, ".windsurfrules", windsurfOnly[0].RelPath)

	agentsOnly := bldr.ResolveDirectives([]string{"agents"})
	require.Len(t, agentsOnly, 1)
	assert.Equal(t, ".agents/AGENTS.md", agentsOnly[0].RelPath)

	// Comma-separated hosts
	multi := bldr.ResolveDirectives([]string{"cursor,claude"})
	require.Len(t, multi, 2)
	assert.Equal(t, ".cursorrules", multi[0].RelPath)
	assert.Equal(t, "CLAUDE.md", multi[1].RelPath)
}

func TestRulesGenerate_FileCreationAndNonDestructive(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Initial write of all files
	res, err := bldr.GenerateRules(tmpDir, bldr.RulesGenerateOptions{
		Hosts: []string{"all"},
	})
	require.NoError(t, err)
	assert.Len(t, res.Written, 4)
	assert.Empty(t, res.Skipped)

	// Verify files actually exist on disk
	for _, rel := range []string{".cursorrules", "CLAUDE.md", ".windsurfrules", ".agents/AGENTS.md"} {
		p := filepath.Join(tmpDir, filepath.FromSlash(rel))
		assert.FileExists(t, p)
	}

	// 2. Non-destructive writes without force: all should be skipped
	resSecond, err := bldr.GenerateRules(tmpDir, bldr.RulesGenerateOptions{
		Hosts: []string{"all"},
		Force: false,
	})
	require.NoError(t, err)
	assert.Empty(t, resSecond.Written)
	assert.Len(t, resSecond.Skipped, 4)

	// 3. Overwrite with force
	resForce, err := bldr.GenerateRules(tmpDir, bldr.RulesGenerateOptions{
		Hosts: []string{"all"},
		Force: true,
	})
	require.NoError(t, err)
	assert.Len(t, resForce.Written, 4)
	assert.Empty(t, resForce.Skipped)
}

func TestRulesGenerate_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	res, err := bldr.GenerateRules(tmpDir, bldr.RulesGenerateOptions{
		Hosts:  []string{"cursor", "claude"},
		DryRun: true,
	})
	require.NoError(t, err)
	assert.Len(t, res.Written, 2)
	assert.Len(t, res.Previews, 2)

	// Verify NO files were written to disk
	for _, rel := range []string{".cursorrules", "CLAUDE.md"} {
		p := filepath.Join(tmpDir, filepath.FromSlash(rel))
		assert.NoFileExists(t, p, "file %s must NOT exist during dry run", rel)
	}
}

func TestRulesGenerate_CommandExecution(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := bldr.NewRulesGenerateCommand()
	require.NotNil(t, cmd)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), buf))
	cmd.SetArgs([]string{"--host", "cursor"})

	err := cmd.Execute()
	require.NoError(t, err)

	cursorPath := filepath.Join(tmpDir, ".cursorrules")
	assert.FileExists(t, cursorPath, ".cursorrules should exist after command execute")
}

func TestRulesGenerate_InitRulesOnlyFlag(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := system.NewInitCmd()
	require.NotNil(t, initCmd)

	buf := &bytes.Buffer{}
	initCmd.SetOut(buf)
	initCmd.SetErr(buf)
	cli.SetContext(initCmd, cli.ContextForProjectRoot(tmpDir))
	initCmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), buf))
	initCmd.SetArgs([]string{"--rules-only"})

	err := initCmd.Execute()
	require.NoError(t, err)

	// Verify all 4 files generated by zqk init --rules-only
	for _, rel := range []string{".cursorrules", "CLAUDE.md", ".windsurfrules", ".agents/AGENTS.md"} {
		p := filepath.Join(tmpDir, filepath.FromSlash(rel))
		assert.FileExists(t, p, "%s should exist after init --rules-only", rel)
	}

	// Verify that normal full project init dirs (e.g. .zqk/process) were NOT created
	processDir := filepath.Join(tmpDir, ".zqk", "process")
	_, err = os.Stat(processDir)
	assert.True(t, os.IsNotExist(err), ".zqk/process should NOT be created with --rules-only")
}
