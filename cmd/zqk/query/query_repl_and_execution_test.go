package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/traversal"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

type cycle138ErrorReader struct {
	err error
}

func (r *cycle138ErrorReader) Read(p []byte) (n int, err error) {
	if r.err != nil {
		return 0, r.err
	}
	return 0, errors.New("simulated read failure")
}

func setupCycle138TestKernel(t *testing.T) (string, *cli.Processor) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(tmpDir)
		teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
		if teardownErr != nil {
			t.Logf("cleanup teardown error: %v", teardownErr)
		}
	})
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	seedCycle138Entities(t, tmpDir)
	return tmpDir, proc
}

func seedCycle138Entities(t *testing.T, tmpDir string) {
	t.Helper()
	goalDir := filepath.Join(tmpDir, ".zqk", "process", "goals")
	planDir := filepath.Join(tmpDir, ".zqk", "process", "priority_plans")
	bliDir := filepath.Join(tmpDir, ".zqk", "process", "backlog_items")
	critDir := filepath.Join(tmpDir, ".zqk", "process", "criteria")

	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(planDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(bliDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(critDir, paths.DirPerm755))

	writeKernelEntity(t, goalDir, "GOAL-ROOT-001.yaml", map[string]any{
		objects.FieldKeyID:    "GOAL-ROOT-001",
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Root Strategic Compass",
		"status":              "active",
	})

	writeKernelEntity(t, goalDir, "GOAL-C138-001.yaml", map[string]any{
		objects.FieldKeyID:    "GOAL-C138-001",
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Cycle 138 Query Subsystem Elevation",
		"status":              "active",
		"depends_on":          "GOAL-ROOT-001",
		"priority_plan_ref":   "PRI-C138-001",
	})

	writeKernelEntity(t, planDir, "PRI-C138-001.yaml", map[string]any{
		objects.FieldKeyID:    "PRI-C138-001",
		objects.FieldKeyKind:  "priority_plan",
		objects.FieldKeyTitle: "Query Subsystem Elevation Sprint",
		"status":              "in_progress",
		"items":               []string{"BLI-C138-001", "BLI-C138-002"},
	})

	writeKernelEntity(t, bliDir, "BLI-C138-001.yaml", map[string]any{
		objects.FieldKeyID:    "BLI-C138-001",
		objects.FieldKeyKind:  "backlog_item",
		objects.FieldKeyTitle: "Elevate Query Subsystem Tests",
		"status":              "completed",
		"priority_plan_refs":  []string{"PRI-C138-001"},
		"criteria_refs":       []string{"CRIT-C138-001"},
	})

	writeKernelEntity(t, critDir, "CRIT-C138-001.yaml", map[string]any{
		objects.FieldKeyID:    "CRIT-C138-001",
		objects.FieldKeyKind:  "criteria",
		objects.FieldKeyTitle: "Statement Coverage Exceeds Threshold",
		"status":              "verified",
	})
}

func writeKernelEntity(t *testing.T, dir, filename string, data map[string]any) {
	t.Helper()
	raw, err := yaml.Marshal(data)
	require.NoError(t, err)
	err = fileutil.WriteFile(filepath.Join(dir, filename), raw, paths.FilePerm644)
	require.NoError(t, err)
}

func TestCycle138_CommandFlagsAndHelp(t *testing.T) {
	cmd := NewQueryCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "query", cmd.Use)
	assert.Contains(t, cmd.Aliases, "zparql")

	assert.NotNil(t, cmd.Flags().Lookup("interactive"))
	assert.NotNil(t, cmd.Flags().Lookup("visualize"))
	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "ZPARQL")
	assert.Contains(t, helpOutput, "interactive")
	assert.Contains(t, helpOutput, "visualize")
}

func TestCycle138_RunQuery_ArgumentAndFileIntake(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotNil(t, proc)

	// Missing arguments
	cmd := NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query string required as argument")

	// Non-existent file
	cmd = NewQueryCmd()
	cmd.SetArgs([]string{"-f", filepath.Join(tmpDir, "non_existent.zparql")})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query file")

	// Valid file
	queryFile := filepath.Join(tmpDir, "valid_query.zparql")
	err = fileutil.WriteFile(queryFile, []byte("MATCH (g:goal) RETURN g.id;"), paths.FilePerm644)
	require.NoError(t, err)

	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"-f", queryFile})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "GOAL-C138-001")
}

