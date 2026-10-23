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
)

func TestParseGrepOptions_FlagsError(t *testing.T) {
	t.Parallel()

	// When cmd is nil, FlagBag.ready() fails and sets an error on the bag.
	_, _, _, err := parseGrepOptions(nil, []string{"search-query"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cobra.Command is nil")
}

func TestParseGrepOptions_JsonAndJsonlTokens(t *testing.T) {
	t.Parallel()

	cmdJSON := NewGrepCmd()
	err := cmdJSON.ParseFlags([]string{"--format", "json", "--max-tokens", "0"})
	require.NoError(t, err)
	optsJSON, reindexJSON, formatJSON, err := parseGrepOptions(cmdJSON, []string{"sampleQuery"})
	require.NoError(t, err)
	assert.False(t, reindexJSON)
	assert.Equal(t, "json", formatJSON)
	assert.Equal(t, search.DefaultMaxTokens, optsJSON.MaxTokens)

	cmdJSONL := NewGrepCmd()
	err = cmdJSONL.ParseFlags([]string{"--format", "jsonl", "--max-tokens", "-1"})
	require.NoError(t, err)
	optsJSONL, _, formatJSONL, err := parseGrepOptions(cmdJSONL, []string{"sampleQuery"})
	require.NoError(t, err)
	assert.Equal(t, "jsonl", formatJSONL)
	assert.Equal(t, search.DefaultMaxTokens, optsJSONL.MaxTokens)
}

func TestParseGrepOptions_CommaSeparatedExt(t *testing.T) {
	t.Parallel()

	cmd := NewGrepCmd()
	err := cmd.ParseFlags([]string{"--ext", "go, txt , md", "--ext", "yaml"})
	require.NoError(t, err)
	opts, _, _, err := parseGrepOptions(cmd, []string{"sampleQuery"})
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "txt", "md", "yaml"}, opts.FileExtensions)
}

func TestResolveQueryAndPath_Matrix(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 0 args with empty pathFlag
	q, p := resolveQueryAndPath([]string{}, "", false, "", "", false)
	assert.Equal(t, "", q)
	assert.Equal(t, ".", p)

	// 0 args with non-empty pathFlag
	q, p = resolveQueryAndPath([]string{}, tempDir, false, "", "", false)
	assert.Equal(t, "", q)
	assert.Equal(t, tempDir, p)

	// 1 arg with existing directory -> treated as path
	q, p = resolveQueryAndPath([]string{tempDir}, "", false, "", "", false)
	assert.Equal(t, "", q)
	assert.Equal(t, tempDir, p)

	// 1 arg with empty pathFlag, structural query and slash -> treated as path
	q, p = resolveQueryAndPath([]string{"sub/dir"}, "", true, "", "", false)
	assert.Equal(t, "", q)
	assert.Equal(t, "sub/dir", p)

	// 1 arg non-existent with slash and non-structural -> treated as query
	q, p = resolveQueryAndPath([]string{"some/query"}, "", false, "", "", false)
	assert.Equal(t, "some/query", q)
	assert.Equal(t, ".", p)

	// 1 arg with empty pathFlag, structural query without slash and not existing -> treated as query
	q, p = resolveQueryAndPath([]string{"sample"}, "", true, "", "", false)
	assert.Equal(t, "sample", q)
	assert.Equal(t, ".", p)

	// 1 arg with empty pathFlag normal query -> treated as query
	q, p = resolveQueryAndPath([]string{"MyFunc"}, "", false, "", "", false)
	assert.Equal(t, "MyFunc", q)
	assert.Equal(t, ".", p)

	// 1 arg with pathFlag set -> treated as query
	q, p = resolveQueryAndPath([]string{"MyFunc"}, "custom/path", false, "", "", false)
	assert.Equal(t, "MyFunc", q)
	assert.Equal(t, "custom/path", p)

	// 2 args with empty pathFlag
	q, p = resolveQueryAndPath([]string{"MyFunc", "pkg/search"}, "", false, "", "", false)
	assert.Equal(t, "MyFunc", q)
	assert.Equal(t, "pkg/search", p)

	// 2 args with pathFlag set -> pathFlag overrides args[1]
	q, p = resolveQueryAndPath([]string{"MyFunc", "pkg/search"}, "override/path", false, "", "", false)
	assert.Equal(t, "MyFunc", q)
	assert.Equal(t, "override/path", p)
}

