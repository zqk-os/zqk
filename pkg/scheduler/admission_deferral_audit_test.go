package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestRecordAdmissionDeferral_NilSafe(t *testing.T) {
	t.Parallel()
	var s *Scheduler
	s.RecordAdmissionDeferral(context.Background(), []string{"SCH-1"}, goroutinelabels.ErrPoolFull)
	(&Scheduler{}).RecordAdmissionDeferral(context.Background(), nil, goroutinelabels.ErrPoolFull)
}

func TestRecordAdmissionDeferral_WritesQueryableAudit(t *testing.T) {
	testRoot, fileStorage := prepareHandlersIsolatedTempProject(t)
	s := &Scheduler{
		projectRoot: testRoot,
		storage:     fileStorage,
		jobs:        make(map[string]*ScheduledJob),
	}
	jobID := "SCH-POOLFULL-HIST-001"
	s.jobs[jobID] = &ScheduledJob{ID: jobID, JobType: "run_wrapper", Category: "manual"}

	s.RecordAdmissionDeferral(context.Background(), []string{jobID}, goroutinelabels.ErrPoolFull)

	secCtx := pkgctx.NewSystemSecurityContext()
	events, err := fileStorage.List(context.Background(), secCtx, nil, storagepkg.ListFilter{
		Kind: "audit_event",
	})
	require.NoError(t, err)

	var matching []map[string]any
	if events != nil {
		for _, ev := range events.Objects {
			if objects.GetString(ev, objects.FieldKeyTargetID) == jobID {
				matching = append(matching, ev)
			}
		}
	}
	require.NotEmpty(t, matching, "expected durable audit event for deferred job %s", jobID)

	et := objects.GetString(matching[0], objects.FieldKeyEventType)
	if et == "" {
		if md, ok := matching[0]["metadata"].(map[string]any); ok {
			et, _ = md["event_type"].(string)
		}
	}
	require.Equal(t, auditEventSchedulerFailed, et)
}
