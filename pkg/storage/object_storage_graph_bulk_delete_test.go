package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// bulkDeleteProbeConn records ExecuteQuery vs DeleteNode to prove set-based bulk delete.
type bulkDeleteProbeConn struct {
	mockGraphConnection
	executeQueries  []provider.Query
	deleteNodeCalls int
}

func (c *bulkDeleteProbeConn) DeleteNode(ctx context.Context, id string, labels []string) error {
	c.deleteNodeCalls++
	return c.mockGraphConnection.DeleteNode(ctx, id, labels)
}

func (c *bulkDeleteProbeConn) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	c.executeQueries = append(c.executeQueries, query)
	ids, _ := query.Params["deleteIds"].([]string)
	if ids == nil {
		if anyIDs, ok := query.Params["deleteIds"].([]any); ok {
			ids = make([]string, 0, len(anyIDs))
			for _, v := range anyIDs {
				if s, ok := v.(string); ok {
					ids = append(ids, s)
				}
			}
		}
	}
	for _, id := range ids {
		if c.nodes != nil {
			delete(c.nodes, id)
		}
	}
	return &provider.QueryResult{Rows: nil}, nil
}

func TestGraphBulkDelete_setBasedDoesNotCallDeleteNodePerID(t *testing.T) {
	conn := &bulkDeleteProbeConn{mockGraphConnection: mockGraphConnection{nodes: map[string]provider.Node{}}}
	const n = 250
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-bulkdel-" + itoa(i)
		ids = append(ids, id)
		conn.nodes[id] = provider.Node{
			ID:         id,
			Labels:     []string{"SchedulerJob", "Entity"},
			Properties: map[string]any{objects.FieldKeyKind: objects.KindSchedulerJob},
		}
	}

	g, err := NewGraphObjectStorage(conn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}
	ctx := WithTestHardDelete(context.Background())
	sec := pkgctx.NewSystemSecurityContext()

	res, err := g.BulkDelete(ctx, sec, ids, false)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if res.SuccessCount != n {
		t.Fatalf("SuccessCount=%d want %d failures=%v", res.SuccessCount, n, res.Errors)
	}
	if conn.deleteNodeCalls != 0 {
		t.Fatalf("expected 0 DeleteNode calls (set-based), got %d", conn.deleteNodeCalls)
	}
	if len(conn.executeQueries) == 0 {
		t.Fatal("expected ExecuteQuery set-delete")
	}
	var deleteQueries int
	for _, q := range conn.executeQueries {
		if strings.Contains(q.Query, "CREATE INDEX") {
			continue
		}
		deleteQueries++
		if !strings.Contains(q.Query, "IN $deleteIds") || !strings.Contains(q.Query, "DETACH DELETE") {
			t.Fatalf("expected set-based DETACH DELETE, got %q", q.Query)
		}
	}
	if deleteQueries != 1 {
		t.Fatalf("expected 1 set-delete query for %d ids, got %d (total queries %d)", n, deleteQueries, len(conn.executeQueries))
	}
}

func TestGraphBulkDelete_chunksLargeSets(t *testing.T) {
	conn := &bulkDeleteProbeConn{mockGraphConnection: mockGraphConnection{nodes: map[string]provider.Node{}}}
	const n = graphBulkSetDeleteBatchSize + 50
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := "SCH-run-chunk-" + itoa(i)
		ids = append(ids, id)
		conn.nodes[id] = provider.Node{ID: id, Labels: []string{"Entity"}, Properties: map[string]any{}}
	}
	g, err := NewGraphObjectStorage(conn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}
	res, err := g.BulkDelete(WithTestHardDelete(context.Background()), pkgctx.NewSystemSecurityContext(), ids, false)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if res.SuccessCount != n {
		t.Fatalf("SuccessCount=%d want %d", res.SuccessCount, n)
	}
	wantDeletes := (n + graphBulkSetDeleteBatchSize - 1) / graphBulkSetDeleteBatchSize
	var deleteQueries int
	for _, q := range conn.executeQueries {
		if strings.Contains(q.Query, "CREATE INDEX") {
			continue
		}
		deleteQueries++
	}
	if deleteQueries != wantDeletes {
		t.Fatalf("delete queries=%d want %d (total %d)", deleteQueries, wantDeletes, len(conn.executeQueries))
	}
	if conn.deleteNodeCalls != 0 {
		t.Fatalf("DeleteNode calls=%d", conn.deleteNodeCalls)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
