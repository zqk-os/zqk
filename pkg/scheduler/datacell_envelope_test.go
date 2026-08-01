package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestLogEventDataCellEnvelopeTick_matchesJobTypeConstant(t *testing.T) {
	t.Parallel()
	if LogEventDataCellEnvelopeTick != JobTypeDataCellEnvelopeTick {
		t.Fatalf("LogEventDataCellEnvelopeTick=%q want %q", LogEventDataCellEnvelopeTick, JobTypeDataCellEnvelopeTick)
	}
}

func TestDryRunDataCellEnvelopePolicy_ExecuteDefault(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	rep, err := DryRunDataCellEnvelopePolicy(tmp, datacell.ProfileStream)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PolicyAction != decisionExecute {
		t.Fatalf("action=%q reason=%q", rep.PolicyAction, rep.PolicyReason)
	}
	if rep.SchedulerCategory != CategoryDataCellEnvelope {
		t.Fatalf("category: %q", rep.SchedulerCategory)
	}
	if rep.SchedulerCategory != datacell.SchedulerCategoryDataCellEnvelope {
		t.Fatalf("scheduler/datacell category mismatch")
	}
	if rep.EnvelopeSummary == "" {
		t.Fatal("expected envelope summary")
	}
}

func TestDryRunDataCellEnvelopePolicy_UnknownProfile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	_, err := DryRunDataCellEnvelopePolicy(tmp, datacell.StorageProfile(""))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDryRunDataCellEnvelopePoliciesForKnownProfiles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	reps, err := DryRunDataCellEnvelopePoliciesForKnownProfiles(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != len(datacell.KnownStorageProfiles) {
		t.Fatalf("got %d reports, want %d", len(reps), len(datacell.KnownStorageProfiles))
	}
	for i, p := range datacell.KnownStorageProfiles {
		if reps[i].StorageProfile != string(p) {
			t.Fatalf("index %d: profile %q", i, reps[i].StorageProfile)
		}
		if reps[i].PolicyAction != decisionExecute {
			t.Fatalf("profile %s: action=%q", p, reps[i].PolicyAction)
		}
	}
}

func TestDataCellEnvelopeTickHandler_Execute(t *testing.T) {
	t.Parallel()
	h := NewDataCellEnvelopeTickHandler(t.TempDir(), logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)
	job := &ScheduledJob{ID: "SCH-envelope-tick-test", Category: CategoryDataCellEnvelope, JobType: JobTypeDataCellEnvelopeTick}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func TestDataCellEnvelopeTickHandler_Execute_drainsStewardEnqueue(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	c, err := NewStreamStewardEnqueueCoordinator(tmp, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Enqueue(context.Background(), datacell.MaintenanceOp{Name: datacell.MaintenanceOpRefreshSummary, Detail: "e2e"}); err != nil {
		t.Fatal(err)
	}
	path := datacell.StewardEnqueueJSONLPath(tmp)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	h := NewDataCellEnvelopeTickHandler(tmp, log, nil)
	job := &ScheduledJob{ID: "SCH-envelope-steward-test", Category: CategoryDataCellEnvelope, JobType: JobTypeDataCellEnvelopeTick}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected steward queue drained (file removed): %v", err)
	}
}

func TestDataCellEnvelopeTickHandler_AppendsMetricsJSONL(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	h := NewDataCellEnvelopeTickHandler(tmp, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)
	job := &ScheduledJob{ID: "SCH-envelope-jsonl-test", Category: CategoryDataCellEnvelope, JobType: JobTypeDataCellEnvelopeTick}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, paths.ProjectDataDir, paths.MetricsDir, dataCellEnvelopeTickJSONL)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatal(err)
	}
	if row["job_id"] != job.ID {
		t.Fatalf("job_id: %v", row["job_id"])
	}
	if row["envelope_tick_scheduler_job_id"] != datacell.EnvelopeTickSchedulerJobID {
		t.Fatalf("envelope_tick_scheduler_job_id: %v", row["envelope_tick_scheduler_job_id"])
	}
	if row["envelope_tick_job_type"] != datacell.EnvelopeTickJobType {
		t.Fatalf("envelope_tick_job_type: %v", row["envelope_tick_job_type"])
	}
	if _, ok := row["kind_operational_envelope_overrides"].(string); !ok {
		t.Fatalf("kind_operational_envelope_overrides: %#v", row["kind_operational_envelope_overrides"])
	}
	if s, ok := row["operational_envelope_resolved_scheduler_job_types"].(string); !ok || s == "" {
		t.Fatalf("operational_envelope_resolved_scheduler_job_types: %#v", row["operational_envelope_resolved_scheduler_job_types"])
	}
	if ev, ok := row["envelope_tick_dispatch_evaluated"].(bool); !ok || ev != false {
		t.Fatalf("envelope_tick_dispatch_evaluated: %#v", row["envelope_tick_dispatch_evaluated"])
	}
	for _, prof := range datacell.KnownStorageProfiles {
		k := "operational_envelope_" + string(prof)
		if s, ok := row[k].(string); !ok || s == "" {
			t.Fatalf("missing or empty %q: %#v", k, row[k])
		}
	}
}

