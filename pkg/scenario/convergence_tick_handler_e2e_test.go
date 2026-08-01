package scenario

import (
	stdcontext "context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func TestConvergenceSessionTickHandler_Execute_UpdatesSession(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "tick-e2e-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	h := scheduler.NewConvergenceSessionTickHandler(provider, projectRoot)
	job := &scheduler.ScheduledJob{
		ID:      "SCH-tick-e2e",
		JobType: scheduler.JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			scheduler.EnvKeyConvergenceSessionID: cvsID,
		},
	}
	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	updated, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read CVS: %v", err)
	}
	if got, _ := updated[objects.FieldKeyLastMeasurementAt].(string); got == emptyValue {
		t.Fatalf("expected last_measurement_at set, got %#v", updated[objects.FieldKeyLastMeasurementAt])
	}
	al, ok := updated[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) < 1 {
		t.Fatalf("activity_log: %#v", updated[objects.FieldKeyActivityLog])
	}
}

func TestConvergenceSessionTickHandler_SkipsDuplicateWatermark(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "tick-dup-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	h := scheduler.NewConvergenceSessionTickHandler(provider, projectRoot)
	job := &scheduler.ScheduledJob{
		ID:      "SCH-tick-dup",
		JobType: scheduler.JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			scheduler.EnvKeyConvergenceSessionID: cvsID,
		},
	}
	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	after1, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	al1, ok := after1[objects.FieldKeyActivityLog].([]any)
	if !ok {
		t.Fatalf("activity_log type: %#v", after1[objects.FieldKeyActivityLog])
	}
	n1 := len(al1)

	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	after2, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	al2, ok := after2[objects.FieldKeyActivityLog].([]any)
	if !ok {
		t.Fatalf("activity_log type: %#v", after2[objects.FieldKeyActivityLog])
	}
	// Duplicate watermark skips full measure persist but still appends a duplicate-watermark audit entry (same as CLI persist path).
	if len(al2) != n1+1 {
		t.Fatalf("duplicate watermark should append audit only; activity_log len %d -> %d", n1, len(al2))
	}
	last, ok := al2[len(al2)-1].(map[string]any)
	if !ok {
		t.Fatalf("last activity_log entry type: %#v", al2[len(al2)-1])
	}
	if got, _ := last["action"].(string); got != "measure_no_new_health_watermark" {
		t.Fatalf("last activity action = %q, want measure_no_new_health_watermark", got)
	}
}

func TestConvergenceSessionTickHandler_SkipsPersistWhenSessionTerminal(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts1 := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts1,
			scheduler.KeyBundleCommandFingerprint: "tick-term-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	h := scheduler.NewConvergenceSessionTickHandler(provider, projectRoot)
	job := &scheduler.ScheduledJob{
		ID:      "SCH-tick-term",
		JobType: scheduler.JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			scheduler.EnvKeyConvergenceSessionID: cvsID,
		},
	}
	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	afterActive, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	lm1, _ := afterActive[objects.FieldKeyLastMeasurementAt].(string)
	al1, ok := afterActive[objects.FieldKeyActivityLog].([]any)
	if !ok {
		t.Fatalf("activity_log: %#v", afterActive[objects.FieldKeyActivityLog])
	}
	nActivity := len(al1)

	// Terminal status without the active→completed path (keeps the test robust if draft→active is constrained).
	if err := provider.Update(ctx, secCtx, cvsID, map[string]any{objects.FieldKeyStatus: "abandoned"}); err != nil {
		t.Fatalf("Update status abandoned: %v", err)
	}

	ts2 := zqktime.FormatRFC3339UTC(time.Now().UTC().Add(5 * time.Second))
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts2,
			scheduler.KeyBundleCommandFingerprint: "tick-term-fp2",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("second Execute (terminal): %v", err)
	}
	afterTerm, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, _ := afterTerm[objects.FieldKeyLastMeasurementAt].(string); got != lm1 {
		t.Fatalf("terminal session should not advance last_measurement_at: got %q want %q", got, lm1)
	}
	al2, ok := afterTerm[objects.FieldKeyActivityLog].([]any)
	if !ok {
		t.Fatalf("activity_log: %#v", afterTerm[objects.FieldKeyActivityLog])
	}
	if len(al2) != nActivity {
		t.Fatalf("terminal tick should not append activity_log: len %d -> %d", nActivity, len(al2))
	}
}

func TestConvergenceSessionTickHandler_SpawnsFollowupDraftWhenTerminalAndEnv(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts1 := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts1,
			scheduler.KeyBundleCommandFingerprint: "tick-spawn-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	h := scheduler.NewConvergenceSessionTickHandler(provider, projectRoot)
	job := &scheduler.ScheduledJob{
		ID:      "SCH-tick-spawn",
		JobType: scheduler.JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			scheduler.EnvKeyConvergenceSessionID:              cvsID,
			scheduler.EnvKeyConvergenceTickSpawnFollowupDraft: "true",
		},
	}
	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	if err := provider.Update(ctx, secCtx, cvsID, map[string]any{objects.FieldKeyStatus: "abandoned"}); err != nil {
		t.Fatalf("Update status abandoned: %v", err)
	}

	ts2 := zqktime.FormatRFC3339UTC(time.Now().UTC().Add(6 * time.Second))
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts2,
			scheduler.KeyBundleCommandFingerprint: "tick-spawn-fp",
			scheduler.KeyTestOutcome:              "test_fail",
		},
	})

	if err := h.Execute(ctx, job); err != nil {
		t.Fatalf("terminal Execute with spawn: %v", err)
	}

	storCtx := pkgctx.NewStorageContext()
	storCtx.MaxPageSize = 500
	res, err := provider.List(stdcontext.Background(), secCtx, storCtx, storage.ListFilter{
		Kind:  objects.KindConvergenceSession,
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("List convergence_session: %v", err)
	}
	var found bool
	for _, o := range res.Objects {
		id, _ := o[objects.FieldKeyID].(string)
		if id == cvsID {
			continue
		}
		if st, _ := o[objects.FieldKeyStatus].(string); st != "draft" {
			continue
		}
		if !relatedObjectRefsContain(o[objects.FieldKeyRelatedObjectRefs], cvsID) {
			continue
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected a new draft convergence_session with related_object_refs containing %s; got %d objects", cvsID, len(res.Objects))
	}
}

func relatedObjectRefsContain(raw any, want string) bool {
	switch x := raw.(type) {
	case []string:
		for _, s := range x {
			if s == want {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