func TestCycle138_RunQuery_StdinIntake(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// File flag with '-'
	cmd := NewQueryCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetIn(bytes.NewBufferString("MATCH (g:goal) RETURN g.id;"))
	cmd.SetArgs([]string{"-f", "-"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "GOAL-C138-001")

	// Positional argument with '-'
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetIn(bytes.NewBufferString("MATCH (g:goal) RETURN g.id;"))
	cmd.SetArgs([]string{"-"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "GOAL-C138-001")
}

func TestCycle138_RunQuery_StdinReadErrors(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// File flag '-' with error reader
	cmd := NewQueryCmd()
	cmd.SetIn(&cycle138ErrorReader{})
	cmd.SetArgs([]string{"-f", "-"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query from stdin")

	// Positional argument '-' with error reader
	cmd = NewQueryCmd()
	cmd.SetIn(&cycle138ErrorReader{})
	cmd.SetArgs([]string{"-"})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query from stdin")
}

func TestCycle138_RunQuery_SyntaxError(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	cmd := NewQueryCmd()
	cmd.SetArgs([]string{"MATCH (invalid syntax !!!;"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ZPARQL query syntax error")
}

func TestCycle138_RunQuery_OutputFormatsAndEmptyMatches(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// Empty matches - Table
	cmd := NewQueryCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (b:backlog_item) WHERE b.status == 'non_existent_status' RETURN b.id;"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "No matches found.")

	// Empty matches - JSON
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (b:backlog_item) WHERE b.status == 'non_existent_status' RETURN b.id;", "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err)
	var jsonRes traversal.QueryResult
	err = json.Unmarshal(outBuf.Bytes(), &jsonRes)
	require.NoError(t, err)
	assert.Empty(t, jsonRes.Rows)

	// Empty matches - Visual
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (b:backlog_item) WHERE b.status == 'non_existent_status' RETURN b.id;", "--visualize"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "No graph matches found to visualize.")
}

func TestCycle138_RunQuery_MatchedResultsTableAndJSON(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// Table output with rows
	cmd := NewQueryCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (g:goal) WHERE g.id == 'GOAL-C138-001' RETURN g.id, g.title;"})
	err := cmd.Execute()
	require.NoError(t, err)
	output := outBuf.String()
	assert.Contains(t, output, "g.id")
	assert.Contains(t, output, "g.title")
	assert.Contains(t, output, "GOAL-C138-001")
	assert.Contains(t, output, "1 rows")

	// JSON output with rows
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (g:goal) WHERE g.id == 'GOAL-C138-001' RETURN g.id, g.title;", "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err)
	var jsonRes traversal.QueryResult
	err = json.Unmarshal(outBuf.Bytes(), &jsonRes)
	require.NoError(t, err)
	assert.Len(t, jsonRes.Rows, 1)
	assert.Equal(t, "GOAL-C138-001", jsonRes.Rows[0]["g.id"])

	// Visualizer output with rows
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"MATCH (g:goal) WHERE g.id == 'GOAL-C138-001' RETURN g.id, g.title;", "--visualize"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "=== ZPARQL Graph Path Visualization ===")
	assert.Contains(t, outBuf.String(), "GOAL-C138-001")
}

func TestCycle138_RunQuery_ContextCancellation(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	idx := traversal.NewGraphIndex()
	err := PopulateIndexFromStorage(context.Background(), proc, idx, "goal")
	require.NoError(t, err)

	ast, err := traversal.ParseZPARQL("MATCH (g:goal) WHERE g.status == 'active' RETURN g.id;")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	executor := traversal.NewQueryExecutor(idx)
	res, err := executor.Execute(ctx, ast)
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCycle138_REPL_InteractiveInvocation(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// Via repl argument
	cmd := NewQueryCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetIn(bytes.NewBufferString(".exit\n"))
	cmd.SetArgs([]string{"repl"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "ZPARQL Interactive Graph Query Console")
	assert.Contains(t, outBuf.String(), "Exiting ZPARQL console. Goodbye!")

	// Via --interactive and --visualize
	cmd = NewQueryCmd()
	outBuf.Reset()
	cmd.SetOut(&outBuf)
	cmd.SetIn(bytes.NewBufferString(".exit\n"))
	cmd.SetArgs([]string{"--interactive", "--visualize"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "ZPARQL Interactive Graph Query Console")
}

func TestCycle138_REPL_MetaCommandsLifecycle(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	script := strings.Join([]string{
		"", // empty line
		".help",
		".h",
		"?",
		".kinds",
		".tables",
		".relations",
		".edges",
		".schema",
		".schema goal",
		".schema non_existent_kind_cycle138",
		".visualize on",
		".visualize off",
		".visualize 1",
		".visualize 0",
		".visualize yes",
		".visualize no",
		".visualize",
		".format table",
		".format json",
		".format visual",
		".format invalid_format",
		".format",
		".clear",
		".refresh",
		".reload",
		".run",
		".unknown_meta_command",
		".quit",
	}, "\n") + "\n"

	cmd := NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString(script))
	cmd.SetArgs([]string{"repl"})
	err := cmd.Execute()
	require.NoError(t, err)

	outStr := outBuf.String()
	assert.Contains(t, outStr, "Available Meta-Commands:")
	assert.Contains(t, outStr, "Indexed Object Kinds:")
	assert.Contains(t, outStr, "Indexed Graph Relationships & Edges:")
	assert.Contains(t, outStr, "Available Schemas:")
	assert.Contains(t, outStr, "Schema Fields for \"goal\"")
	assert.Contains(t, outStr, "No schema fields found for kind")
	assert.Contains(t, outStr, "Graph path visualizer mode enabled.")
	assert.Contains(t, outStr, "Graph path visualizer mode disabled")
	assert.Contains(t, outStr, "Output format set to: table")
	assert.Contains(t, outStr, "Output format set to: json")
	assert.Contains(t, outStr, "Output format set to: visual")
	assert.Contains(t, outStr, "Query buffer cleared.")
	assert.Contains(t, outStr, "Refreshing knowledge kernel graph from storage...")

	errStr := errBuf.String()
	assert.Contains(t, errStr, "Unknown format. Supported: table, json, visual")
	assert.Contains(t, errStr, "Query buffer is empty.")
	assert.Contains(t, errStr, "Unknown meta-command \".unknown_meta_command\"")
}

func TestCycle138_REPL_QueryExecution(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	script := strings.Join([]string{
		"MATCH (g:goal) RETURN g.id, g.title;",
		"MATCH (g:goal)",
		"WHERE g.status == 'active'",
		"RETURN g.id;",
		"MATCH (INVALID SYNTAX !!!;",
		"MATCH (g:goal) RETURN g.id",
		".run",
		".format json",
		"MATCH (g:goal) RETURN g.id;",
		".format visual",
		"MATCH (g:goal) RETURN g.id;",
		".q",
	}, "\n") + "\n"

	cmd := NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString(script))
	cmd.SetArgs([]string{"repl"})
	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, outBuf.String(), "GOAL-C138-001")
	assert.Contains(t, errBuf.String(), "ZPARQL Syntax Error:")
}

func TestCycle138_REPL_ExecutionContextCancellation(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := NewQueryCmd()
	cmd.SetContext(ctx)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(bytes.NewBufferString("MATCH (g:goal) RETURN g.id;\n.exit\n"))

	err := RunInteractiveREPL(cmd, proc, false)
	require.NoError(t, err)
	assert.Contains(t, errBuf.String(), "❌ Execution Error:")
}

func TestCycle138_REPL_ScannerError(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetIn(&cycle138ErrorReader{})

	err := RunInteractiveREPL(cmd, proc, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading interactive input")
}

func TestCycle138_Visualizer_FormatPatternsAndEdgeDirections(t *testing.T) {
	// Nil and empty result
	assert.Contains(t, FormatVisualizer(nil, nil), "No graph matches found")
	assert.Contains(t, FormatVisualizer(&traversal.QueryResult{Rows: nil}, nil), "No graph matches found")

	ast := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "a", Kind: "goal"},
					{Variable: "b", Kind: "priority_plan"},
					{Variable: "c", Kind: ""},
					{Variable: "d", Kind: "criteria"},
				},
				Edges: []traversal.EdgePattern{
					{EdgeType: "plan", Direction: traversal.DirectionOutgoing},
					{EdgeType: "", Direction: traversal.DirectionIncoming},
					{EdgeType: "crit", Direction: traversal.DirectionUndirected},
				},
			},
		},
	}

	result := &traversal.QueryResult{
		Headers: []string{"a.id", "b.title", "c.id", "d.id", "extra_prop"},
		Rows: []map[string]any{
			{
				"a.id":       "GOAL-1",
				"b.title":    "Plan Title",
				"c.id":       "NODE-3",
				"d.id":       "CRIT-4",
				"extra_prop": "val",
			},
		},
	}

	rendered := FormatVisualizer(result, ast)
	assert.Contains(t, rendered, "=== ZPARQL Graph Path Visualization ===")
	assert.Contains(t, rendered, "[1] Path:")
	assert.Contains(t, rendered, "───[:plan]──►")
	assert.Contains(t, rendered, "◄──[:related]──")
	assert.Contains(t, rendered, "───[:crit]───")
	assert.Contains(t, rendered, "(c:node NODE-3)")
	assert.Contains(t, rendered, "Properties: ")
	assert.Contains(t, rendered, "extra_prop=val")
	assert.Contains(t, rendered, "(Visualized 1 path(s))")
}

