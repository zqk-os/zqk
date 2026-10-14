package graph

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func setupGraphIsolatedProject(t *testing.T) string {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.graph.unit",
	})
	return proj.Root
}

func executeGraphCmd(t *testing.T, cmd *cobra.Command, projectRoot string, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewGraphCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := NewGraphCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "graph", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["query"], "graph command must include 'query'")
	assert.True(t, subNames["discover"], "graph command must include 'discover'")
	assert.True(t, subNames["export"], "graph command must include 'export'")
}

func TestGraphQueryCmd_ValidationAndExecution(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	// 1. Missing query string should fail
	cmd := NewQueryCmd()
	_, err := executeGraphCmd(t, cmd, testRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query string required")

	// 2. Query against standard file backend should report error
	cmd2 := NewQueryCmd()
	_, err2 := executeGraphCmd(t, cmd2, testRoot, "MATCH (n:BacklogItem) RETURN n")
	if err2 != nil {
		assert.Contains(t, err2.Error(), "query execution failed")
	}
}

func TestGraphQuery_OutputQueryResults(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	// 1. Empty results
	buf := new(bytes.Buffer)
	cmd := NewQueryCmd()
	ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf)
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
	cmd.SetOut(buf)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	emptyResult := &storage.QueryResult{
		Objects: nil,
	}
	err = outputQueryResults(cmd, proc, emptyResult)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No results found.")

	// 2. Object results (human formatted)
	buf.Reset()
	objResult := &storage.QueryResult{
		Objects: []map[string]any{
			{
				objects.FieldKeyID:    "BLI-100",
				objects.FieldKeyKind:  "backlog_item",
				objects.FieldKeyTitle: "Implement Graph Visualizer",
			},
			{
				"custom_field": "custom_val",
			},
		},
	}
	err = outputQueryResults(cmd, proc, objResult)
	require.NoError(t, err)
	outStr := buf.String()
	assert.Contains(t, outStr, "[backlog_item] BLI-100: Implement Graph Visualizer")
	assert.Contains(t, outStr, "custom_field")

	// 3. JSON structured format
	buf.Reset()
	cmdJSON := NewQueryCmd()
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf))
	cli.SetContext(cmdJSON, cli.ContextForProjectRoot(testRoot))
	_ = cmdJSON.Flags().Set("format", "json")
	cmdJSON.SetOut(buf)

	procJSON, err := cli.NewProcessor(cmdJSON)
	require.NoError(t, err)
	err = outputQueryResults(cmdJSON, procJSON, objResult)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "BLI-100")
}

func TestGraphDiscover_TreeRenderingAndRecursion(t *testing.T) {
	t.Parallel()

	// Test renderTree directly
	var sb strings.Builder
	root := &discoverNode{
		id:    "GOAL-1",
		kind:  "goal",
		title: "Root Objective",
		children: []*discoverNode{
			{
				id:    "REQ-1",
				kind:  "requirement",
				title: "Core Feature Requirement",
				children: []*discoverNode{
					{
						id:    "BLI-1",
						kind:  "backlog_item",
						title: "Task Unit",
					},
				},
			},
			{
				id:    "REQ-2",
				kind:  "requirement",
				title: "Secondary Requirement",
			},
		},
	}

	renderTree(&sb, root, "", true, true)
	treeOutput := sb.String()

	assert.Contains(t, treeOutput, "📦 [goal] GOAL-1: Root Objective")
	assert.Contains(t, treeOutput, "[requirement] REQ-1: Core Feature Requirement")
	assert.Contains(t, treeOutput, "[backlog_item] BLI-1: Task Unit")
	assert.Contains(t, treeOutput, "[requirement] REQ-2: Secondary Requirement")
	assert.Contains(t, treeOutput, "└──")
	assert.Contains(t, treeOutput, "├──")
}

func TestGraphDiscoverCmd_Execution(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	// 1. Missing argument check
	cmd := NewDiscoverCmd()
	_, err := executeGraphCmd(t, cmd, testRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s)")

	// 2. Discover non-existent object
	cmd2 := NewDiscoverCmd()
	_, err2 := executeGraphCmd(t, cmd2, testRoot, "BLI-NON-EXISTENT")
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "failed to read root object")

	// 3. Create a valid object and discover it
	cmdCreate := NewDiscoverCmd()
	cmdCreate.SetContext(pkgctx.NewSystemContext())
	cli.SetContext(cmdCreate, cli.ContextForProjectRoot(testRoot))
	proc, err := cli.NewProcessor(cmdCreate)
	require.NoError(t, err)

	ctx := context.Background()
	secCtx := proc.SecurityContext()
	obj := map[string]any{
		objects.FieldKeyID:    "BLI-DISCOVER-001",
		objects.FieldKeyKind:  objects.KindBacklogItem,
		objects.FieldKeyTitle: "Discoverable Test Item",
	}
	err = proc.Storage().Create(ctx, secCtx, obj)
	require.NoError(t, err)

	cmdDiscover := NewDiscoverCmd()
	out, err := executeGraphCmd(t, cmdDiscover, testRoot, "BLI-DISCOVER-001", "--depth", "2")
	require.NoError(t, err)
	assert.Contains(t, out, "Relationship Discovery for:")
	assert.Contains(t, out, "BLI-DISCOVER-001")
	assert.Contains(t, out, "Discoverable Test Item")
}

func TestGraphExportCmd_FormatsAndFileExport(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	// Seed an object to export
	cmdProc := NewExportCmd()
	cmdProc.SetContext(pkgctx.NewSystemContext())
	cli.SetContext(cmdProc, cli.ContextForProjectRoot(testRoot))
	proc, err := cli.NewProcessor(cmdProc)
	require.NoError(t, err)

	ctx := context.Background()
	secCtx := proc.SecurityContext()
	obj := map[string]any{
		objects.FieldKeyID:    "GOAL-EXPORT-001",
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyTitle: "Exportable Goal Holon",
	}
	_ = proc.Storage().Create(ctx, secCtx, obj)

	// 1. Export as JSON-LD / databook to stdout
	cmd1 := NewExportCmd()
	out1, err1 := executeGraphCmd(t, cmd1, testRoot, "--kind", "goal", "--format", "json-ld")
	require.NoError(t, err1)
	assert.Contains(t, out1, "@context")
	assert.Contains(t, out1, "DataBook")

	// 2. Export as Turtle / RDF to stdout
	cmd2 := NewExportCmd()
	out2, err2 := executeGraphCmd(t, cmd2, testRoot, "--kind", "goal", "--format", "turtle")
	require.NoError(t, err2)
	assert.NotEmpty(t, out2)

	// 3. Export to a destination file
	tmpFile := filepath.Join(t.TempDir(), "export.jsonld")
	cmd3 := NewExportCmd()
	out3, err3 := executeGraphCmd(t, cmd3, testRoot, "--kind", "goal", "--format", "databook", "--output", tmpFile)
	require.NoError(t, err3)
	assert.Contains(t, out3, "Exported")
	assert.Contains(t, out3, tmpFile)

	fileBytes, readErr := fileutil.ReadFile(tmpFile)
	require.NoError(t, readErr)
	assert.Contains(t, string(fileBytes), "@context")
	assert.Contains(t, string(fileBytes), "DataBook")

	// 4. Unsupported export format rejection
	cmd4 := NewExportCmd()
	_, err4 := executeGraphCmd(t, cmd4, testRoot, "--kind", "goal", "--format", "invalid_xml_format")
	require.Error(t, err4)
	assert.Contains(t, err4.Error(), "unsupported export format")
}
