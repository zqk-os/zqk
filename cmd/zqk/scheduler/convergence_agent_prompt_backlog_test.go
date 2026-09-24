package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBacklogItemRefsFromCVS(t *testing.T) {
	t.Parallel()
	if got := backlogItemRefsFromCVS(nil); got != nil {
		t.Fatalf("nil cvs: %v", got)
	}
	if got := backlogItemRefsFromCVS(map[string]any{}); got != nil {
		t.Fatalf("empty: %v", got)
	}
	got := backlogItemRefsFromCVS(map[string]any{
		objects.FieldKeyBacklogItemRefs: []any{"  BLI-a  ", "BLI-b", 42, "BLI-c"},
	})
	if len(got) != 3 || got[0] != "BLI-a" || got[1] != "BLI-b" || got[2] != "BLI-c" {
		t.Fatalf("got %v", got)
	}
}

func TestIsCriteriaRefToken(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "CRIT", "CRIT-", "crit-x", "CRIT-a b", "CRIT-a_b"} {
		if isCriteriaRefToken(s) {
			t.Fatalf("%q should be false", s)
		}
	}
	for _, s := range []string{"CRIT-DATACELL-002", "CRIT-1", "CRIT-OK-1"} {
		if !isCriteriaRefToken(s) {
			t.Fatalf("%q should be true", s)
		}
	}
}

func TestExtractVerificationHintLines(t *testing.T) {
	t.Parallel()
	if got := extractVerificationHintLines(""); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
	got := extractVerificationHintLines("noop")
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	got = extractVerificationHintLines("run zqk test run TST-EXAMPLE-001")
	if len(got) != 1 || got[0] != "zqk test run TST-EXAMPLE-001" {
		t.Fatalf("got %v", got)
	}
	got = extractVerificationHintLines("line1\n  go test ./pkg/bar -timeout 60s\n")
	if len(got) != 1 || got[0] != "go test ./pkg/bar -timeout 60s" {
		t.Fatalf("got %v", got)
	}
	prose := "Documented matrix: unit (pkg/datacell), profile matrix tests, integration/registry, spec cell suite triggers, zqk test run for storage/scheduler/cmd/zqk/system when those trees change."
	if got := extractVerificationHintLines(prose); len(got) != 0 {
		t.Fatalf("prose should not yield fake commands: %v", got)
	}
	if verificationHintAcceptable(prose) {
		t.Fatal("prose must not be acceptable as verification hint")
	}
	if !verificationHintAcceptable("go test ./pkg/foo") || !verificationHintAcceptable("zqk test run TST-EXAMPLE-001") {
		t.Fatal("expected acceptable hints")
	}
}
