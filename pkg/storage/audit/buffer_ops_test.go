package audit

import "testing"

func TestCloneAndExtractGroup(t *testing.T) {
	t.Parallel()
	src := map[string]*Group{
		"a": {Key: "a", Count: 2},
		"b": nil,
	}
	keys, groups := CloneGroups(src)
	if len(keys) != 2 || groups["a"] == nil || groups["a"].Count != 2 {
		t.Fatalf("keys=%v groups=%v", keys, groups)
	}
	groups["a"].Count = 9
	if src["a"].Count != 2 {
		t.Fatal("clone must not alias source")
	}
	got := ExtractGroup(src, "a")
	if !ShouldFlushGroup(got) || src["a"] != nil {
		t.Fatalf("extract got=%v src=%v", got, src)
	}
	if ExtractGroup(src, "missing") != nil || ShouldFlushGroup(nil) {
		t.Fatal("missing/nil")
	}
}
