package query_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/query"
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
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := query.NewQueryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"MATCH (n) RETURN n.id;", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"headers"`)
}

