package objects

import (
	"errors"
	"testing"
	"time"
)

func TestKindMappingsMetrics_Comprehensive(t *testing.T) {
	ResetKindMappingsMetrics()
	m := GetKindMappingsMetrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}

	// 1. Config loads
	m.RecordConfigLoad(50*time.Millisecond, nil)
	m.RecordConfigLoad(100*time.Millisecond, errors.New("load failed"))

	// 2. Backend switches and config merges
	m.RecordBackendSwitch()
	m.RecordBackendConfigMerge()

	// 3. Lookups and cache
	m.RecordDirectoryLookup(true, false)
	m.RecordDirectoryLookup(false, true)
	m.RecordKindLookup(true, false)
	m.RecordKindLookup(false, true)
	m.RecordLookupError()

	// 4. Initialization and scans
	m.RecordInitialization(10*time.Millisecond, nil)
	m.RecordInitialization(20*time.Millisecond, nil)
	m.RecordInitialization(5*time.Millisecond, errors.New("init error"))
	m.RecordDirectoryScan()
	m.RecordSpecScan()
	m.RecordMappingDiscovered()

	snap := m.GetSnapshot()

	if snap.ConfigLoads != 1 || snap.ConfigLoadFailures != 1 {
		t.Errorf("unexpected config load metrics: %#v", snap)
	}
	if snap.BackendSwitches != 1 || snap.BackendConfigMerges != 1 {
		t.Errorf("unexpected backend metrics: %#v", snap)
	}
	if snap.DirectoryLookups != 2 || snap.KindLookups != 2 {
		t.Errorf("unexpected lookup counts: %#v", snap)
	}
	if snap.CacheHits != 2 || snap.CacheMisses != 2 {
		t.Errorf("unexpected cache hits/misses: %#v", snap)
	}
	if snap.InferenceRuleHits != 2 || snap.LookupErrors != 1 {
		t.Errorf("unexpected inference/lookup errors: %#v", snap)
	}
	if snap.Initializations != 3 || snap.DiscoveryErrors != 1 {
		t.Errorf("unexpected initialization metrics: %#v", snap)
	}
	if snap.DirectoriesScanned != 1 || snap.SpecsScanned != 1 || snap.MappingsDiscovered != 1 {
		t.Errorf("unexpected scan metrics: %#v", snap)
	}

	// Rates and averages
	if hitRate := snap.CacheHitRate(); hitRate != 50.0 {
		t.Errorf("expected 50%% hit rate, got %f", hitRate)
	}
	if avgConfig := snap.AverageConfigLoadTime(); avgConfig != 50*time.Millisecond {
		t.Errorf("expected 50ms avg config load time, got %v", avgConfig)
	}
	if avgInit := snap.AverageInitializationTime(); avgInit != 10*time.Millisecond {
		t.Errorf("expected 10ms avg init time, got %v", avgInit)
	}
	if successRate := snap.ConfigLoadSuccessRate(); successRate != 50.0 {
		t.Errorf("expected 50%% config success rate, got %f", successRate)
	}
	if ruleRate := snap.InferenceRuleUsageRate(); ruleRate != 50.0 {
		t.Errorf("expected 50%% inference rule rate, got %f", ruleRate)
	}

	// Zero denominator coverage
	emptySnap := KindMappingsMetricsSnapshot{}
	if emptySnap.CacheHitRate() != 0 {
		t.Error("expected 0 for empty cache hit rate")
	}
	if emptySnap.AverageConfigLoadTime() != 0 {
		t.Error("expected 0 for empty avg config load time")
	}
	if emptySnap.AverageInitializationTime() != 0 {
		t.Error("expected 0 for empty avg init time")
	}
	if emptySnap.ConfigLoadSuccessRate() != 0 {
		t.Error("expected 0 for empty config success rate")
	}
	if emptySnap.InferenceRuleUsageRate() != 0 {
		t.Error("expected 0 for empty inference rule usage rate")
	}
}
