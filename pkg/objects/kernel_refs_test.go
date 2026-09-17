package objects

import "testing"

func TestDocEntryRefsDualReadLeftoverDocumentRefs(t *testing.T) {
	t.Parallel()
	if IsKernelObjectRefField(FieldKeyDocumentRefs) {
		t.Fatal("document_refs is leftover, not a kernel pointer")
	}
	if !IsKernelObjectRefField(FieldKeyDocEntryRefs) {
		t.Fatal("doc_entry_refs must be a kernel pointer")
	}
	obj := map[string]any{
		FieldKeyDocEntryRefs: []string{"DOC-NEW"},
		FieldKeyDocumentRefs: []any{"DOC-OLD", "docs/architecture/FOO.md", " "},
	}
	got := KernelObjectRefIDs(obj, FieldKeyDocEntryRefs)
	if len(got) != 2 || got[0] != "DOC-NEW" || got[1] != "DOC-OLD" {
		t.Fatalf("dual-read IDs: %v", got)
	}
	parsed, err := ParseObject(map[string]any{
		FieldKeyID:           "BLI-1",
		FieldKeyKind:         KindBacklogItem,
		FieldKeyDocumentRefs: []string{"DOC-OLD", "docs/path.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.DocEntryRefs) != 1 || parsed.DocEntryRefs[0] != "DOC-OLD" {
		t.Fatalf("parse leftover: %v", parsed.DocEntryRefs)
	}
	v, ok := parsed.GetField(FieldKeyDocEntryRefs)
	if !ok {
		t.Fatal("GetField doc_entry_refs")
	}
	ids, _ := v.([]string)
	if len(ids) != 1 || ids[0] != "DOC-OLD" {
		t.Fatalf("GetField: %v", v)
	}
}

func TestIsLeftoverNonKernelRefField(t *testing.T) {
	t.Parallel()
	if !IsLeftoverNonKernelRefField(FieldKeyDocumentRefs) {
		t.Fatal("document_refs leftover")
	}
	if IsLeftoverNonKernelRefField(FieldKeyDocEntryRefs) {
		t.Fatal("doc_entry_refs is live")
	}
}
