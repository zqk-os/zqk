package object

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestFormatStuckPromote_EmptyPercentDefaults(t *testing.T) {
	msg := formatStuckPromote("POL-TEST", promoteStuckDiag{
		Kind:                   "policy",
		CurrentStatus:          objects.ObjectStatusDraft,
		CurrentPercent:         0,
		MissingPercentDefaults: true,
		UsedTransitionGraph:    true,
		GraphNeighbors:         []string{"under_review", "archived"},
		SkippedByPercent: []string{
			"under_review (percent_complete 0 <= current 0)",
			"archived (percent_complete 0 <= current 0)",
		},
	}, nil, nil)
	for _, want := range []string{
		"cannot promote past '" + objects.ObjectStatusDraft + "'",
		"kind policy",
		"Lifecycle one-hop edges",
		"under_review",
		"Skipped (not strictly forward by percent_complete)",
		"default_by_status is empty/missing",
		"policy_lifecycle.yaml",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in message:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "no further non-terminal lifecycle statuses to try") {
		t.Fatalf("legacy unhelpful phrase still present:\n%s", msg)
	}
}

func TestFormatStuckPromote_ValidationRejections(t *testing.T) {
	msg := formatStuckPromote("POL-TEST", promoteStuckDiag{
		Kind:                "policy",
		CurrentStatus:       objects.ObjectStatusDraft,
		CurrentPercent:      0,
		UsedTransitionGraph: true,
		GraphNeighbors:      []string{"under_review"},
		ProbeOrder:          []string{"under_review"},
	}, []string{"under_review"}, map[string]string{
		"under_review": "body: required field missing",
	})
	if !strings.Contains(msg, "Rejected candidates") {
		t.Fatalf("expected rejected candidates section:\n%s", msg)
	}
	if !strings.Contains(msg, "body: required field missing") {
		t.Fatalf("expected validation detail:\n%s", msg)
	}
	if !strings.Contains(msg, "zqk object promote POL-TEST") {
		t.Fatalf("expected re-run tip:\n%s", msg)
	}
}

func TestFormatStuckPromote_CompleteWithArchivedNeighbor(t *testing.T) {
	msg := formatStuckPromote("BLI-TEST-001", promoteStuckDiag{
		Kind:                "backlog_item",
		CurrentStatus:       objects.ObjectStatusComplete,
		CurrentPercent:      100,
		UsedTransitionGraph: true,
		GraphNeighbors:      []string{objects.ObjectStatusArchived},
		SkippedNonProgress:  []string{objects.ObjectStatusArchived},
	}, nil, nil)

	if !strings.Contains(msg, "cannot promote past 'complete'") {
		t.Fatalf("expected cannot promote past complete, got:\n%s", msg)
	}
	if !strings.Contains(msg, "zqk object promote BLI-TEST-001") {
		t.Fatalf("expected promote guidance to archived, got:\n%s", msg)
	}
	if strings.Contains(msg, "object park BLI-TEST-001 --to archived") {
		t.Fatalf("archive from complete must tip promote, not park:\n%s", msg)
	}
	if strings.Contains(msg, "raise percent_complete.default_by_status") {
		t.Fatalf("should NOT recommend raising percent_complete for complete/archived:\n%s", msg)
	}
}
