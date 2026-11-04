package query

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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

type cycle100ErrReader struct{}

func (e *cycle100ErrReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated stdin failure")
}

type cycle100PartialThenErrReader struct {
	lines []string
	idx   int
}

func (r *cycle100PartialThenErrReader) Read(p []byte) (n int, err error) {
	if r.idx < len(r.lines) {
		s := r.lines[r.idx]
		r.idx++
		copy(p, s)
		return len(s), nil
	}
	return 0, errors.New("stream error reading line")
}

func setupCycle100TestKernel(t *testing.T) (string, *cli.Processor) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	goalDir := filepath.Join(tmpDir, ".zqk", "process", "goals")
	planDir := filepath.Join(tmpDir, ".zqk", "process", "priority_plans")
	bliDir := filepath.Join(tmpDir, ".zqk", "process", "backlog_items")
	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(planDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(bliDir, paths.DirPerm755))

	goalData := map[string]any{
		objects.FieldKeyID:    "GOAL-CYCLE100-001",
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Elevate Query Subsystem",
		"status":              "active",
		"depends_on":          "GOAL-ROOT-001",
		"priority_plan_ref":   "PRI-CYCLE100-001",
	}
	rawGoal, err := yaml.Marshal(goalData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "GOAL-CYCLE100-001.yaml"), rawGoal, paths.FilePerm644))

	planData := map[string]any{
		objects.FieldKeyID:    "PRI-CYCLE100-001",
		objects.FieldKeyKind:  "priority_plan",
		objects.FieldKeyTitle: "Query Subsystem Sprint",
		"status":              "in_progress",
		"items":               []string{"BLI-CYCLE100-001", "BLI-CYCLE100-002"},
	}
	rawPlan, err := yaml.Marshal(planData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(planDir, "PRI-CYCLE100-001.yaml"), rawPlan, paths.FilePerm644))

	bli1Data := map[string]any{
		objects.FieldKeyID:    "BLI-CYCLE100-001",
		objects.FieldKeyKind:  "backlog_item",
		objects.FieldKeyTitle: "Craft Elevation Tests",
		"status":              "completed",
		"priority_plan_refs":  []string{"PRI-CYCLE100-001"},
	}
	rawBli1, err := yaml.Marshal(bli1Data)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "BLI-CYCLE100-001.yaml"), rawBli1, paths.FilePerm644))

	return tmpDir, proc
}

