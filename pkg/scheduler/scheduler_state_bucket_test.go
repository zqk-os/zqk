package scheduler

import (
	"strings"
	"testing"
)

func TestSchedulerBucketRules_invariants(t *testing.T) {
	t.Parallel()

	seenName := make(map[string]struct{}, len(schedulerBucketRules))
	for i, r := range schedulerBucketRules {
		if strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Prefix) == "" {
			t.Fatalf("rule %d has empty name or prefix: %+v", i, r)
		}
		if _, dup := seenName[r.Name]; dup {
			t.Fatalf("duplicate bucket name %q", r.Name)
		}
		seenName[r.Name] = struct{}{}
	}

	// Specific before general: internal cmd subtree must precede SCH-run-cmd-.
	internalIdx := -1
	cmdIdx := -1
	for i, r := range schedulerBucketRules {
		switch r.Name {
		case "internal":
			internalIdx = i
		case "cmd":
			cmdIdx = i
		}
	}
	if internalIdx < 0 || cmdIdx < 0 {
		t.Fatal("expected both internal and cmd rules")
	}
	if internalIdx >= cmdIdx {
		t.Fatalf("rule order: internal (idx %d) must come before cmd (idx %d)", internalIdx, cmdIdx)
	}
}

func TestSchedulerStateBucket_examplesMatchRulesTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want string
	}{
		{"SCH-run-bundle-1", "bundle"},
		{"SCH-run-cmd-zqk-system-0", "cmd"},
		{"SCH-run-cmd-zqk-internal-0", "internal"},
		{"SCH-run-pkg-storage-1", "pkg"},
		{"SCH-run-data-cell-stream-organism-0", "data-cell-stream"},
		{"SCH-run-ext-foo", "ext"},
		{"SCH-007", schedulerBucketCatchAll},
	}
	for _, tc := range cases {
		if got := schedulerStateBucket(tc.id); got != tc.want {
			t.Errorf("schedulerStateBucket(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestSchedulerStateBucket_READMELegendCoversAllBucketNames(t *testing.T) {
	t.Parallel()
	legend := schedulerStateBucketREADMEBucketLegend()
	for _, r := range schedulerBucketRules {
		if !strings.Contains(legend, r.Name) {
			t.Errorf("legend %q missing rule name %q", legend, r.Name)
		}
	}
	if !strings.Contains(legend, schedulerBucketCatchAll) {
		t.Errorf("legend %q missing catch-all %q", legend, schedulerBucketCatchAll)
	}
}
