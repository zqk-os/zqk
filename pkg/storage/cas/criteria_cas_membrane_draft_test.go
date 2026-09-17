package cas

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestShouldUseObjectDraftPlane_PreliminaryStatuses(t *testing.T) {
	t.Parallel()
	// exploring/identified are preliminary origins; draft is too. Active is not.
	if !shouldUseObjectDraftPlane(objects.KindBacklogItem, objects.ObjectStatusExploring) {
		t.Fatal("backlog_item exploring must park on draft plane")
	}
	if !shouldUseObjectDraftPlane(objects.KindTechnicalDebt, "identified") {
		t.Fatal("technical_debt identified must park on draft plane")
	}
	if !shouldUseObjectDraftPlane(objects.KindDocEntry, "draft") {
		t.Fatal("doc_entry draft must park on draft plane")
	}
	if shouldUseObjectDraftPlane(objects.KindBacklogItem, objects.ObjectStatusPlanned) {
		t.Fatal("planned backlog_item must use CAS, not draft plane")
	}
	if shouldUseObjectDraftPlane("glossary_term", "active") {
		t.Fatal("glossary_term active must use CAS")
	}
}