func TestCycle100_RunQuery_ExecutionWithMatchedRows(t *testing.T) {
	_, _ = setupCycle100TestKernel(t)

	// 1. Table format with matches (exercises table row printing and row count)
	cmdTable := NewQueryCmd()
	var bufTable bytes.Buffer
	cmdTable.SetOut(&bufTable)
	cmdTable.SetArgs([]string{"MATCH (g:goal) RETURN g.id, g.title, g.status;", "--format", "table"})
	err := cmdTable.Execute()
	require.NoError(t, err)
	outTable := bufTable.String()
	assert.Contains(t, outTable, "g.id\t| g.title\t| g.status")
	assert.Contains(t, outTable, "GOAL-CYCLE100-001")
	assert.Contains(t, outTable, "Elevate Query Subsystem")
	assert.Contains(t, outTable, "(1 rows)")

	// 2. JSON format with matches
	cmdJSON := NewQueryCmd()
	var bufJSON bytes.Buffer
	cmdJSON.SetOut(&bufJSON)
	cmdJSON.SetArgs([]string{"MATCH (g:goal) RETURN g.id, g.title;", "--format", "json"})
	err = cmdJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"headers"`)
	assert.Contains(t, bufJSON.String(), "GOAL-CYCLE100-001")

	// 3. Visual format with matches
	cmdVisual := NewQueryCmd()
	var bufVisual bytes.Buffer
	cmdVisual.SetOut(&bufVisual)
	cmdVisual.SetArgs([]string{"MATCH (g:goal) RETURN g.id, g.title;", "--format", "visual"})
	err = cmdVisual.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufVisual.String(), "ZPARQL Graph Path Visualization")
	assert.Contains(t, bufVisual.String(), "GOAL-CYCLE100-001")

	// 4. --visualize flag with matches
	cmdVisualFlag := NewQueryCmd()
	var bufVisualFlag bytes.Buffer
	cmdVisualFlag.SetOut(&bufVisualFlag)
	cmdVisualFlag.SetArgs([]string{"MATCH (g:goal) RETURN g.id, g.title;", "--visualize"})
	err = cmdVisualFlag.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufVisualFlag.String(), "ZPARQL Graph Path Visualization")
}

func TestCycle100_RunQuery_StdinReadErrors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// Stdin error via args[0] == "-"
	cmdArgStdin := NewQueryCmd()
	cmdArgStdin.SetIn(&cycle100ErrReader{})
	cmdArgStdin.SetArgs([]string{"-"})
	err := cmdArgStdin.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query from stdin")

	// Stdin error via flag -f "-"
	cmdFlagStdin := NewQueryCmd()
	cmdFlagStdin.SetIn(&cycle100ErrReader{})
	cmdFlagStdin.SetArgs([]string{"-f", "-"})
	err = cmdFlagStdin.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query from stdin")
}

func TestCycle100_ExtractTargetKinds_ComplexAST(t *testing.T) {
	// Deduplication across multiple patterns
	ast := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "g", Kind: "goal"},
					{Variable: "p", Kind: "priority_plan"},
				},
			},
			{
				Nodes: []traversal.NodePattern{
					{Variable: "g2", Kind: "goal"},
					{Variable: "b", Kind: "backlog_item"},
				},
			},
		},
	}
	kinds := extractTargetKinds(ast)
	assert.Len(t, kinds, 3)
	assert.Contains(t, kinds, "goal")
	assert.Contains(t, kinds, "priority_plan")
	assert.Contains(t, kinds, "backlog_item")

	// Pattern with an empty kind triggers wildcard scan (returns nil)
	astWildcard := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "x", Kind: "goal"},
					{Variable: "w", Kind: ""},
				},
			},
		},
	}
	assert.Nil(t, extractTargetKinds(astWildcard))
}

func TestCycle100_PopulateIndexFromStorage_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	goalDir := filepath.Join(tmpDir, ".zqk", "process", "goals")
	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))

	// Write item missing an ID (should be skipped)
	invalidData := map[string]any{
		objects.FieldKeyKind: "goal",
		"title":              "Missing ID",
	}
	rawInvalid, err := yaml.Marshal(invalidData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "NO-ID.yaml"), rawInvalid, paths.FilePerm644))

	// Write item with various edge definitions
	richData := map[string]any{
		objects.FieldKeyID:   "GOAL-EDGE-001",
		objects.FieldKeyKind: "goal",
		"depends_on":         []string{"DEP-1", "DEP-2"},
		"requirement_refs":   []any{"REQ-1", "REQ-2"},
		"parent_ref":         "PARENT-1",
		"custom_property":    "ignored_for_edges",
	}
	rawRich, err := yaml.Marshal(richData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "GOAL-EDGE-001.yaml"), rawRich, paths.FilePerm644))

	idx := traversal.NewGraphIndex()
	// Target explicit kind
	err = PopulateIndexFromStorage(context.Background(), proc, idx, "goal")
	require.NoError(t, err)

	// NO-ID should not be indexed
	_, okEmpty := idx.GetNode("")
	assert.False(t, okEmpty)

	// GOAL-EDGE-001 should be indexed with edges
	_, okNode := idx.GetNode("GOAL-EDGE-001")
	assert.True(t, okNode)
	assert.ElementsMatch(t, []string{"DEP-1", "DEP-2"}, idx.GetOutEdges("GOAL-EDGE-001", "depends_on"))
	assert.ElementsMatch(t, []string{"REQ-1", "REQ-2"}, idx.GetOutEdges("GOAL-EDGE-001", "requirement_refs"))
	assert.ElementsMatch(t, []string{"REQ-1", "REQ-2"}, idx.GetOutEdges("GOAL-EDGE-001", "requirement"))
	assert.Equal(t, []string{"PARENT-1"}, idx.GetOutEdges("GOAL-EDGE-001", "parent_ref"))
	assert.Equal(t, []string{"PARENT-1"}, idx.GetOutEdges("GOAL-EDGE-001", "parent"))
}

func TestCycle100_AddEdges_Exhaustive(t *testing.T) {
	idx := traversal.NewGraphIndex()
	idx.AddNode("nodeA", "kindA", map[string]any{"id": "nodeA"})

	// String with content
	addEdges(idx, "nodeA", "rel1", "targetA")
	assert.Equal(t, []string{"targetA"}, idx.GetOutEdges("nodeA", "rel1"))

	// Empty string
	addEdges(idx, "nodeA", "relEmpty", "")
	assert.Empty(t, idx.GetOutEdges("nodeA", "relEmpty"))

	// []string with empty target included
	addEdges(idx, "nodeA", "relSlice", []string{"t1", "", "t2"})
	assert.Equal(t, []string{"t1", "t2"}, idx.GetOutEdges("nodeA", "relSlice"))

	// []any with various types including empty strings and ints
	addEdges(idx, "nodeA", "relAny", []any{"a1", 999, "", "a2", nil})
	assert.Equal(t, []string{"a1", "a2"}, idx.GetOutEdges("nodeA", "relAny"))

	// Unsupported type (int, map) should be ignored without panicking
	addEdges(idx, "nodeA", "relInt", 12345)
	assert.Empty(t, idx.GetOutEdges("nodeA", "relInt"))
}

func TestCycle100_Visualizer_UncoveredBranches(t *testing.T) {
	// Pattern with 0 nodes
	astEmptyNodes := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{},
				Edges: []traversal.EdgePattern{
					{EdgeType: "items"},
				},
			},
		},
	}
	resRows := &traversal.QueryResult{
		Headers: []string{"col1"},
		Rows: []map[string]any{
			{"col1": "val1"},
		},
	}
	outEmptyNodes := FormatVisualizer(resRows, astEmptyNodes)
	assert.Contains(t, outEmptyNodes, "=== ZPARQL Graph Path Visualization ===")

	// Pattern with node missing kind (defaults to "node") and edge missing EdgeType (defaults to "related")
	astDefaults := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "n1", Kind: ""},
					{Variable: "n2", Kind: ""},
				},
				Edges: []traversal.EdgePattern{
					{EdgeType: "", Direction: traversal.DirectionOutgoing},
				},
			},
		},
	}
	resDefaults := &traversal.QueryResult{
		Headers: []string{"n1.id", "n2.id", "extra_nil", "extra_val"},
		Rows: []map[string]any{
			{
				"n1.id":     "NODE-1",
				"n2.id":     "NODE-2",
				"extra_nil": nil,
				"extra_val": "displayed_property",
			},
		},
	}
	outDefaults := FormatVisualizer(resDefaults, astDefaults)
	assert.Contains(t, outDefaults, "(n1:node NODE-1)")
	assert.Contains(t, outDefaults, "───[:related]──► (n2:node NODE-2)")
	assert.Contains(t, outDefaults, "Properties: n1.id=NODE-1, n2.id=NODE-2, extra_val=displayed_property")

	// Node list results when ID header is capitalized or primary ID from other headers
	resCapID := &traversal.QueryResult{
		Headers: []string{"ID", "label"},
		Rows: []map[string]any{
			{
				"ID":    "CAP-ID-123",
				"label": "Item Label",
			},
		},
	}
	outCapID := FormatVisualizer(resCapID, nil)
	assert.Contains(t, outCapID, "● [CAP-ID-123]")
	assert.Contains(t, outCapID, "├─ label: Item Label")
}

func TestCycle100_REPL_PrintKindsAndRelations_NilAndEdgeCases(t *testing.T) {
	var buf bytes.Buffer

	// 1. printIndexedKinds with nil index
	printIndexedKinds(&buf, nil)
	assert.Contains(t, buf.String(), "Graph index uninitialized.")

	// 2. printIndexedKinds with nodes without kind
	buf.Reset()
	idxNoKind := traversal.NewGraphIndex()
	idxNoKind.AddNode("node1", "", map[string]any{"id": "node1"})
	printIndexedKinds(&buf, idxNoKind)
	assert.Contains(t, buf.String(), "Indexed Object Kinds:")
	assert.Contains(t, buf.String(), "0 unique kinds")

	// 2b. printIndexedKinds with populated kinds
	buf.Reset()
	idxWithKinds := traversal.NewGraphIndex()
	idxWithKinds.AddNode("g1", "goal", map[string]any{"id": "g1", "kind": "goal"})
	idxWithKinds.AddNode("g2", "goal", map[string]any{"id": "g2", "kind": "goal"})
	idxWithKinds.AddNode("b1", "backlog_item", map[string]any{"id": "b1", "kind": "backlog_item"})
	printIndexedKinds(&buf, idxWithKinds)
	assert.Contains(t, buf.String(), "goal")
	assert.Contains(t, buf.String(), "backlog_item")
	assert.Contains(t, buf.String(), "2 unique kinds, 3 total nodes")

	// 3. printIndexedRelations with nil index
	buf.Reset()
	printIndexedRelations(&buf, nil)
	assert.Contains(t, buf.String(), "Graph index uninitialized.")

	// 4. printIndexedRelations with initialized index
	buf.Reset()
	idxValid := traversal.NewGraphIndex()
	printIndexedRelations(&buf, idxValid)
	assert.Contains(t, buf.String(), "Indexed Graph Relationships & Edges:")
	assert.Contains(t, buf.String(), "Common Traversal Edges")
}

func TestCycle100_REPL_ScannerError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	// Stream that fails during scan
	failingReader := &cycle100PartialThenErrReader{
		lines: []string{},
	}
	cmd.SetIn(failingReader)
	cmd.SetArgs([]string{"--interactive"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading interactive input")
}

func TestCycle100_REPL_ExecutionWithMatchedRowsAndDurations(t *testing.T) {
	_, proc := setupCycle100TestKernel(t)

	// Run interactive REPL with query that matches items and outputs in visual & table format
	input := strings.Join([]string{
		"MATCH (g:goal) RETURN g.id, g.title;",
		".visualize on",
		"MATCH (g:goal) RETURN g.id, g.title;",
		".format json",
		"MATCH (g:goal) RETURN g.id, g.title;",
		".exit",
	}, "\n") + "\n"

	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs([]string{"--interactive"})

	err := RunInteractiveREPL(cmd, proc, false)
	require.NoError(t, err)

	outStr := outBuf.String()
	assert.Contains(t, outStr, "GOAL-CYCLE100-001")
	assert.Contains(t, outStr, "Elevate Query Subsystem")
	assert.Contains(t, outStr, "(1 rows in")
	assert.Contains(t, outStr, "ZPARQL Graph Path Visualization")
	assert.Contains(t, outStr, "Query executed in")
	assert.Contains(t, outStr, `"headers"`)
}
