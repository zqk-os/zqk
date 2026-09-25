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
	validYAML := "id: CRIT-x\ncategory: acceptance\ntitle: Valid criteria title\ndescription: Substantive description with more than ten characters\n"
	if err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte(validYAML)); err != nil {
		t.Fatalf("valid criteria CAS write failed: %v", err)
	}
	aliasYAML := "id: CRIT-x\ntype: acceptance\ntitle: Valid criteria title\ndescription: Substantive description with more than ten characters\n"
	if err := RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte(aliasYAML)); err != nil {
		t.Fatalf("type alias remap for CAS: %v", err)
	}
	err = RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: \"  \"\ntitle: Valid criteria title\ndescription: Substantive description with more than ten characters\n"))
	if err == nil || !strings.Contains(err.Error(), objects.FieldKeyCategory) {
		t.Fatalf("whitespace category must refuse CAS, got %v", err)
	}
	// Invalid category enum
	err = RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: unknown_cat\ntitle: Valid criteria title\ndescription: Substantive description with more than ten characters\n"))
	if err == nil || !strings.Contains(err.Error(), "category") {
		t.Fatalf("invalid category enum must refuse CAS, got %v", err)
	}
	// Missing title
	err = RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: acceptance\ndescription: Substantive description with more than ten characters\n"))
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("missing title must refuse CAS, got %v", err)
	}
	// Missing description
	err = RefuseCriteriaCASWithoutCategory(objects.KindCriteria, false, []byte("id: CRIT-x\ncategory: acceptance\ntitle: Valid criteria title\n"))
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("missing description must refuse CAS, got %v", err)
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
		objects.FieldKeyTitle:       "Valid criteria title",
		objects.FieldKeyDescription: "Substantive criteria description for membrane test",
	}
	if UseObjectDraftPlane(objects.KindCriteria, have, false) {
		t.Fatal("categorized awaiting_verification is not preliminary; write CAS")
	}
	// Missing title must also park off CAS
	noTitle := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "Substantive criteria description for membrane test",
	}
	if !UseObjectDraftPlane(objects.KindCriteria, noTitle, false) {
		t.Fatal("criteria without title must park off CAS")
	}
}
