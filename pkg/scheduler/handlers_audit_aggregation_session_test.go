package scheduler

import (
	"testing"
)

func TestAuditAggregationSession_Smoke(t *testing.T) {
	session := &auditAggregationSession{
		phaseDurations: make(map[string]float64),
	}
	if session.phaseDurations == nil {
		t.Fatal("expected non-nil phase durations map")
	}
}