func TestIsDirOrPathTarget_Matrix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.txt")
	err := fileutil.WriteFile(filePath, []byte("content"), paths.FilePerm644)
	require.NoError(t, err)

	// Existing directory
	assert.True(t, isDirOrPathTarget(dir, false, "", "", false))

	// Existing file without slash in name (relative) -> false
	assert.False(t, isDirOrPathTarget("sample.txt", false, "", "", false))

	// Structural flags active (astMode, astKind, astReceiver, reindex)
	assert.True(t, isDirOrPathTarget("sample/dir", true, "", "", false))
	assert.True(t, isDirOrPathTarget("sample/dir", false, "struct", "", false))
	assert.True(t, isDirOrPathTarget("sample/dir", false, "", "Receiver", false))
	assert.True(t, isDirOrPathTarget("sample/dir", false, "", "", true))

	// Non-existent target without slash and non-structural
	assert.False(t, isDirOrPathTarget("nonexistentsingletoken", false, "", "", false))
}

func TestRenderGrepResult_AllFormatsAndVariations(t *testing.T) {
	t.Parallel()

	sampleResult := &search.SearchResult{
		TotalMatches:    3,
		EstimatedTokens: 42,
		Matches: []search.Match{
			{
				File:          "engine.go",
				Line:          10,
				Column:        1,
				LineContent:   "func (e *Engine) Start() error {",
				SymbolKind:    "method",
				Receiver:      "Engine",
				SymbolName:    "Start",
				ContextBefore: []string{"// Start documentation line 1", "// Start documentation line 2"},
				ContextAfter:  []string{"\treturn nil", "}"},
			},
			{
				File:         "engine.go",
				Line:         20,
				Column:       1,
				LineContent:  "type Engine struct {",
				SymbolKind:   "struct",
				Receiver:     "",
				SymbolName:   "Engine",
				ContextAfter: []string{"\tID string", "}"},
			},
			{
				File:          "notes.txt",
				Line:          5,
				Column:        3,
				LineContent:   "Plain note line without symbol.",
				ContextBefore: []string{"Header:"},
			},
		},
	}

	// 1. Text rendering
	var textBuf bytes.Buffer
	err := renderGrepResult(&textBuf, "text", sampleResult)
	require.NoError(t, err)
	textOut := textBuf.String()
	assert.Contains(t, textOut, "engine.go- // Start documentation line 1")
	assert.Contains(t, textOut, "engine.go:10:1: [method] (Engine) Start: func (e *Engine) Start() error {")
	assert.Contains(t, textOut, "engine.go+ \treturn nil")
	assert.Contains(t, textOut, "engine.go:20:1: [struct] Engine: type Engine struct {")
	assert.Contains(t, textOut, "notes.txt:5:3: Plain note line without symbol.")

	// 2. JSON rendering
	var jsonBuf bytes.Buffer
	err = renderGrepResult(&jsonBuf, "json", sampleResult)
	require.NoError(t, err)
	var decodedJSON search.SearchResult
	err = json.Unmarshal(jsonBuf.Bytes(), &decodedJSON)
	require.NoError(t, err)
	assert.Equal(t, 3, decodedJSON.TotalMatches)

	// 3. YAML rendering
	var yamlBuf bytes.Buffer
	err = renderGrepResult(&yamlBuf, "yaml", sampleResult)
	require.NoError(t, err)
	assert.Contains(t, yamlBuf.String(), "total_matches: 3")
}

func TestRunGrepExecution_ContextLinesFlag(t *testing.T) {
	t.Parallel()

	dir := setupTestCodebase(t)
	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--context", "1", "Start", dir})

	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "engine.go- ")
	assert.Contains(t, output, "Start")
	assert.Contains(t, output, "engine.go+ ")
}

func TestRunGrepExecution_ReindexOnlyNoQuery(t *testing.T) {
	t.Parallel()

	dir := setupTestCodebase(t)
	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--reindex", dir})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Trigram index successfully rebuilt and cached.")
}

func TestRunGrepExecution_ReindexWithQuery(t *testing.T) {
	t.Parallel()

	dir := setupTestCodebase(t)
	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--reindex", "Engine", dir})

	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Trigram index successfully rebuilt and cached.")
	assert.Contains(t, output, "engine.go")
}

func TestRunGrepExecution_ReindexFailure(t *testing.T) {
	t.Parallel()

	nonExistentAbsPath := filepath.Join(t.TempDir(), "nonexistent-target-subpath")
	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--reindex", nonExistentAbsPath})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed rebuilding trigram index")
}

func TestRunGrepExecution_AbsPathSearch(t *testing.T) {
	t.Parallel()

	dir := setupTestCodebase(t)
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err)

	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Engine", absDir})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "engine.go")
}
