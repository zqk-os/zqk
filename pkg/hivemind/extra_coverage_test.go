package hivemind_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestActivityLog_GetRecentAndUnsignedPulse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	spine := &mockSpine{}

	// Nil signer (unsigned pulse)
	log := hivemind.NewActivityLog(spine, nil)

	log.Pulse(ctx, "source-1", "cat-1", "msg 1", nil)
	log.Pulse(ctx, "source-2", "cat-2", "msg 2", map[string]any{"extra": "data"})
	log.Pulse(ctx, "source-3", "cat-3", "msg 3", nil)

	// GetRecent with n less than total
	recent2 := log.GetRecent(2)
	if len(recent2) != 2 {
		t.Errorf("expected 2 recent logs, got %d", len(recent2))
	}
	if recent2[0].Source != "source-2" || recent2[1].Source != "source-3" {
		t.Errorf("unexpected recent entries: %+v", recent2)
	}

	// GetRecent with n greater than total
	recentAll := log.GetRecent(10)
	if len(recentAll) != 3 {
		t.Errorf("expected 3 total logs, got %d", len(recentAll))
	}
}

type errGraphConnection struct {
	provider.GraphConnection
}

func (e *errGraphConnection) ExecuteVectorQuery(_ context.Context, _ provider.VectorQuery) (*provider.QueryResult, error) {
	return nil, errors.New("simulated vector query failure")
}

type nestedGraphConnection struct {
	provider.GraphConnection
}

func (n *nestedGraphConnection) ExecuteVectorQuery(_ context.Context, _ provider.VectorQuery) (*provider.QueryResult, error) {
	return &provider.QueryResult{
		Rows: []map[string]any{
			{
				objects.FieldKeyID:    "doc-1",
				objects.FieldKeyScore: float64(0.95),
				"properties": map[string]any{
					objects.FieldKeyKind: "nested_kind",
				},
			},
			{
				objects.FieldKeyID:    "doc-2",
				objects.FieldKeyScore: float32(0.85),
				"properties": map[string]any{
					objects.FieldKeyKind: "other_kind",
				},
			},
			{
				objects.FieldKeyID: "doc-3",
				"similarity":       float32(0.75),
				"properties": map[string]any{
					objects.FieldKeyKind: "nested_kind",
				},
			},
			{
				objects.FieldKeyID: "doc-4",
				"similarity":       float64(0.65),
				"properties": map[string]any{
					objects.FieldKeyKind: "nested_kind",
				},
			},
		},
	}, nil
}

func TestDynamicQueryRouter_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vector := []float32{0.5, 0.5}

	// 1. Error during vector query execution
	errConn := &errGraphConnection{}
	errRouter := hivemind.NewDynamicQueryRouter(errConn, hivemind.RouterConfig{})
	if _, err := errRouter.RouteHybridQuery(ctx, vector, 5, hivemind.HybridConstraints{}); err == nil {
		t.Errorf("expected error on failed vector query")
	}

	// 2. Nested properties, default limit (<= 0), limit capping, and float32/float64 scores
	nestedConn := &nestedGraphConnection{}
	router := hivemind.NewDynamicQueryRouter(nestedConn, hivemind.RouterConfig{})

	// limit <= 0 defaults to 10
	res, err := router.RouteHybridQuery(ctx, vector, 0, hivemind.HybridConstraints{Kind: "nested_kind"})
	if err != nil {
		t.Fatalf("RouteHybridQuery failed: %v", err)
	}
	// doc-1 (0.95), doc-3 (0.75), doc-4 (0.65) match nested_kind; doc-2 has other_kind
	if len(res) != 3 {
		t.Errorf("expected 3 results matching nested_kind, got %d", len(res))
	}

	// limit of 2 forces truncation (len(results) > limit)
	resTrunc, err := router.RouteHybridQuery(ctx, vector, 2, hivemind.HybridConstraints{Kind: "nested_kind"})
	if err != nil {
		t.Fatalf("RouteHybridQuery with truncation failed: %v", err)
	}
	if len(resTrunc) != 2 {
		t.Errorf("expected 2 results after truncation, got %d", len(resTrunc))
	}
}
