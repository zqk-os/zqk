package storage

import (
	"strings"
	"testing"
)

func TestNormalizeChangeJournalTitle_PreservesLongTitle(t *testing.T) {
	long := strings.Repeat("x", 512)
	if got := normalizeChangeJournalTitle(long); got != long {
		t.Fatalf("expected full canonical title to be preserved, got length %d", len(got))
	}
}

func TestNormalizeChangeJournalTitle_LeavesShortTitle(t *testing.T) {
	in := "Update: backlog_item:BLI-901"
	got := normalizeChangeJournalTitle(in)
	if got != in {
		t.Fatalf("expected unchanged title %q, got %q", in, got)
	}
}
