package storage

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPureGoIndexer_NegativeAndBoundaryValidation(t *testing.T) {
	t.Parallel()

	indexer := NewPureGoIndexer()
	require.NotNil(t, indexer)

	// 1. Nil node rejected
	err := indexer.IndexNode(nil)
	assert.Error(t, err)

	// 2. Empty ID rejected
	err = indexer.IndexNode(&IndexedNode{ID: "   "})
	assert.Error(t, err)

	// 3. Unindexed node retrieval returns false
	_, ok := indexer.GetNode("NON-EXISTENT")
	assert.False(t, ok)

	// 4. Missing edges return nil
	assert.Nil(t, indexer.GetOutEdges("NON-EXISTENT", "some_rel"))
	assert.Nil(t, indexer.GetInEdges("NON-EXISTENT", "some_rel"))

	// 5. Traverse empty path for nonexistent node returns ErrPureGoNodeNotFound
	_, err = indexer.Traverse(context.Background(), "MISSING", []string{}, 10)
	assert.ErrorIs(t, err, ErrPureGoNodeNotFound)

	// 6. Traverse empty path for existing node returns the node
	node := &IndexedNode{
		ID:        "N-001",
		Kind:      "backlog_item",
		Status:    "in_progress",
		CreatedAt: time.Now(),
	}
	require.NoError(t, indexer.IndexNode(node))
	nodes, err := indexer.Traverse(context.Background(), "N-001", []string{}, 10)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "N-001", nodes[0].ID)

	// 7. Traverse with cancelled context aborts early
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = indexer.Traverse(cancelledCtx, "N-001", []string{"rel"}, 10)
	assert.ErrorIs(t, err, context.Canceled)

	// 8. Traverse depth limit exceeded
	nodeLoop := &IndexedNode{
		ID:         "N-LOOP",
		References: map[string][]string{"step": {"N-LOOP"}},
	}
	require.NoError(t, indexer.IndexNode(nodeLoop))
	_, err = indexer.Traverse(context.Background(), "N-LOOP", []string{"step", "step", "step"}, 2)
	assert.ErrorIs(t, err, ErrPureGoCyclicLimitExceeded)
}

func TestPureGoIndexer_ReindexAndEdgeUpdates(t *testing.T) {
	t.Parallel()

	indexer := NewPureGoIndexer()

	nodeA := &IndexedNode{
		ID:     "NODE-A",
		Kind:   "requirement",
		Status: "originated",
		References: map[string][]string{
			"goal_refs": {"GOAL-1"},
		},
	}
	require.NoError(t, indexer.IndexNode(nodeA))
	assert.Equal(t, 1, indexer.NodeCount())
	assert.Equal(t, []string{"GOAL-1"}, indexer.GetOutEdges("NODE-A", "goal_refs"))
	assert.Equal(t, []string{"NODE-A"}, indexer.GetInEdges("GOAL-1", "goal_refs"))

	// Reindex with updated status and different edge
	updatedA := &IndexedNode{
		ID:     "NODE-A",
		Kind:   "requirement",
		Status: "in_progress",
		References: map[string][]string{
			"goal_refs": {"GOAL-2"},
		},
	}
	require.NoError(t, indexer.IndexNode(updatedA))
	assert.Equal(t, 1, indexer.NodeCount()) // Still 1 node

	// Old edge cleared, new edge present
	assert.Equal(t, []string{"GOAL-2"}, indexer.GetOutEdges("NODE-A", "goal_refs"))
	assert.Nil(t, indexer.GetInEdges("GOAL-1", "goal_refs"))
	assert.Equal(t, []string{"NODE-A"}, indexer.GetInEdges("GOAL-2", "goal_refs"))

	// Status index updated
	assert.Empty(t, indexer.GetNodesByStatus("originated"))
	inProg := indexer.GetNodesByStatus("in_progress")
	require.Len(t, inProg, 1)
	assert.Equal(t, "NODE-A", inProg[0].ID)
}

func TestPureGoIndexer_TraverseRecursive_CycleSafety(t *testing.T) {
	t.Parallel()

	indexer := NewPureGoIndexer()

	// Cyclic graph: A -> B -> C -> A
	nA := &IndexedNode{ID: "A", References: map[string][]string{"next": {"B"}}}
	nB := &IndexedNode{ID: "B", References: map[string][]string{"next": {"C"}}}
	nC := &IndexedNode{ID: "C", References: map[string][]string{"next": {"A"}}}

	require.NoError(t, indexer.IndexNode(nA))
	require.NoError(t, indexer.IndexNode(nB))
	require.NoError(t, indexer.IndexNode(nC))

	// Recursive traversal should discover B and C without infinite loop due to visited set
	results, err := indexer.TraverseRecursive(context.Background(), "A", "next", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2) // B and C visited (A was startID)

	// Cancelled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = indexer.TraverseRecursive(cancCtx, "A", "next", 10)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPureGoIndexer_GobSerialization(t *testing.T) {
	t.Parallel()

	indexer := NewPureGoIndexer()
	node := &IndexedNode{
		ID:          "SER-001",
		Kind:        "test_case",
		Status:      "planned",
		Title:       "Serialization Test",
		NamespaceID: "zqk:kernel",
		Attributes: map[string]any{
			"version": "1.0",
		},
		References: map[string][]string{
			"criteria_refs": {"CRIT-1"},
		},
	}
	require.NoError(t, indexer.IndexNode(node))

	var buf bytes.Buffer
	require.NoError(t, indexer.SaveSnapshot(&buf))

	// Load into fresh indexer
	restored := NewPureGoIndexer()
	require.NoError(t, restored.LoadSnapshot(&buf))

	assert.Equal(t, 1, restored.NodeCount())
	n, ok := restored.GetNode("SER-001")
	require.True(t, ok)
	assert.Equal(t, "Serialization Test", n.Title)
	assert.Equal(t, []string{"CRIT-1"}, restored.GetOutEdges("SER-001", "criteria_refs"))

	// Test RemoveNode
	require.NoError(t, restored.RemoveNode("SER-001"))
	assert.Equal(t, 0, restored.NodeCount())
	// Remove non-existent returns ErrPureGoNodeNotFound
	assert.ErrorIs(t, restored.RemoveNode("NON-EXISTENT"), ErrPureGoNodeNotFound)
}
