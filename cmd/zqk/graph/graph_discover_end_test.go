package graph

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockElevationStorage struct {
	storage.NoopObjectStorage
	readFn         func(id string) (map[string]any, error)
	getNeighborsFn func(id string, direction string) ([]map[string]any, error)
	listFn         func(filter storage.ListFilter) (*storage.QueryResult, error)
	queryFn        func(query storage.Query) (*storage.QueryResult, error)
}

func (m *mockElevationStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFn != nil {
		return m.readFn(id)
	}
	return m.NoopObjectStorage.Read(ctx, secCtx, id)
}

func (m *mockElevationStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	if m.getNeighborsFn != nil {
		return m.getNeighborsFn(id, direction)
	}
	return m.NoopObjectStorage.GetNeighbors(ctx, secCtx, id, direction)
}

func (m *mockElevationStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(filter)
	}
	return m.NoopObjectStorage.List(ctx, secCtx, storageCtx, filter)
}

func (m *mockElevationStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	if m.queryFn != nil {
		return m.queryFn(query)
	}
	return m.NoopObjectStorage.Query(ctx, secCtx, storageCtx, query)
}

func setupElevationCmdWithMockStorage(t *testing.T, cmd *cobra.Command, mock *mockElevationStorage) (*cli.Processor, *bytes.Buffer) {
	t.Helper()
	buf := new(bytes.Buffer)
	sysCtx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf)
	cmdCtx := cli.WithStorageProvider(sysCtx, mock)
	cmd.SetContext(cmdCtx)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(t.TempDir()))

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	return proc, buf
}

func TestDiscover_BuildTree_ComprehensiveBranches(t *testing.T) {
	t.Parallel()

	// 1. currentDepth > maxDepth branch
	t.Run("depth exceeds max depth", func(t *testing.T) {
		mock := &mockElevationStorage{}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "NODE-1", kind: "goal", title: "Test Node"}
		visited := map[string]bool{"NODE-1": true}
		err := buildTree(proc, root, 3, 2, visited)
		require.NoError(t, err)
		assert.Empty(t, root.children)
	})

	// 2. Storage GetNeighbors error
	t.Run("get neighbors storage error", func(t *testing.T) {
		mock := &mockElevationStorage{
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				return nil, errors.New("simulated neighbor lookup failure")
			},
		}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "NODE-FAIL", kind: "goal", title: "Fail"}
		visited := map[string]bool{"NODE-FAIL": true}
		err := buildTree(proc, root, 1, 2, visited)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulated neighbor lookup failure")
	})

	// 3. Neighbor with empty ID
	t.Run("neighbor with empty ID skipped", func(t *testing.T) {
		mock := &mockElevationStorage{
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				return []map[string]any{
					{
						objects.FieldKeyID:    "",
						objects.FieldKeyKind:  "requirement",
						objects.FieldKeyTitle: "Empty ID Item",
					},
				}, nil
			},
		}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "NODE-EMPTY", kind: "goal", title: "Parent"}
		visited := map[string]bool{"NODE-EMPTY": true}
		err := buildTree(proc, root, 1, 2, visited)
		require.NoError(t, err)
		assert.Empty(t, root.children)
	})

	// 4. Neighbor already visited (cycle protection)
	t.Run("cycle detected and skipped", func(t *testing.T) {
		mock := &mockElevationStorage{
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				return []map[string]any{
					{
						objects.FieldKeyID:    "NODE-CYCLE",
						objects.FieldKeyKind:  "goal",
						objects.FieldKeyTitle: "Cycle Node",
					},
				}, nil
			},
		}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "NODE-ROOT", kind: "goal", title: "Parent"}
		visited := map[string]bool{
			"NODE-ROOT":  true,
			"NODE-CYCLE": true,
		}
		err := buildTree(proc, root, 1, 2, visited)
		require.NoError(t, err)
		assert.Empty(t, root.children)
	})

	// 5. Recursive buildTree failure on child
	t.Run("recursive child traversal failure", func(t *testing.T) {
		mock := &mockElevationStorage{
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				if id == "NODE-PARENT" {
					return []map[string]any{
						{
							objects.FieldKeyID:    "NODE-CHILD",
							objects.FieldKeyKind:  "requirement",
							objects.FieldKeyTitle: "Child Item",
						},
					}, nil
				}
				return nil, errors.New("child tree traversal error")
			},
		}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "NODE-PARENT", kind: "goal", title: "Parent"}
		visited := map[string]bool{"NODE-PARENT": true}
		err := buildTree(proc, root, 1, 2, visited)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "child tree traversal error")
		assert.Len(t, root.children, 1)
	})

	// 6. Multi-level successful traversal
	t.Run("successful multi-level recursion", func(t *testing.T) {
		mock := &mockElevationStorage{
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				if id == "GOAL-ROOT-001" {
					return []map[string]any{
						{
							objects.FieldKeyID:    "REQ-LVL1-001",
							objects.FieldKeyKind:  "requirement",
							objects.FieldKeyTitle: "Lvl 1 Requirement",
						},
					}, nil
				}
				if id == "REQ-LVL1-001" {
					return []map[string]any{
						{
							objects.FieldKeyID:    "BLI-LVL2-001",
							objects.FieldKeyKind:  "backlog_item",
							objects.FieldKeyTitle: "Lvl 2 Backlog Item",
						},
					}, nil
				}
				return nil, nil
			},
		}
		cmd := NewDiscoverCmd()
		proc, _ := setupElevationCmdWithMockStorage(t, cmd, mock)

		root := &discoverNode{id: "GOAL-ROOT-001", kind: "goal", title: "Root"}
		visited := map[string]bool{"GOAL-ROOT-001": true}
		err := buildTree(proc, root, 1, 3, visited)
		require.NoError(t, err)
		require.Len(t, root.children, 1)
		assert.Equal(t, "REQ-LVL1-001", root.children[0].id)
		require.Len(t, root.children[0].children, 1)
		assert.Equal(t, "BLI-LVL2-001", root.children[0].children[0].id)
	})
}