func TestCycle138_Visualizer_NodeListAndFindValue(t *testing.T) {
	// Pattern with no edges -> triggers formatNodeListResults
	ast := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "g", Kind: "goal"},
				},
				Edges: nil,
			},
		},
	}

	result := &traversal.QueryResult{
		Headers: []string{"g.id", "g.status"},
		Rows: []map[string]any{
			{"g.id": "GOAL-001", "g.status": "active"},
		},
	}

	rendered := FormatVisualizer(result, ast)
	assert.Contains(t, rendered, "● [GOAL-001]")
	assert.Contains(t, rendered, "g.id: GOAL-001")
	assert.Contains(t, rendered, "g.status: active")

	// Primary ID fallback to 'id' header and first header
	resFallbackID := &traversal.QueryResult{
		Headers: []string{"id", "title"},
		Rows: []map[string]any{
			{"id": "GEN-002", "title": "Generic Node"},
		},
	}
	assert.Contains(t, FormatVisualizer(resFallbackID, nil), "● [GEN-002]")

	resFirstHeader := &traversal.QueryResult{
		Headers: []string{"name", "value"},
		Rows: []map[string]any{
			{"name": "Alpha", "value": "Beta"},
		},
	}
	assert.Contains(t, FormatVisualizer(resFirstHeader, nil), "● [Alpha]")

	// findNodeValue direct tests
	assert.Equal(t, "", findNodeValue(nil, ""))
	assert.Equal(t, "GOAL-1", findNodeValue(map[string]any{"g.id": "GOAL-1"}, "g"))
	assert.Equal(t, "T-1", findNodeValue(map[string]any{"g.title": "T-1"}, "g"))
	assert.Equal(t, "Custom", findNodeValue(map[string]any{"g.custom": "Custom"}, "g"))
	assert.Equal(t, "x", findNodeValue(map[string]any{"other": "val"}, "x"))
}

