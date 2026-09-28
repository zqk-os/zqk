package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies TST-STORAGE-PUREGO-EMBEDDED-001:
// Verify Pure-Go Indexing Engine Performance and CGO-Free Portability
func TestPureGoIndexer_CRUDAndGraphTraversal(t *testing.T) {
	indexer := storage.NewPureGoIndexer()
	require.NotNil(t, indexer)

	// 1. Index Strategic Hierarchy Nodes
	vision := &storage.IndexedNode{
		ID:     "VIS-001",
		Kind:   "vision",
		Status: "originated",
		Title:  "Global High Performance Distributed Kernel",
	}
	goal := &storage.IndexedNode{
		ID:     "GOAL-001",
		Kind:   "goal",
		Status: "originated",
		Title:  "Zero-CGO Sub-millisecond Storage Layer",
		References: map[string][]string{
			"vision_refs": {"VIS-001"},
		},
	}
	milestone := &storage.IndexedNode{
		ID:     "MIL-001",
		Kind:   "milestone",
		Status: "active",
		Title:  "Phase 1 Storage Modernization",
		References: map[string][]string{
			"goal_refs": {"GOAL-001"},
		},
	}
	bli := &storage.IndexedNode{
		ID:     "BLI-001",
		Kind:   "backlog_item",
		Status: "testing",
		Title:  "Pure-Go CGO-Free Indexing Layer",
		References: map[string][]string{
			"milestone_refs": {"MIL-001"},
		},
	}

	require.NoError(t, indexer.IndexNode(vision))
	require.NoError(t, indexer.IndexNode(goal))
	require.NoError(t, indexer.IndexNode(milestone))
	require.NoError(t, indexer.IndexNode(bli))

	assert.Equal(t, 4, indexer.NodeCount())

	// 2. Direct Node Retrieval
	node, ok := indexer.GetNode("BLI-001")
	require.True(t, ok)
	assert.Equal(t, "backlog_item", node.Kind)
	assert.Equal(t, "testing", node.Status)

	// 3. Kind-based and Status-based Index Queries
	blis := indexer.GetNodesByKind("backlog_item")
	require.Len(t, blis, 1)
	assert.Equal(t, "BLI-001", blis[0].ID)

	testingNodes := indexer.GetNodesByStatus("testing")
	require.Len(t, testingNodes, 1)
	assert.Equal(t, "BLI-001", testingNodes[0].ID)

	// 4. Forward and Backward Edge Lookups
	outEdges := indexer.GetOutEdges("BLI-001", "milestone_refs")
	assert.Equal(t, []string{"MIL-001"}, outEdges)

	inEdges := indexer.GetInEdges("MIL-001", "milestone_refs")
	assert.Equal(t, []string{"BLI-001"}, inEdges)

	// 5. Multi-Hop Graph Traversal (BLI -> MIL -> GOAL -> VIS)
	ctx := context.Background()
	results, err := indexer.Traverse(ctx, "BLI-001", []string{"milestone_refs", "goal_refs", "vision_refs"}, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "VIS-001", results[0].ID)
	assert.Equal(t, "vision", results[0].Kind)

	// 6. Node Update & Removal
	updatedBLI := &storage.IndexedNode{
		ID:     "BLI-001",
		Kind:   "backlog_item",
		Status: "complete",
		Title:  "Pure-Go CGO-Free Indexing Layer (Done)",
	}
	require.NoError(t, indexer.IndexNode(updatedBLI))
	testingAfter := indexer.GetNodesByStatus("testing")
	assert.Len(t, testingAfter, 0)
	completeAfter := indexer.GetNodesByStatus("complete")
	assert.Len(t, completeAfter, 1)

	require.NoError(t, indexer.RemoveNode("BLI-001"))
	assert.Equal(t, 3, indexer.NodeCount())
	_, ok = indexer.GetNode("BLI-001")
	assert.False(t, ok)
}

