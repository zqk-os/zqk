package grep

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
	"gopkg.in/yaml.v3"
)

func TestNewGrepCmd(t *testing.T) {
	t.Parallel()

	cmd := NewGrepCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "grep [query] [path]", cmd.Use)
	assert.Contains(t, cmd.Aliases, "zgrep")
	assert.NotNil(t, cmd.Flags().Lookup("ignore-case"))
	assert.NotNil(t, cmd.Flags().Lookup("ast"))
	assert.NotNil(t, cmd.Flags().Lookup("max-tokens"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
}

func TestAuditGrep(t *testing.T) {
	auditor := qa.NewASTAuditor()
	violations, err := auditor.AuditFile("grep.go")
	require.NoError(t, err)
	for _, v := range violations {
		if v.Severity == "high" || v.Severity == "medium" || v.Severity == "error" {
			t.Errorf("Prohibited violation [%s] %s at line %d: %s", v.Severity, v.Type, v.Pos.Line, v.Message)
		}
	}
}

func setupTestCodebase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	goCode := `package sample

// Engine performs quantum orchestration.
type Engine struct {
	ID string
}

// Start initiates the engine loop.
func (e *Engine) Start() error {
	return nil
}

// StopEngine halts the engine.
func StopEngine() bool {
	return true
}
`
	txtContent := "Quantum engine notes and Engine documentation.\n"

	err := fileutil.WriteFile(filepath.Join(dir, "engine.go"), []byte(goCode), paths.FilePerm644)
	require.NoError(t, err)

	err = fileutil.WriteFile(filepath.Join(dir, "notes.txt"), []byte(txtContent), paths.FilePerm644)
	require.NoError(t, err)

	return dir
}

// TestGrepCmd_FunctionalAcceptance satisfies CRIT-1791580891099223000-6d0fbb45:
// AST indexing, trigram candidate search, receiver queries, and token budgeting.
func TestGrepCmd_FunctionalAcceptance(t *testing.T) {
	dir := setupTestCodebase(t)

	t.Run("literal text search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"Engine", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "engine.go")
		assert.Contains(t, output, "notes.txt")
	})

	t.Run("case-insensitive search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--ignore-case", "quantum", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "engine.go")
		assert.Contains(t, output, "notes.txt")
	})

	t.Run("ast mode - struct search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--ast", "--kind", "struct", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "engine.go")
		assert.Contains(t, output, "[struct]")
		assert.Contains(t, output, "Engine")
		assert.NotContains(t, output, "notes.txt")
	})

	t.Run("ast mode - receiver search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--ast", "--recv", "Engine", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "engine.go")
		assert.Contains(t, output, "(Engine)")
		assert.Contains(t, output, "Start")
		assert.NotContains(t, output, "StopEngine")
	})

	t.Run("ast mode - function search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--ast", "--kind", "func", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Start")
		assert.Contains(t, output, "StopEngine")
	})

	t.Run("token budgeted json output", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"Engine", dir, "-f", "json", "--max-tokens", "2000"})

		err := cmd.Execute()
		require.NoError(t, err)

		var res search.SearchResult
		err = json.Unmarshal(buf.Bytes(), &res)
		require.NoError(t, err)
		assert.NotEmpty(t, res.Matches)
		assert.Greater(t, res.EstimatedTokens, 0)
		assert.False(t, res.Truncated)
	})

	t.Run("yaml output format", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"Engine", dir, "-f", "yaml"})

		err := cmd.Execute()
		require.NoError(t, err)

		var res search.SearchResult
		err = yaml.Unmarshal(buf.Bytes(), &res)
		require.NoError(t, err)
		assert.NotEmpty(t, res.Matches)
	})

	t.Run("reindex trigram cache", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--reindex", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Trigram index successfully rebuilt")
	})
}

// TestGrepCmd_BoundaryAndErrorHandling satisfies CRIT-1791580891099224000-1e96aac4:
// Missing queries, regex syntax errors, invalid paths, and budgeting caps.
func TestGrepCmd_BoundaryAndErrorHandling(t *testing.T) {
	dir := setupTestCodebase(t)

	t.Run("missing search query error", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "search query required")
	})

	t.Run("path flag specified explicitly", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"Engine", "--path", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "engine.go")
	})

	t.Run("non-existent directory", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"Engine", filepath.Join(dir, "nonexistent-subpath-xyz")})

		err := cmd.Execute()
		require.Error(t, err)
	})

	t.Run("valid regex search", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--regex", "func.*Start", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "func (e *Engine) Start()")
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--regex", "[unclosed-bracket(", dir})

		err := cmd.Execute()
		require.Error(t, err)
	})

	t.Run("word match ignores substrings", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--word-regexp", "Eng", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})

	t.Run("file extension filter", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--ext", "txt", "Engine", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "notes.txt")
		assert.NotContains(t, output, "engine.go")
	})

	t.Run("max token truncation with small token budget", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		// Set token budget low enough to force truncation
		cmd.SetArgs([]string{"Engine", dir, "-f", "json", "--max-tokens", "10"})

		err := cmd.Execute()
		require.NoError(t, err)

		var res search.SearchResult
		err = json.Unmarshal(buf.Bytes(), &res)
		require.NoError(t, err)
		assert.True(t, res.Truncated)
		assert.Equal(t, "max_tokens", res.TruncateReason)
	})

	t.Run("empty matches result is clean exit", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"NonExistentTokenPattern12345XYZ", dir})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}
