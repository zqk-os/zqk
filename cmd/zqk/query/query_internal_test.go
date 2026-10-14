package query

import (
	"bytes"
	"context"
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

func TestAddEdges_AllTypes(t *testing.T) {
	idx := traversal.NewGraphIndex()
	idx.AddNode("source1", "goal", map[string]any{"id": "source1"})

	// string target
	addEdges(idx, "source1", "rel_str", "target1")
	// empty string target
	addEdges(idx, "source1", "rel_empty", "")

	// []string targets
	addEdges(idx, "source1", "rel_slice", []string{"target2", "", "target3"})

	// []any targets
	addEdges(idx, "source1", "rel_any", []any{"target4", 123, "", "target5"})

	assert.Equal(t, []string{"target1"}, idx.GetOutEdges("source1", "rel_str"))
	assert.Equal(t, []string{"target2", "target3"}, idx.GetOutEdges("source1", "rel_slice"))
	assert.Equal(t, []string{"target4", "target5"}, idx.GetOutEdges("source1", "rel_any"))
}

func TestExtractTargetKinds(t *testing.T) {
	// nil AST
	assert.Nil(t, extractTargetKinds(nil))

	// Empty patterns
	assert.Nil(t, extractTargetKinds(&traversal.QueryAST{}))

	// Wildcard node (empty kind) returns nil
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
					{Variable: "b", Kind: "backlog_item"},
					{Variable: "g", Kind: "goal"},
				},
			},
		},
	}
	kinds := extractTargetKinds(astExplicit)
	assert.Len(t, kinds, 2)
}

func TestVisualizer_FormattingBranches(t *testing.T) {
	// 1. Nil or empty results
	resEmpty := FormatVisualizer(nil, nil)
	assert.Contains(t, resEmpty, "No graph matches found")

	resEmpty2 := FormatVisualizer(&traversal.QueryResult{}, nil)
	assert.Contains(t, resEmpty2, "No graph matches found")

	// 2. Node list results without edges
	resNodes := &traversal.QueryResult{
		Headers: []string{"b.id", "b.title", "b.status"},
		Rows: []map[string]any{
			{
				"b.id":     "BLI-101",
				"b.title":  "Test Backlog Item",
				"b.status": "active",
			},
		},
	}
	outNodes := FormatVisualizer(resNodes, nil)
	assert.Contains(t, outNodes, "BLI-101")
	assert.Contains(t, outNodes, "Test Backlog Item")

	// 3. Node list with fallback header
	resFallback := &traversal.QueryResult{
		Headers: []string{"custom_field"},
		Rows: []map[string]any{
			{"custom_field": "val123"},
		},
	}
	outFallback := FormatVisualizer(resFallback, nil)
	assert.Contains(t, outFallback, "val123")

	// 4. Pattern results with outgoing, incoming, and undirected edges
	astEdges := &traversal.QueryAST{
		Patterns: []traversal.PathPattern{
			{
				Nodes: []traversal.NodePattern{
					{Variable: "p", Kind: "priority_plan"},
					{Variable: "b", Kind: "backlog_item"},
					{Variable: "c", Kind: "criteria"},
				},
				Edges: []traversal.EdgePattern{
					{
						EdgeType:  "items",
						Direction: traversal.DirectionOutgoing,
					},
					{
						EdgeType:  "criteria_refs",
						Direction: traversal.DirectionIncoming,
					},
				},
			},
			{
				Nodes: []traversal.NodePattern{
					{Variable: "x", Kind: "item"},
					{Variable: "y", Kind: ""},
				},
				Edges: []traversal.EdgePattern{
					{
						EdgeType:  "",
						Direction: traversal.DirectionUndirected,
					},
				},
			},
		},
	}

	resEdges := &traversal.QueryResult{
		Headers: []string{"p.id", "b.id", "extra_prop"},
		Rows: []map[string]any{
			{
				"p.id":       "PRI-001",
				"b.id":       "BLI-001",
				"extra_prop": "some_value",
			},
		},
	}

	outEdges := FormatVisualizer(resEdges, astEdges)
	assert.Contains(t, outEdges, "PRI-001")
	assert.Contains(t, outEdges, "BLI-001")
	assert.Contains(t, outEdges, "items")
	assert.Contains(t, outEdges, "extra_prop=some_value")

	// 5. Test findNodeValue edge cases
	assert.Equal(t, "", findNodeValue(map[string]any{}, ""))
	assert.Equal(t, "fallback_var", findNodeValue(map[string]any{}, "fallback_var"))
	assert.Equal(t, "my_title", findNodeValue(map[string]any{"g.title": "my_title"}, "g"))
	assert.Equal(t, "pref_val", findNodeValue(map[string]any{"x.prefix_val": "pref_val"}, "x"))
}

