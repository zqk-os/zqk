package graph_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/graph"
	"github.com/stretchr/testify/require"
)

func TestGraphStore(t *testing.T) {
	ctx := context.Background()
	store := graph.NewStore()

	node := &graph.Node{
		ID:   "REQ-001",
		Kind: "requirement",
		Properties: map[string]any{
			"status": objects.ObjectStatusPlanned,
		},
	}

	err := store.PutNode(ctx, node)
	require.NoError(t, err)

	fetched, ok := store.GetNode(ctx, "REQ-001")
	require.True(t, ok)
	require.Equal(t, "requirement", fetched.Kind)
}