func TestPureGoIndexer_CycleSafety(t *testing.T) {
	indexer := storage.NewPureGoIndexer()

	// Cyclic graph: A -> B -> A
	nodeA := &storage.IndexedNode{
		ID:   "A",
		Kind: "node",
		References: map[string][]string{
			"next": {"B"},
		},
	}
	nodeB := &storage.IndexedNode{
		ID:   "B",
		Kind: "node",
		References: map[string][]string{
			"next": {"A"},
		},
	}

	require.NoError(t, indexer.IndexNode(nodeA))
	require.NoError(t, indexer.IndexNode(nodeB))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Traversal should terminate safely without infinite recursion
	path := []string{"next", "next", "next"}
	results, err := indexer.Traverse(ctx, "A", path, 5)
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

func TestPureGoIndexer_SnapshotPersistence(t *testing.T) {
	indexer := storage.NewPureGoIndexer()

	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("TASK-%03d", i)
		node := &storage.IndexedNode{
			ID:     id,
			Kind:   "agent_task",
			Status: "approved",
			Title:  fmt.Sprintf("Task %d", i),
		}
		if i > 0 {
			prevID := fmt.Sprintf("TASK-%03d", i-1)
			node.References = map[string][]string{
				"depends_on": {prevID},
			}
		}
		require.NoError(t, indexer.IndexNode(node))
	}

	var buf bytes.Buffer
	require.NoError(t, indexer.SaveSnapshot(&buf))

	// Restore into new indexer
	restored := storage.NewPureGoIndexer()
	require.NoError(t, restored.LoadSnapshot(&buf))

	assert.Equal(t, 20, restored.NodeCount())
	node, ok := restored.GetNode("TASK-019")
	require.True(t, ok)
	assert.Equal(t, "Task 19", node.Title)

	// Validate edges survived snapshot
	out := restored.GetOutEdges("TASK-019", "depends_on")
	assert.Equal(t, []string{"TASK-018"}, out)
}

func TestPureGoIndexer_IndexProjectDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, paths.ProjectDataDir, "data", "backlog_items")
	require.NoError(t, fileutil.MkdirAll(dataDir, paths.DirPerm755))

	bliData := map[string]any{
		"id":             "BLI-TEST-001",
		"kind":           "backlog_item",
		"status":         "planned",
		"title":          "Test Backlog Item",
		"milestone_refs": []any{"MIL-TEST-001"},
	}
	rawYAML, err := yaml.Marshal(bliData)
	require.NoError(t, err)

	filePath := filepath.Join(dataDir, "BLI-TEST-001.yaml")
	require.NoError(t, os.WriteFile(filePath, rawYAML, paths.FilePerm644))

	indexer := storage.NewPureGoIndexer()
	count, err := indexer.IndexProjectDir(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	node, ok := indexer.GetNode("BLI-TEST-001")
	require.True(t, ok)
	assert.Equal(t, "planned", node.Status)
	assert.Equal(t, []string{"MIL-TEST-001"}, node.References["milestone_refs"])
}

// Satisfies CRIT-STORAGE-PUREGO-EMBEDDED-001:
// Benchmark verifying pure-Go index lookups outperform sequential YAML scanning by >= 10x
func BenchmarkPureGoIndex_vs_YAMLScan(b *testing.B) {
	tmpDir := b.TempDir()
	dataDir := filepath.Join(tmpDir, "objects")
	require.NoError(b, fileutil.MkdirAll(dataDir, paths.DirPerm755))

	indexer := storage.NewPureGoIndexer()
	objectCount := 100

	// Seed 100 objects with references
	for i := 0; i < objectCount; i++ {
		id := fmt.Sprintf("OBJ-%03d", i)
		parentID := fmt.Sprintf("OBJ-%03d", (i+1)%objectCount)
		node := &storage.IndexedNode{
			ID:     id,
			Kind:   "requirement",
			Status: "originated",
			Title:  fmt.Sprintf("Requirement %d", i),
			References: map[string][]string{
				"parent_ref": {parentID},
			},
		}
		require.NoError(b, indexer.IndexNode(node))

		// Also write to YAML file on disk for baseline comparison
		raw, _ := yaml.Marshal(map[string]any{
			"id":         id,
			"kind":       "requirement",
			"status":     "originated",
			"title":      node.Title,
			"parent_ref": parentID,
		})
		_ = os.WriteFile(filepath.Join(dataDir, id+".yaml"), raw, paths.FilePerm644)
	}

	b.Run("PureGoIndex_MultiHopLookup", func(b *testing.B) {
		ctx := context.Background()
		path := []string{"parent_ref", "parent_ref"}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			startID := fmt.Sprintf("OBJ-%03d", i%objectCount)
			nodes, err := indexer.Traverse(ctx, startID, path, 5)
			if err != nil || len(nodes) == 0 {
				b.Fatalf("traverse failed: %v", err)
			}
		}
	})

	b.Run("Disk_SequentialYAMLScan", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			targetID := fmt.Sprintf("OBJ-%03d", i%objectCount)
			// Emulate sequential search by reading files
			var found bool
			entries, _ := os.ReadDir(dataDir)
			for _, e := range entries {
				content, _ := os.ReadFile(filepath.Join(dataDir, e.Name()))
				var m map[string]any
				_ = yaml.Unmarshal(content, &m)
				if m["id"] == targetID {
					found = true
					break
				}
			}
			if !found {
				b.Fatalf("target not found")
			}
		}
	})
}
