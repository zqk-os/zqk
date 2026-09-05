package swarm

import (
	"strings"
	"testing"
)

func TestClipSwarmToolResult_listDump(t *testing.T) {
	t.Parallel()
	dump := strings.Repeat("x", swarmListResultCap+2000)
	got, truncated := clipSwarmToolResult("zqk_object_list", dump)
	if !truncated {
		t.Fatal("expected list dump to clip")
	}
	if len(got) >= len(dump) {
		t.Fatalf("clipped len %d, original %d", len(got), len(dump))
	}
	if !strings.Contains(got, "not task evidence") {
		t.Fatalf("missing write-steer footer: %q", got[len(got)-120:])
	}
	if strings.Contains(got, strings.Repeat("x", swarmListResultCap+1)) {
		t.Fatal("clip kept more than the list cap")
	}
}

func TestClipSwarmToolResult_shortPassthrough(t *testing.T) {
	t.Parallel()
	got, truncated := clipSwarmToolResult("zqk_write_code", "ok")
	if truncated || got != "ok" {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
}

func TestSwarmToolResultCap_listTighterThanRead(t *testing.T) {
	t.Parallel()
	if swarmToolResultCap("zqk_object_list") >= swarmToolResultCap("zqk_read_code") {
		t.Fatal("list dumps must clip tighter than file reads")
	}
}