func TestCycle138_Storage_PopulateIndexAndAddEdges(t *testing.T) {
	tmpDir, proc := setupCycle138TestKernel(t)
	require.NotEmpty(t, tmpDir)
	require.NotNil(t, proc)

	// Populate with explicit kinds
	idx := traversal.NewGraphIndex()
	err := PopulateIndexFromStorage(context.Background(), proc, idx, "goal", "priority_plan")
	require.NoError(t, err)
	assert.True(t, len(idx.GetAllNodeIDs()) > 0)

	// Populate with all kinds
	idxAll := traversal.NewGraphIndex()
	err = PopulateIndexFromStorage(context.Background(), proc, idxAll)
	require.NoError(t, err)
	assert.True(t, len(idxAll.GetAllNodeIDs()) >= len(idx.GetAllNodeIDs()))

	// addEdges with various types
	edgeIdx := traversal.NewGraphIndex()
	edgeIdx.AddNode("source", "kind", map[string]any{"id": "source"})
	edgeIdx.AddNode("target1", "kind", map[string]any{"id": "target1"})
	edgeIdx.AddNode("target2", "kind", map[string]any{"id": "target2"})
	edgeIdx.AddNode("target3", "kind", map[string]any{"id": "target3"})

	addEdges(edgeIdx, "source", "rel_str", "target1")
	addEdges(edgeIdx, "source", "rel_str_empty", "")
	addEdges(edgeIdx, "source", "rel_slice", []string{"target2", ""})
	addEdges(edgeIdx, "source", "rel_any", []any{"target3", "", 12345})
	addEdges(edgeIdx, "source", "rel_other", 999)

	assert.Len(t, edgeIdx.GetOutEdges("source", "rel_str"), 1)
	assert.Len(t, edgeIdx.GetOutEdges("source", "rel_str_empty"), 0)
	assert.Len(t, edgeIdx.GetOutEdges("source", "rel_slice"), 1)
	assert.Len(t, edgeIdx.GetOutEdges("source", "rel_any"), 1)
	assert.Len(t, edgeIdx.GetOutEdges("source", "rel_other"), 0)
}

