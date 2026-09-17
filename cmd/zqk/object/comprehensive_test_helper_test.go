package object

import (
	"testing"
)

func TestGenerateComprehensiveTestID(t *testing.T) {
	id := generateComprehensiveTestID("change_journal_entry", 1)
	expected := "CHA-50001"
	if id != expected {
		t.Errorf("expected generateComprehensiveTestID(\"change_journal_entry\", 1) = %s, got %s", expected, id)
	}
}
