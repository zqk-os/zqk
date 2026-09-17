package audit

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestMetricsSnapshotRates(t *testing.T) {
	t.Parallel()
	s := MetricsSnapshot{
		EventsCreated:     4,
		EventsCreatedCAS:  1,
		EventsFailed:      1,
		EventsValidated:   2,
		EventsSkipped:     1,
		TotalCreationTime: 8,
	}
	if s.AverageCreationTime() != 2 {
		t.Fatalf("avg=%v", s.AverageCreationTime())
	}
	if s.CASUsageRate() != 25 || s.FailureRate() != 25 || s.ValidationSkipRate() != 50 {
		t.Fatalf("cas=%v fail=%v skip=%v", s.CASUsageRate(), s.FailureRate(), s.ValidationSkipRate())
	}
	zero := MetricsSnapshot{}
	if zero.AverageCreationTime() != 0 || zero.CASUsageRate() != 0 || zero.ValidationSkipRate() != 0 {
		t.Fatalf("%+v", zero)
	}
	d := s.DurationSeconds()
	if d.Avg != s.AverageCreationTime().Seconds() || d.Fastest != d.Avg || d.Baseline != d.Avg {
		t.Fatalf("%+v", d)
	}
}

func TestClassifyEventType(t *testing.T) {
	t.Parallel()
	if ClassifyEventType(EventTypeObjectCreation) != BucketCreation {
		t.Fatal("creation")
	}
	if ClassifyEventType(EventTypeCacheRefresh) != BucketSystem {
		t.Fatal("system")
	}
	if ClassifyEventType("nope") != BucketOther {
		t.Fatal("other")
	}
}

func TestCommandMetricMetadata(t *testing.T) {
	t.Parallel()
	s := MetricsSnapshot{EventsCreated: 4, EventsCreatedCAS: 1, EventsFailed: 1, MaxCreationTime: time.Millisecond}
	got := CommandMetricMetadata(s, 60)
	if got[MetaEventsCreatedCAS] != int64(1) || got[MetaMeasurementPeriod] != float64(60) {
		t.Fatalf("%v", got)
	}
	if s.SuccessCount() != 3 {
		t.Fatalf("success=%d", s.SuccessCount())
	}
	obj := map[string]any{objects.FieldKeyMetadata: map[string]any{"keep": true}}
	AttachCommandMetricMetadata(obj, s, 1)
	meta, _ := obj[objects.FieldKeyMetadata].(map[string]any)
	if meta["keep"] != true || meta[MetaEventsCreatedCAS] != int64(1) {
		t.Fatalf("%v", meta)
	}
}
