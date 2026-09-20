package objects

import "testing"

func TestArchiveStatusForKind(t *testing.T) {
	t.Parallel()

	t.Run("question_has_no_archive_status", func(t *testing.T) {
		t.Parallel()
		status, ok := ArchiveStatusForKind("question")
		if ok {
			t.Fatalf("question must not report an archive status, got %q", status)
		}
	})

	t.Run("backlog_item_uses_archived", func(t *testing.T) {
		t.Parallel()
		status, ok := ArchiveStatusForKind("backlog_item")
		if !ok {
			t.Fatal("backlog_item should have an archive status")
		}
		if status != ObjectStatusArchived {
			t.Fatalf("got %q, want %q", status, ObjectStatusArchived)
		}
	})
}