func TestCycle138_Storage_ExtractTargetKinds(t *testing.T) {
	// Nil AST
	assert.Nil(t, extractTargetKinds(nil))

	// Empty patterns
	assert.Nil(t, extractTargetKinds(&traversal.QueryAST{}))

	// Wildcard pattern (empty kind) -> returns nil to scan all
	astWildcard := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "n", Kind: ""},
				},
			},
		},
	}
	assert.Nil(t, extractTargetKinds(astWildcard))

	// Explicit kinds
	astExplicit := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "g", Kind: "goal"},
					{Variable: "p", Kind: "priority_plan"},
					{Variable: "g2", Kind: "goal"},
				},
			},
		},
	}
	kinds := extractTargetKinds(astExplicit)
	assert.Len(t, kinds, 2)
	assert.Contains(t, kinds, "goal")
	assert.Contains(t, kinds, "priority_plan")
}

func TestCycle138_PrintHelpersDirect(t *testing.T) {
	var buf bytes.Buffer

	printREPLBanner(&buf)
	assert.Contains(t, buf.String(), "ZPARQL Interactive Graph Query Console")

	buf.Reset()
	printREPLHelp(&buf)
	assert.Contains(t, buf.String(), "Available Meta-Commands:")

	buf.Reset()
	printIndexedKinds(&buf, nil)
	assert.Contains(t, buf.String(), "Graph index uninitialized.")

	idx := traversal.NewGraphIndex()
	idx.AddNode("N1", "goal", map[string]any{"id": "N1", "kind": "goal"})
	idx.AddNode("N2", "goal", map[string]any{"id": "N2", "kind": "goal"})
	idx.AddNode("N3", "", map[string]any{"id": "N3"})
	buf.Reset()
	printIndexedKinds(&buf, idx)
	assert.Contains(t, buf.String(), "Indexed Object Kinds:")
	assert.Contains(t, buf.String(), "goal")

	buf.Reset()
	printIndexedRelations(&buf, nil)
	assert.Contains(t, buf.String(), "Graph index uninitialized.")

	buf.Reset()
	printIndexedRelations(&buf, idx)
	assert.Contains(t, buf.String(), "Indexed Graph Relationships & Edges:")

	buf.Reset()
	printSchemaInfo(&buf, "")
	assert.Contains(t, buf.String(), "Available Schemas:")

	buf.Reset()
	printSchemaInfo(&buf, "goal")
	assert.Contains(t, buf.String(), "Schema Fields for \"goal\"")

	buf.Reset()
	printSchemaInfo(&buf, "non_existent_kind_cycle138")
	assert.Contains(t, buf.String(), "No schema fields found for kind")
}

func TestCycle138_RenderQueryResultDirect(t *testing.T) {
	var buf bytes.Buffer
	res := &traversal.QueryResult{
		Headers: []string{"id", "title"},
		Rows: []map[string]any{
			{"id": "GOAL-1", "title": "Elevate Tests"},
		},
	}
	ast := &traversal.QueryAST{}

	// JSON format
	buf.Reset()
	renderQueryResult(&buf, res, ast, "json", 10*time.Millisecond)
	assert.Contains(t, buf.String(), `"id": "GOAL-1"`)

	// Visual format
	buf.Reset()
	renderQueryResult(&buf, res, ast, "visual", 10*time.Millisecond)
	assert.Contains(t, buf.String(), "=== ZPARQL Graph Path Visualization ===")
	assert.Contains(t, buf.String(), "Query executed in")

	// Table format with 0 rows
	buf.Reset()
	emptyRes := &traversal.QueryResult{Headers: []string{"id"}, Rows: nil}
	renderQueryResult(&buf, emptyRes, ast, "table", 5*time.Millisecond)
	assert.Contains(t, buf.String(), "No matches found.")

	// Table format with rows
	buf.Reset()
	renderQueryResult(&buf, res, ast, "table", 8*time.Millisecond)
	assert.Contains(t, buf.String(), "id\t| title")
	assert.Contains(t, buf.String(), "GOAL-1\t| Elevate Tests")
	assert.Contains(t, buf.String(), "(1 rows in")
}
