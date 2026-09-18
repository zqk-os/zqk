package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func TestGatherLatestCriteriaEvidence_KeepsNewestTimestamp(t *testing.T) {
	rows := []map[string]any{
		{
			schedpkg.KeyEventType:                     schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-a"},
			schedpkg.KeyTimestamp:                     "2026-04-19T08:00:00Z",
			schedpkg.KeyCriteriaVerificationSatisfied: false,
		},
		{
			schedpkg.KeyEventType:                     schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-a"},
			schedpkg.KeyTimestamp:                     "2026-04-19T09:00:00Z",
			schedpkg.KeyCriteriaVerificationSatisfied: true,
		},
	}
	out := gatherLatestCriteriaEvidence(rows)
	if len(out) != 1 {
		t.Fatalf("got %d keys", len(out))
	}
	row := out["CRIT-a"]
	if row[schedpkg.KeyTimestamp] != "2026-04-19T09:00:00Z" {
		t.Fatalf("expected newest row, got %#v", row[schedpkg.KeyTimestamp])
	}
}

func TestGatherLatestCriteriaEvidence_IgnoresOtherEvents(t *testing.T) {
	rows := []map[string]any{
		{schedpkg.KeyEventType: "completed"},
	}
	out := gatherLatestCriteriaEvidence(rows)
	if len(out) != 0 {
		t.Fatalf("got %#v", out)
	}
}
