package storage

import "testing"

func TestFilterBlockingDependents(t *testing.T) {
	in := []string{"BLI-1", "AUD-9", "CHA-1", "CRIT-2"}
	out := filterBlockingDependents(in)
	if len(out) != 2 || out[0] != "BLI-1" || out[1] != "CRIT-2" {
		t.Fatalf("got %#v", out)
	}
}
