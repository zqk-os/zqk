package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// bulkCreateProbeConn records ExecuteBatch vs CreateNode to prove UNWIND path.
type bulkCreateProbeConn struct {
	mockGraphConnection
	executeBatchCalls int
	createNodeCalls   int
	lastBatchOps      int
}

func (c *bulkCreateProbeConn) CreateNode(ctx context.Context, node provider.Node) error {
	c.createNodeCalls++
	return c.mockGraphConnection.CreateNode(ctx, node)
}

func (c *bulkCreateProbeConn) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	c.executeBatchCalls++
	c.lastBatchOps = len(operations)
	return c.mockGraphConnection.ExecuteBatch(ctx, operations)
}

func TestGraphBulkCreate_usesExecuteBatchNotCreateNodeLoop(t *testing.T) {
	t.Setenv(zqkenv.RawMockGraph().Name(), "true")

	conn := &bulkCreateProbeConn{mockGraphConnection: mockGraphConnection{nodes: map[string]provider.Node{}}}
	g, err := NewGraphObjectStorage(conn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}

	const n = 120
	objs := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-bulkcreate-" + itoa(i)
		objs = append(objs, map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyKind:   objects.KindSchedulerJob,
			objects.FieldKeyStatus: objects.ObjectStatusPending,
			objects.FieldKeyName:   id,
		})
	}

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	res, err := g.BulkCreate(ctx, sec, objs)
	if err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	if res.SuccessCount != n {
		t.Fatalf("SuccessCount=%d want %d failures=%v", res.SuccessCount, n, res.Errors)
	}
	if conn.executeBatchCalls == 0 {
		t.Fatal("expected ExecuteBatch (UNWIND path), got 0 calls")
	}
	// CreateNode may still run inside mock ExecuteBatch; the Graph BulkCreate
	// path must not call CreateNode directly (only via ExecuteBatch).
	// Probe counts CreateNode on the outer conn — ExecuteBatch delegates to
	// embedded mock which uses CreateNode on embedded, not outer override...
	// So createNodeCalls on probe stays 0 if BulkCreate never calls CreateNode.
	if conn.createNodeCalls != 0 {
		t.Fatalf("BulkCreate must not call CreateNode directly (got %d); use ExecuteBatch", conn.createNodeCalls)
	}
	if conn.lastBatchOps == 0 {
		t.Fatal("expected non-empty ExecuteBatch operations")
	}
}

func TestGraphBulkCreate_chunksAtUNWINDBatchSize(t *testing.T) {
	t.Setenv(zqkenv.RawMockGraph().Name(), "true")

	conn := &bulkCreateProbeConn{mockGraphConnection: mockGraphConnection{nodes: map[string]provider.Node{}}}
	g, err := NewGraphObjectStorage(conn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}

	n := graphBulkUNWINDBatchSize + 3
	objs := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-bulkchunk-" + itoa(i)
		objs = append(objs, map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyKind:   objects.KindSchedulerJob,
			objects.FieldKeyStatus: objects.ObjectStatusPending,
			objects.FieldKeyName:   id,
		})
	}

	res, err := g.BulkCreate(context.Background(), pkgctx.NewSystemSecurityContext(), objs)
	if err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	if res.SuccessCount != n {
		t.Fatalf("SuccessCount=%d want %d errors=%v", res.SuccessCount, n, res.Errors)
	}
	wantCalls := 2 // 500 + 3
	if conn.executeBatchCalls != wantCalls {
		t.Fatalf("ExecuteBatch calls=%d want %d", conn.executeBatchCalls, wantCalls)
	}
}

func TestGraphBulkUpdate_usesExecuteBatch(t *testing.T) {
	t.Setenv(zqkenv.RawMockGraph().Name(), "true")

	conn := &bulkCreateProbeConn{mockGraphConnection: mockGraphConnection{nodes: map[string]provider.Node{}}}
	g, err := NewGraphObjectStorage(conn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	const n = 80
	objs := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-bulkupd-" + itoa(i)
		objs = append(objs, map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyKind:   objects.KindSchedulerJob,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
			objects.FieldKeyTitle:  id,
			"job_type":             "run_wrapper",
			"trigger_type":         "immediate",
		})
	}
	if res, err := g.BulkCreate(ctx, sec, objs); err != nil || res.SuccessCount != n {
		t.Fatalf("BulkCreate setup: err=%v success=%v", err, res)
	}
	conn.executeBatchCalls = 0
	conn.createNodeCalls = 0

	updates := make([]BulkUpdateItem, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-bulkupd-" + itoa(i)
		updates = append(updates, BulkUpdateItem{
			ID:      id,
			Updates: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived},
		})
	}
	res, err := g.BulkUpdate(ctx, sec, updates)
	if err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	if res.SuccessCount != n {
		t.Fatalf("BulkUpdate SuccessCount=%d want %d errors=%v", res.SuccessCount, n, res.Errors)
	}
	if conn.executeBatchCalls == 0 {
		t.Fatal("expected ExecuteBatch for BulkUpdate")
	}
}
