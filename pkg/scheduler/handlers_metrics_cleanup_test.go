package scheduler

import (
	"testing"
)

func TestChangeJournalAggregationHandler_Init(t *testing.T) {
	handler := NewChangeJournalAggregationHandler(nil)
	if handler == nil {
		t.Fatal("expected non-nil ChangeJournalAggregationHandler")
	}
}
