package storage

import (
	"testing"
	"time"
)

func TestFileLockStrategyMetrics_LifetimeCounters(t *testing.T) {
	var mNil *FileLockStrategyMetrics
	eNil, rNil := mNil.GetFileLockStrategyTotalStats()
	if eNil != 0 || rNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got (%d, %d)", eNil, rNil)
	}

	m := GetFileLockStrategyMetrics()
	eInit, rInit := m.GetFileLockStrategyTotalStats()

	m.RecordAcquisition("flock", "index", 10*time.Millisecond)
	m.RecordAcquisition("flock", "index", 15*time.Millisecond)
	m.RecordAcquisition("flock", "cache", 5*time.Millisecond)

	eAfter, rAfter := m.GetFileLockStrategyTotalStats()
	if eAfter <= eInit {
		t.Errorf("expected strategy events counter to increment, got init=%d after=%d", eInit, eAfter)
	}
	if rAfter < rInit {
		t.Errorf("expected resource types tracked counter to non-decrease, got init=%d after=%d", rInit, rAfter)
	}
}
