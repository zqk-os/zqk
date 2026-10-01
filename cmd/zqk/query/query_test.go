package query_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/query"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestQueryCmd_Help(t *testing.T) {
	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "ZPARQL")
}

func TestQueryCmd_ExecuteFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	queryFile := filepath.Join(tmpDir, "test.zparql")
	err := fileutil.WriteFile(queryFile, []byte("MATCH (b:backlog_item) RETURN b.id;"), 0644)
	require.NoError(t, err)

	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"-f", queryFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"headers"`)
}

func TestQueryCmd_ExecuteDirectString(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"MATCH (b:backlog_item) RETURN b.id;", "--format", "table"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "No matches found.")
}

func TestQueryCmd_ExecuteWildcard(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"MATCH (n) RETURN n.id;", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"headers"`)
}

func TestQueryCmd_InteractiveREPL_HelpAndExit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := query.NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString(".help\n.exit\n"))
	cmd.SetArgs([]string{"--interactive"})

	err := cmd.Execute()
	require.NoError(t, err)
	output := outBuf.String()
	require.Contains(t, output, "ZPARQL Interactive Graph Query Console")
	require.Contains(t, output, "Available Meta-Commands:")
	require.Contains(t, output, ".help")
	require.Contains(t, output, ".kinds")
	require.Contains(t, output, "Exiting ZPARQL console. Goodbye!")
}

func TestQueryCmd_InteractiveREPL_MetaCommands(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	input := ".kinds\n.tables\n.schema\n.schema goal\n.format json\n.visualize on\n.clear\n.quit\n"

	cmd := query.NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString(input))
	cmd.SetArgs([]string{"-i"})

	err := cmd.Execute()
	require.NoError(t, err)
	output := outBuf.String()
	require.Contains(t, output, "Indexed Object Kinds:")
	require.Contains(t, output, "Indexed Graph Relationships & Edges:")
	require.Contains(t, output, "Available Schemas")
	require.Contains(t, output, "Output format set to: json")
	require.Contains(t, output, "Graph path visualizer mode enabled.")
	require.Contains(t, output, "Query buffer cleared.")
	require.Contains(t, output, "Exiting ZPARQL console. Goodbye!")
}

func TestQueryCmd_InteractiveREPL_MultiLineQuery(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	input := "MATCH (b:backlog_item)\nRETURN b.id;\n.exit\n"

	cmd := query.NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString(input))
	cmd.SetArgs([]string{"repl"})

	err := cmd.Execute()
	require.NoError(t, err)
	output := outBuf.String()
	require.Contains(t, output, "ZPARQL Interactive Graph Query Console")
	require.Contains(t, output, "No matches found")
	require.Contains(t, output, "Exiting ZPARQL console. Goodbye!")
}

func TestQueryCmd_VisualizeFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"MATCH (n) RETURN n.id;", "--visualize"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "ZPARQL Graph Path Visualization")
}