func TestRunDiscover_ErrorHandling(t *testing.T) {
	t.Parallel()

	// 1. buildTree fails within runDiscover
	t.Run("buildTree failure wraps error", func(t *testing.T) {
		mock := &mockElevationStorage{
			readFn: func(id string) (map[string]any, error) {
				return map[string]any{
					objects.FieldKeyID:    id,
					objects.FieldKeyKind:  "goal",
					objects.FieldKeyTitle: "Sample Goal",
				}, nil
			},
			getNeighborsFn: func(id string, direction string) ([]map[string]any, error) {
				return nil, errors.New("subsystem network partition")
			},
		}
		cmd := NewDiscoverCmd()
		setupElevationCmdWithMockStorage(t, cmd, mock)

		err := runDiscover(cmd, "GOAL-001", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build relationship tree")
	})
}

func TestRunExport_FormatsAndErrorBranches(t *testing.T) {
	t.Parallel()

	// 1. storageProvider.List failure wraps error
	t.Run("list objects failure wraps error", func(t *testing.T) {
		mock := &mockElevationStorage{
			listFn: func(filter storage.ListFilter) (*storage.QueryResult, error) {
				return nil, errors.New("storage disk failure")
			},
		}
		cmd := NewExportCmd()
		setupElevationCmdWithMockStorage(t, cmd, mock)

		err := runExport(cmd, "json-ld", "goal", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list objects for graph export")
	})

	// 2. Formats: ttl, rdf, json, table, empty
	sampleObjects := []map[string]any{
		{
			objects.FieldKeyID:    "GOAL-EXP-001",
			objects.FieldKeyKind:  "goal",
			objects.FieldKeyTitle: "Sample Export Goal",
		},
	}

	formatsToTest := []string{"ttl", "rdf", "json", "table", ""}
	for _, fmtCase := range formatsToTest {
		fmtCase := fmtCase
		t.Run("format_"+fmtCase, func(t *testing.T) {
			mock := &mockElevationStorage{
				listFn: func(filter storage.ListFilter) (*storage.QueryResult, error) {
					return &storage.QueryResult{Objects: sampleObjects}, nil
				},
			}
			cmd := NewExportCmd()
			_, buf := setupElevationCmdWithMockStorage(t, cmd, mock)

			err := runExport(cmd, fmtCase, "goal", "")
			require.NoError(t, err)
			assert.NotEmpty(t, buf.String())
		})
	}

	// 3. File export write failure
	t.Run("file write failure when path is directory", func(t *testing.T) {
		mock := &mockElevationStorage{
			listFn: func(filter storage.ListFilter) (*storage.QueryResult, error) {
				return &storage.QueryResult{Objects: sampleObjects}, nil
			},
		}
		cmd := NewExportCmd()
		setupElevationCmdWithMockStorage(t, cmd, mock)

		// Passing an existing directory path causes file write failure
		existingDir := t.TempDir()
		err := runExport(cmd, "json-ld", "goal", existingDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write export to")
	})
}

func TestRunQuery_CoverageBranches(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	// 1. runQuery against standard file backend reports query execution failure
	t.Run("query execution fails on file backend", func(t *testing.T) {
		cmd := NewQueryCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
		buf := new(bytes.Buffer)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf))
		cmd.SetOut(buf)

		err := runQuery(cmd, "MATCH (n:BacklogItem) RETURN n", "cypher", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query execution failed")
	})

	// 2. outputQueryResults format tests
	t.Run("output query results structured formats", func(t *testing.T) {
		mock := &mockElevationStorage{}
		cmdYAML := NewQueryCmd()
		cmdYAML.Flags().String("format", "", "")
		cmdYAML.Flags().Set("format", "yaml")
		procYAML, bufYAML := setupElevationCmdWithMockStorage(t, cmdYAML, mock)

		res := &storage.QueryResult{
			Objects: []map[string]any{
				{
					objects.FieldKeyID:    "BLI-Q-001",
					objects.FieldKeyKind:  "backlog_item",
					objects.FieldKeyTitle: "Queried BLI",
				},
			},
		}

		err := outputQueryResults(cmdYAML, procYAML, res)
		require.NoError(t, err)
		assert.Contains(t, bufYAML.String(), "BLI-Q-001")

		// JSON format
		cmdJSON := NewQueryCmd()
		cmdJSON.Flags().String("format", "", "")
		cmdJSON.Flags().Set("format", "json")
		procJSON, bufJSON := setupElevationCmdWithMockStorage(t, cmdJSON, mock)

		err = outputQueryResults(cmdJSON, procJSON, res)
		require.NoError(t, err)
		assert.Contains(t, bufJSON.String(), "BLI-Q-001")
	})
}

func TestDiscover_EndToEnd_RealHierarchy(t *testing.T) {
	testRoot := setupGraphIsolatedProject(t)

	proc := createGraphTestProcessor(t, NewDiscoverCmd(), testRoot)
	ctx := context.Background()
	secCtx := proc.SecurityContext()

	// Seed connected objects with outgoing reference
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-GRAPH-001",
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeyTitle:         "Graph Subsystem Reliability",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proc.Storage().Create(ctx, secCtx, goal))

	bli := map[string]any{
		objects.FieldKeyID:            "BLI-GRAPH-001",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Elevation Unit Tests",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyGoalRefs:      []string{"GOAL-GRAPH-001"},
	}
	require.NoError(t, proc.Storage().Create(ctx, secCtx, bli))

	// Discover BLI-GRAPH-001 with depth 2
	cmdDiscover := NewDiscoverCmd()
	out, err := executeGraphCmd(t, cmdDiscover, testRoot, "BLI-GRAPH-001", "--depth", "2")
	require.NoError(t, err)
	assert.Contains(t, out, "BLI-GRAPH-001")
	assert.Contains(t, out, "GOAL-GRAPH-001")
}
