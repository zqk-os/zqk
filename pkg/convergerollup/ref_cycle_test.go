package convergerollup

import (
	"fmt"
	"testing"
)

func TestDetectCVSRefCycle_none(t *testing.T) {
	g := map[string][]string{
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-c"},
		"CVS-c": {},
	}
	refsFor := func(id string) ([]string, error) {
		return g[id], nil
	}
	cycle, ok := DetectCVSRefCycle("CVS-a", refsFor, MaxRelatedCVSHopDepth)
	if ok {
		t.Fatalf("unexpected cycle %#v", cycle)
	}
}

func TestDetectCVSRefCycle_direct(t *testing.T) {
	g := map[string][]string{
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-a"},
	}
	refsFor := func(id string) ([]string, error) {
		return g[id], nil
	}
	cycle, ok := DetectCVSRefCycle("CVS-a", refsFor, MaxRelatedCVSHopDepth)
	if !ok {
		t.Fatal("want cycle")
	}
	if len(cycle) < 3 {
		t.Fatalf("cycle too short: %#v", cycle)
	}
}

func TestDetectCVSRefCycle_maxDepthStops(t *testing.T) {
	g := map[string][]string{
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-c"},
		"CVS-c": {"CVS-a"},
	}
	refsFor := func(id string) ([]string, error) {
		return g[id], nil
	}
	_, ok := DetectCVSRefCycle("CVS-a", refsFor, 1)
	if ok {
		t.Fatal("depth 1 should not expand b->c enough to require reporting cycle as detected")
	}
}

func TestDetectCVSRefCycle_readErr(t *testing.T) {
	refsFor := func(id string) ([]string, error) {
		return nil, fmt.Errorf("boom")
	}
	_, ok := DetectCVSRefCycle("CVS-a", refsFor, MaxRelatedCVSHopDepth)
	if ok {
		t.Fatal("read error should not report cycle")
	}
}
