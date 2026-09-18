package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestEmitJobExecutionEventViaCoordinator_FailClosed_EmptyProjectRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := emitJobExecutionEventViaCoordinator(
		ctx,
		"",
		nil,
		"scheduler_job_started",
		"SCH-TEST-001",
		"maintenance",
		"general",
		true,
		0,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty projectRoot in emitJobExecutionEventViaCoordinator, got nil")
	}
}

func TestCreateJobAuditEvent_FailClosed(t *testing.T) {
	t.Parallel()

	s := &Scheduler{
		projectRoot: "",
		storage:     nil,
	}

	ctx := context.Background()
	err := s.createJobAuditEvent(
		ctx,
		"scheduler_job_started",
		"SCH-TEST-002",
		"maintenance",
		"general",
		true,
		0,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for createJobAuditEvent with empty projectRoot, got nil")
	}
}

func TestEmitJobExecutionEvent_StartedAndTerminal_QueryableByTargetID(t *testing.T) {
	testRoot, fileStorage := prepareHandlersIsolatedTempProject(t)
	ctx := context.Background()
	jobID := "SCH-AUD-REGRESSION-001"

	// 1. Emit started event
	err := emitJobExecutionEventViaCoordinator(
		ctx,
		testRoot,
		fileStorage,
		"scheduler_job_started",
		jobID,
		"maintenance",
		"general",
		true,
		0,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error emitting started event: %v", err)
	}

	// 2. Emit completed event
	err = emitJobExecutionEventViaCoordinator(
		ctx,
		testRoot,
		fileStorage,
		"scheduler_job_completed",
		jobID,
		"maintenance",
		"general",
		true,
		1500*time.Millisecond,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error emitting completed event: %v", err)
	}

	// 3. Query audit events by target_id; should return both started and terminal events
	secCtx := pkgctx.NewSystemSecurityContext()
	events, err := fileStorage.List(ctx, secCtx, nil, storagepkg.ListFilter{
		Kind: "audit_event",
	})
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}

	var matching []map[string]any
	if events != nil {
		for _, ev := range events.Objects {
			if objects.GetString(ev, objects.FieldKeyTargetID) == jobID {
				matching = append(matching, ev)
			}
		}
	}

	if len(matching) != 2 {
		t.Fatalf("expected 2 audit events for job %s (started + completed), got %d", jobID, len(matching))
	}

	eventTypes := make(map[string]bool)
	for _, ev := range matching {
		et := objects.GetString(ev, objects.FieldKeyEventType)
		eventTypes[et] = true
	}
	if !eventTypes["scheduler_job_started"] {
		t.Errorf("missing scheduler_job_started event in audit trail for %s", jobID)
	}
	if !eventTypes["scheduler_job_completed"] {
		t.Errorf("missing scheduler_job_completed event in audit trail for %s", jobID)
	}
}
