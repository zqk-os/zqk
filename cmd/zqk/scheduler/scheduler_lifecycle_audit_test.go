package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
)

func TestSchedulerJobLifecycle_NotAggregatedWhenTargetIDSet(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.scheduler.lifecycle",
	})
	testRoot := proj.Root

	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Ensure buffer is ENABLED to verify that target_id prevents aggregation even under enabled buffering
	bufferRegistry := storagepkg.GetGlobalBufferRegistry()
	secCtx := pkgctx.NewSystemSecurityContext()
	buf := bufferRegistry.GetOrCreate(testRoot, secCtx)
	buf.SetEnabled(true)

	jobID := "SCH-TARGET-TEST-001"
	ctx := context.Background()

	// 1. Emit scheduler_job_started with target_id
	optsStarted := &storagepkg.AuditEventOptions{
		EventType:  "scheduler_job_started",
		Operation:  "Scheduler job " + jobID + " started",
		Severity:   "low",
		TargetKind: objects.KindSchedulerJob,
		TargetID:   jobID,
		Metadata: map[string]any{
			"job_id": jobID,
		},
	}
	if err := storagepkg.CreateAuditEventWithBuilder(ctx, testRoot, secCtx, storageProvider, optsStarted); err != nil {
		t.Fatalf("failed to create started audit event: %v", err)
	}

	// 2. Emit scheduler_job_completed with target_id
	optsCompleted := &storagepkg.AuditEventOptions{
		EventType:  "scheduler_job_completed",
		Operation:  "Scheduler job " + jobID + " completed",
		Severity:   "low",
		TargetKind: objects.KindSchedulerJob,
		TargetID:   jobID,
		Metadata: map[string]any{
			"job_id":  jobID,
			"success": true,
		},
	}
	if err := storagepkg.CreateAuditEventWithBuilder(ctx, testRoot, secCtx, storageProvider, optsCompleted); err != nil {
		t.Fatalf("failed to create completed audit event: %v", err)
	}

	// Flush CAS listing queue for project root if CAS is active
	_ = caspkg.FlushKindListingIndexForProjectRootWithTimeout(testRoot, "audit_event", 5*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)

	// 3. List by target_id (with convergence poll)
	storageCtx := pkgctx.NewStorageContext()
	var res *storagepkg.QueryResult
	for i := 0; i < 20; i++ {
		res, err = storageProvider.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
			Kind: "audit_event",
			Filters: map[string]any{
				"target_id": jobID,
			},
		})
		if err == nil && res != nil && len(res.Objects) == 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to list audit events by target_id: %v", err)
	}

	if len(res.Objects) != 2 {
		t.Fatalf("expected exactly 2 audit events for target_id %s, got %d: %+v", jobID, len(res.Objects), res.Objects)
	}

	var hasStarted, hasCompleted bool
	for _, obj := range res.Objects {
		et := objects.GetString(obj, "event_type")
		tid := objects.GetString(obj, "target_id")
		if tid != jobID {
			t.Errorf("expected target_id %s, got %s", jobID, tid)
		}
		if et == "scheduler_job_started" {
			hasStarted = true
		}
		if et == "scheduler_job_completed" {
			hasCompleted = true
		}
	}

	if !hasStarted || !hasCompleted {
		t.Fatalf("expected both started and completed events, got started=%v completed=%v", hasStarted, hasCompleted)
	}
}