func TestDataCellEnvelopeTickHandler_AppendsMetricsJSONL_tokenPolicyDenyJSON(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	s := &Scheduler{
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		secCtx:      pkgctx.NewSystemSecurityContext(),
		projectRoot: tmp,
		jobs:        map[string]*ScheduledJob{},
	}
	h := NewDataCellEnvelopeTickHandler(tmp, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), s)
	job := &ScheduledJob{
		ID:       "SCH-envelope-token-policy-test",
		Category: CategoryDataCellEnvelope,
		JobType:  JobTypeDataCellEnvelopeTick,
		EnvironmentVariables: map[string]string{
			EnvKeyEnvelopeTickDispatchDenyTokensJSON: `["file_presence"]`,
			EnvKeyEnvelopeTickDispatchMode:           envelopeTickDispatchModeOff,
		},
	}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, paths.ProjectDataDir, paths.MetricsDir, dataCellEnvelopeTickJSONL)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatal(err)
	}
	if row["envelope_tick_dispatch_token_deny_json"] != true {
		t.Fatalf("envelope_tick_dispatch_token_deny_json: %#v", row["envelope_tick_dispatch_token_deny_json"])
	}
	n, ok := row["envelope_tick_dispatch_token_deny_n"].(float64)
	if !ok || n < 1 {
		t.Fatalf("envelope_tick_dispatch_token_deny_n: %#v", row["envelope_tick_dispatch_token_deny_n"])
	}
}

func TestDataCellEnvelopeTickHandler_AppendsMetricsJSONL_tokenPolicyAllowJSON(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	s := &Scheduler{
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		secCtx:      pkgctx.NewSystemSecurityContext(),
		projectRoot: tmp,
		jobs:        map[string]*ScheduledJob{},
	}
	h := NewDataCellEnvelopeTickHandler(tmp, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), s)
	job := &ScheduledJob{
		ID:       "SCH-envelope-token-allow-test",
		Category: CategoryDataCellEnvelope,
		JobType:  JobTypeDataCellEnvelopeTick,
		EnvironmentVariables: map[string]string{
			EnvKeyEnvelopeTickDispatchAllowTokensJSON: `["file_presence"]`,
			EnvKeyEnvelopeTickDispatchMode:            envelopeTickDispatchModeOff,
		},
	}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, paths.ProjectDataDir, paths.MetricsDir, dataCellEnvelopeTickJSONL)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatal(err)
	}
	if row["envelope_tick_dispatch_token_allow_json"] != true {
		t.Fatalf("envelope_tick_dispatch_token_allow_json: %#v", row["envelope_tick_dispatch_token_allow_json"])
	}
	n, ok := row["envelope_tick_dispatch_token_allow_n"].(float64)
	if !ok || n < 1 {
		t.Fatalf("envelope_tick_dispatch_token_allow_n: %#v", row["envelope_tick_dispatch_token_allow_n"])
	}
}

func TestDataCellEnvelopeTickHandler_AppendsMetricsJSONL_tokenPolicyInvalidJSONFallsBackToFullResolution(t *testing.T) {
	t.Parallel()

	wantResolved := strings.Join(datacell.ResolvedSchedulerJobTypesFromDiscoveryTokens(), ",")

	tmp := t.TempDir()
	s := &Scheduler{
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		secCtx:      pkgctx.NewSystemSecurityContext(),
		projectRoot: tmp,
		jobs:        map[string]*ScheduledJob{},
	}
	h := NewDataCellEnvelopeTickHandler(tmp, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), s)
	job := &ScheduledJob{
		ID:       "SCH-envelope-token-policy-invalid",
		Category: CategoryDataCellEnvelope,
		JobType:  JobTypeDataCellEnvelopeTick,
		EnvironmentVariables: map[string]string{
			EnvKeyEnvelopeTickDispatchDenyTokensJSON: `{`,
			EnvKeyEnvelopeTickDispatchMode:           envelopeTickDispatchModeOff,
		},
	}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, paths.ProjectDataDir, paths.MetricsDir, dataCellEnvelopeTickJSONL)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatal(err)
	}
	if got, ok := row["operational_envelope_resolved_scheduler_job_types"].(string); !ok || got != wantResolved {
		t.Fatalf("resolved types: got %q want %q", got, wantResolved)
	}
	if _, ok := row["envelope_tick_dispatch_token_deny_json"]; ok {
		t.Fatalf("invalid token JSON must not populate token policy metrics; row=%#v", row)
	}
}

func TestJobTypeHandlerRegistry_DataCellEnvelopeTick(t *testing.T) {
	t.Parallel()
	spec, ok := jobTypeHandlerRegistry[JobTypeDataCellEnvelopeTick]
	if !ok {
		t.Fatal("jobTypeHandlerRegistry missing JobTypeDataCellEnvelopeTick")
	}
	if spec.handlerKey != "data_cell_envelope_tick" {
		t.Fatalf("handlerKey=%q", spec.handlerKey)
	}
	if spec.build == nil {
		t.Fatal("nil build")
	}
	tmp := t.TempDir()
	factory := NewHandlerFactory(
		nil,
		nil,
		nil,
		tmp,
		logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		NewNotificationContext(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil),
		nil,
		NewDefaultSchedulerMetricsCollector(),
		nil,
	)
	job := &ScheduledJob{ID: "SCH-registry-test", JobType: JobTypeDataCellEnvelopeTick}
	h := spec.build(factory.(*HandlerFactory), job)
	if h == nil {
		t.Fatal("handler is nil")
	}
}
