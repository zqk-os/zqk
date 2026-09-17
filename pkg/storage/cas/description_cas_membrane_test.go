package cas

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestRefuseCASWithoutDescription(t *testing.T) {
	t.Parallel()

	// 1. Draft writes are always permitted without description (park on draft plane)
	if err := RefuseCASWithoutDescription(objects.KindBacklogItem, true, []byte("id: BLI-1\ntitle: test\n")); err != nil {
		t.Fatalf("draft write must be allowed without description: %v", err)
	}

	// 2. Kinds not requiring description (e.g. qa_success, glossary_term) are permitted
	if err := RefuseCASWithoutDescription("qa_success", false, []byte("id: QAS-1\nseal_hash: abc\n")); err != nil {
		t.Fatalf("qa_success must not require description: %v", err)
	}

	// 3. CAS writes without description must be refused for required kinds
	err := RefuseCASWithoutDescription(objects.KindBacklogItem, false, []byte("id: BLI-1\ntitle: test\n"))
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty description") {
		t.Fatalf("CAS write without description must be refused, got: %v", err)
	}

	// 4. Description too short (< 10 chars) must be refused
	err = RefuseCASWithoutDescription(objects.KindBacklogItem, false, []byte("id: BLI-1\ntitle: test\ndescription: short\n"))
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("CAS write with short description must be refused, got: %v", err)
	}

	// 5. Placeholder description ('required', 'TODO', 'none') must be refused
	err = RefuseCASWithoutDescription(objects.KindPersona, false, []byte("id: PER-1\ntitle: test\ndescription: required\n"))
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("CAS write with placeholder description must be refused, got: %v", err)
	}

	// 6. Valid substantive description must pass
	validYAML := []byte("id: BLI-1\ntitle: test\ndescription: This is a fully substantive description of the object.\n")
	if err := RefuseCASWithoutDescription(objects.KindBacklogItem, false, validYAML); err != nil {
		t.Fatalf("valid description must pass CAS membrane: %v", err)
	}
}

func TestParkObjectWithoutDescription(t *testing.T) {
	t.Parallel()

	missing := map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	if !ParkObjectWithoutDescription(objects.KindBacklogItem, missing) {
		t.Fatal("backlog_item without description must park on draft plane")
	}

	short := map[string]any{
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
		objects.FieldKeyDescription: "short",
	}
	if !ParkObjectWithoutDescription(objects.KindBacklogItem, short) {
		t.Fatal("backlog_item with short description must park on draft plane")
	}

	valid := map[string]any{
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
		objects.FieldKeyDescription: "This is a valid substantive description.",
	}
	if ParkObjectWithoutDescription(objects.KindBacklogItem, valid) {
		t.Fatal("backlog_item with valid description must not park")
	}
}
