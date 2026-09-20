package mcp

import "testing"

func TestMcpFilterExprs_ListAndCountShareMembrane(t *testing.T) {
	t.Parallel()
	if got := mcpFilterExprs([]any{"status=grooming"}); len(got) != 1 || got[0] != "status=grooming" {
		t.Fatalf("[]any: %v", got)
	}
	if got := mcpFilterExprs([]string{"status=grooming", "kind=x"}); len(got) != 2 {
		t.Fatalf("[]string: %v", got)
	}
	if got := mcpFilterExprs("priority_plan_ref=PRI-1"); len(got) != 1 || got[0] != "priority_plan_ref=PRI-1" {
		t.Fatalf("string: %v", got)
	}
	if got := mcpFilterExprs(nil); got != nil {
		t.Fatalf("nil: %v", got)
	}
	if got := mcpFilterExprs([]any{}); got != nil {
		t.Fatalf("empty []any: %v", got)
	}
}
