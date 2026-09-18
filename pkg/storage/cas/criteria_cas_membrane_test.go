package cas

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRefuseCriteriaCASWithoutCategory(t *testing.T) {
	t.Parallel()
	if err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, true, []byte("id: CRIT-x\n")); err != nil {
		t.Fatalf("draft write must be allowed: %v", err)
	}
	if err := RefuseCriteriaCASWithoutCategory(objects.KindBacklogItem, false, []byte("id: BLI-x\n")); err != nil {
		t.Fatalf("non-criteria: %v", err)
	}
	err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\nkind: criteria\n"))
	if err == nil || !strings.Contains(err.Error(), objects.FieldKeyCategory) {
		t.Fatalf("hash CAS without category must refuse, got %v", err)
	}
	if err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: acceptance\n")); err != nil {
		t.Fatalf("categorized CAS: %v", err)
	}
	if err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ntype: acceptance\n")); err != nil {
		t.Fatalf("type alias remap for CAS: %v", err)
	}
	err = RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: \"  \"\n"))
	if err == nil || !strings.Contains(err.Error(), objects.FieldKeyCategory) {
		t.Fatalf("whitespace category must refuse CAS, got %v", err)
	}
}

func TestUseObjectDraftPlane_incompleteCriteria(t *testing.T) {
	t.Parallel()
	missing := map[string]any{
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
	}
	if !UseObjectDraftPlane(objects.KindCriteria, missing, false) {
		t.Fatal("create without category must park off CAS")
	}
	if UseObjectDraftPlane(objects.KindCriteria, missing, true) {
		t.Fatal("--promote must not park; CAS membrane refuses instead")
	}
	have := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "Substantive criteria description for membrane test",
	}
	if UseObjectDraftPlane(objects.KindCriteria, have, false) {
		t.Fatal("categorized awaiting_verification is not preliminary; write CAS")
	}
}
