package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/metricsrecording"
)

func TestStorageMetrics_SkippedByDefaultInTestBinary(t *testing.T) {
	if metricsrecording.Enabled() {
		t.Skip("suite set ZQK_TEST_METRICS_RECORDING; skipping default-off assertion")
	}
	m := caspkg.GetObjectStorageMetrics()
	before := m.Creates.Load()
	m.RecordCreate(time.Millisecond, nil)
	if m.Creates.Load() != before {
		t.Fatal("expected CAS create to be skipped when metrics recording disabled")
	}
}

func TestStorageMetrics_RecordsWhenOptedIn(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	m := caspkg.GetObjectStorageMetrics()
	before := m.Creates.Load()
	m.RecordCreate(time.Millisecond, nil)
	if m.Creates.Load() != before+1 {
		t.Fatal("expected CAS create when opted in")
	}
}

func TestStorageMetrics_AuditWhenOptedIn(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	c := GetGlobalAuditMetricsCollector()
	before := c.GetSnapshot().EventsValidated
	c.RecordAuditEventValidation(true)
	if c.GetSnapshot().EventsValidated != before+1 {
		t.Fatal("expected validation metric when opted in")
	}
}

func TestStorageMetrics_AuditCollectSkippedWhenDisabled(t *testing.T) {
	if metricsrecording.Enabled() {
		t.Skip("recording enabled for whole suite")
	}
	c := GetGlobalAuditMetricsCollector()
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	now := time.Now()
	id, err := c.CollectMetrics(ctx, secCtx, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("CollectMetrics: %v", err)
	}
	if id != emptyValue {
		t.Fatalf("expected empty metric id when disabled, got %q", id)
	}
}

func TestStorageMetrics_KindDeny(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	metricsrecording.DenyKind("scheduler_health_metric")
	defer metricsrecording.AllowKind("scheduler_health_metric")

	if metricsrecording.EnabledForKind("scheduler_health_metric") {
		t.Fatal("expected kind denied")
	}
	if !metricsrecording.EnabledForKind("command_metric") {
		t.Fatal("expected other kind allowed")
	}
}