func TestRunQuery_ErrorsAndOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. Missing query argument
	cmdNoArg := NewQueryCmd()
	var bufErr bytes.Buffer
	cmdNoArg.SetErr(&bufErr)
	cmdNoArg.SetOut(&bufErr)
	cmdNoArg.SetArgs([]string{})
	err := cmdNoArg.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query string required")

	// 2. Syntax error in query
	cmdSyntax := NewQueryCmd()
	cmdSyntax.SetArgs([]string{"INVALID SYNTAX QUERY !!"})
	err = cmdSyntax.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ZPARQL query syntax error")

	// 3. Read query from stdin via argument "-"
	cmdStdinArg := NewQueryCmd()
	var bufOut bytes.Buffer
	cmdStdinArg.SetOut(&bufOut)
	cmdStdinArg.SetIn(strings.NewReader("MATCH (n) RETURN n.id;"))
	cmdStdinArg.SetArgs([]string{"-", "--format", "json"})
	err = cmdStdinArg.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), `"headers"`)

	// 4. Read query from stdin via flag "-f -"
	cmdStdinFlag := NewQueryCmd()
	bufOut.Reset()
	cmdStdinFlag.SetOut(&bufOut)
	cmdStdinFlag.SetIn(strings.NewReader("MATCH (n) RETURN n.id;"))
	cmdStdinFlag.SetArgs([]string{"-f", "-", "--format", "json"})
	err = cmdStdinFlag.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), `"headers"`)

	// 5. Read query from non-existent file
	cmdBadFile := NewQueryCmd()
	cmdBadFile.SetArgs([]string{"-f", filepath.Join(tmpDir, "non_existent.zparql")})
	err = cmdBadFile.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read query file")

	// 6. Format visual output
	cmdVisual := NewQueryCmd()
	bufOut.Reset()
	cmdVisual.SetOut(&bufOut)
	cmdVisual.SetArgs([]string{"MATCH (n) RETURN n.id;", "--format", "visual"})
	err = cmdVisual.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), "ZPARQL Graph Path Visualization")
}

func TestREPL_MetaCommandsAndExecution(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	input := strings.Join([]string{
		".h",
		".kinds",
		".tables",
		".edges",
		".relations",
		".schema",
		".schema nonexistent_kind_xyz",
		".schema goal",
		".format",
		".format table",
		".format json",
		".format visual",
		".format invalid_fmt",
		".visualize on",
		".visualize off",
		".visualize toggle",
		".visualize",
		".refresh",
		".reload",
		".unknown_meta_cmd",
		".run", // empty buffer
		"MATCH (n) RETURN n.id",
		".run", // executes buffer without semicolon
		"INVALID SYNTAX ;",
		".clear",
		".q",
	}, "\n") + "\n"

	cmd := NewQueryCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs([]string{"-i"})

	err := cmd.Execute()
	require.NoError(t, err)

	outStr := outBuf.String()
	errStr := errBuf.String()

	assert.Contains(t, outStr, "Available Meta-Commands:")
	assert.Contains(t, outStr, "Indexed Object Kinds:")
	assert.Contains(t, outStr, "Indexed Graph Relationships & Edges:")
	assert.Contains(t, outStr, "Available Schemas")
	assert.Contains(t, outStr, "No schema fields found for kind")
	assert.Contains(t, outStr, "Schema Fields for \"goal\"")
	assert.Contains(t, outStr, "Current format:")
	assert.Contains(t, outStr, "Output format set to: table")
	assert.Contains(t, outStr, "Output format set to: json")
	assert.Contains(t, outStr, "Output format set to: visual")
	assert.Contains(t, outStr, "Graph path visualizer mode enabled.")
	assert.Contains(t, outStr, "Graph path visualizer mode disabled")
	assert.Contains(t, outStr, "Re-indexed")
	assert.Contains(t, outStr, "Exiting ZPARQL console. Goodbye!")

	assert.Contains(t, errStr, "Unknown format. Supported: table, json, visual")
	assert.Contains(t, errStr, "Unknown meta-command \".unknown_meta_cmd\"")
	assert.Contains(t, errStr, "Query buffer is empty.")
	assert.Contains(t, errStr, "ZPARQL Syntax Error")
}

func TestRenderQueryResult_Branches(t *testing.T) {
	var buf bytes.Buffer

	// 1. JSON
	res := &traversal.QueryResult{
		Headers: []string{"id", "title"},
		Rows: []map[string]any{
			{"id": "G-1", "title": "Goal 1"},
			{"id": "G-2", "title": "Goal 2"},
		},
	}
	renderQueryResult(&buf, res, nil, "json", 10*time.Millisecond)
	assert.Contains(t, buf.String(), `"id": "G-1"`)

	// 2. Visual
	buf.Reset()
	renderQueryResult(&buf, res, nil, "visual", 10*time.Millisecond)
	assert.Contains(t, buf.String(), "ZPARQL Graph Path Visualization")

	// 3. Tabular empty
	buf.Reset()
	renderQueryResult(&buf, &traversal.QueryResult{}, nil, "table", 10*time.Millisecond)
	assert.Contains(t, buf.String(), "No matches found.")

	// 4. Tabular with rows
	buf.Reset()
	renderQueryResult(&buf, res, nil, "table", 10*time.Millisecond)
	assert.Contains(t, buf.String(), "G-1")
	assert.Contains(t, buf.String(), "Goal 2")
	assert.Contains(t, buf.String(), "(2 rows in")
}

func TestPopulateIndexFromStorage_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// Create test objects in storage
	cmd := NewQueryCmd()
	cmd.SetContext(context.Background())
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// Write mock object files to storage
	goalDir := filepath.Join(tmpDir, ".zqk", "process", "goals")
	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))

	goalData := map[string]any{
		objects.FieldKeyID:    "GOAL-TEST-001",
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Test Goal",
		"depends_on":          "GOAL-PARENT-001",
		"criteria_refs":       []string{"CRIT-001", "CRIT-002"},
		"priority_plan_ref":   "PRI-001",
		"single_ref":          "SREF-001",
		"multi_refs":          []any{"MREF-1", "MREF-2"},
	}
	raw, err := yaml.Marshal(goalData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "GOAL-TEST-001.yaml"), raw, paths.FilePerm644))

	idx := traversal.NewGraphIndex()
	err = PopulateIndexFromStorage(context.Background(), proc, idx)
	require.NoError(t, err)

	// Verify node and edges indexed
	node, ok := idx.GetNode("GOAL-TEST-001")
	assert.True(t, ok)
	assert.NotNil(t, node)
	assert.Equal(t, []string{"GOAL-PARENT-001"}, idx.GetOutEdges("GOAL-TEST-001", "depends_on"))
}
