package storage

import "testing"

func TestUpdateMutationMetrics_RecordSnapshotReset(t *testing.T) {
	rInit, kInit := GetUpdateMutationTotalStats()

	ResetUpdateMutationClassMetrics()
	RecordUpdateMutationClass("scheduler_job", updateMutationClassRuntimeDelta)
	RecordUpdateMutationClass("scheduler_job", updateMutationClassRuntimeDelta)
	RecordUpdateMutationClass("scheduler_job", updateMutationClassStructural)
	RecordUpdateMutationClass("backlog_item", updateMutationClassStructural)

	rAfter, kAfter := GetUpdateMutationTotalStats()
	if rAfter != rInit+4 {
		t.Fatalf("expected recorded total to increase by 4, got init=%d after=%d", rInit, rAfter)
	}
	if kAfter < kInit+2 {
		t.Fatalf("expected unique kinds tracked to increase by at least 2, got init=%d after=%d", kInit, kAfter)
	}

	got := GetUpdateMutationClassSnapshot()
	if got["scheduler_job"][updateMutationClassRuntimeDelta] != 2 {
		t.Fatalf("scheduler_job runtime_delta=%d want 2", got["scheduler_job"][updateMutationClassRuntimeDelta])
	}
	if got["scheduler_job"][updateMutationClassStructural] != 1 {
		t.Fatalf("scheduler_job structural=%d want 1", got["scheduler_job"][updateMutationClassStructural])
	}
	if got["backlog_item"][updateMutationClassStructural] != 1 {
		t.Fatalf("backlog_item structural=%d want 1", got["backlog_item"][updateMutationClassStructural])
	}

	ResetUpdateMutationClassMetrics()
	got = GetUpdateMutationClassSnapshot()
	if len(got) != 0 {
		t.Fatalf("expected empty snapshot after reset, got %d kinds", len(got))
	}
}
