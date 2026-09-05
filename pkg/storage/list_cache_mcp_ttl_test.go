package storage

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestGetListCache_MCPShortTTL(t *testing.T) {
	t.Setenv(zqkenv.MCPAccountID(), "ACC-test-mcp-list-ttl")
	root := t.TempDir()
	filter := &ListFilter{Kind: "backlog_item"}
	result := &QueryResult{
		Objects: []map[string]any{{objects.FieldKeyID: "BLI-1"}},
		Groups:  map[string][]map[string]any{},
		Meta:    map[string]any{"total_count": 1},
	}
	SetListCache(root, filter, 10, result)
	got, ok := GetListCache(root, filter, 10)
	if !ok || got == nil || len(got.Objects) != 1 {
		t.Fatalf("expected MCP list cache hit within TTL, ok=%v", ok)
	}
	globalListCache.mu.Lock()
	for _, e := range globalListCache.entries {
		e.AddedAt = time.Now().Add(-3 * time.Second)
	}
	globalListCache.mu.Unlock()
	if _, ok := GetListCache(root, filter, 10); ok {
		t.Fatal("expected MCP list cache miss after TTL")
	}
}
