package storage

import (
	"strings"
	"testing"
)

func TestNormalizeChangeJournalTitle_TruncatesLongTitle(t *testing.T) {
	long := strings.Repeat("x", changeJournalTitleMaxLength+25)
	got := normalizeChangeJournalTitle(long)
	if len(got) != changeJournalTitleMaxLength {
		t.Fatalf("expected truncated title length %d, got %d", changeJournalTitleMaxLength, len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated title suffix '...', got %q", got[len(got)-3:])
	}
}

func TestNormalizeChangeJournalTitle_LeavesShortTitle(t *testing.T) {
	in := "Update: backlog_item:ITEM-901"
	got := normalizeChangeJournalTitle(in)
	if got != in {
		t.Fatalf("expected unchanged title %q, got %q", in, got)
	}
}
