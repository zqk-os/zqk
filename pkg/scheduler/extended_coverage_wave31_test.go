package scheduler

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_MeshLeaseSupervision_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-mesh-lease-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	hIface := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger)
	h := hIface.(*MeshLeaseSupervisionHandler)

	// 1. Helper getFloat testing
	if getFloat(123) != 123.0 {
		t.Errorf("expected 123.0 from int")
	}
	if getFloat(int64(456)) != 456.0 {
		t.Errorf("expected 456.0 from int64")
	}
	if getFloat(float32(7.5)) != 7.5 {
		t.Errorf("expected 7.5 from float32")
	}
	if getFloat(float64(8.5)) != 8.5 {
		t.Errorf("expected 8.5 from float64")
	}
	if getFloat("invalid") != 0.0 {
		t.Errorf("expected 0.0 from string")
	}

	// 2. reapStaleSubprocesses with empty map
	h.reapStaleSubprocesses(map[string]bool{})

	// 3. Execute with no active leases
	job := &ScheduledJob{
		ID:      "SCH-mesh-lease-1",
		JobType: "mesh_lease_supervision",
	}
	_ = h.Execute(ctx, job)

	// 4. Seed a revoked lease and an exhausted lease
	secCtx := pkgctx.NewSystemSecurityContext()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-mesh-lease")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)

	revokedLeaseID := "ZSN-lease-revoked"
	_ = sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 revokedLeaseID,
		objects.FieldKeyKind:               objects.KindZqkSession,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeySessionMode:        "federated_lease",
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyRevokedAt:          time.Now().Format(time.RFC3339),
		objects.FieldKeyTitle:              "Revoked Lease",
		objects.FieldKeyDescription:        "Lease that was revoked",
	})

	exhaustedLeaseID := "ZSN-lease-exhausted"
	_ = sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 exhaustedLeaseID,
		objects.FieldKeyKind:               objects.KindZqkSession,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeySessionMode:        "federated_lease",
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyMaxUnits:           100.0,
		objects.FieldKeyConsumedUnits:      100.0,
		objects.FieldKeyTermType:           "metered",
		objects.FieldKeyTitle:              "Exhausted Lease",
		objects.FieldKeyDescription:        "Lease with exhausted quota",
	})

	// Execute should process the revoked and exhausted leases
	_ = h.Execute(ctx, job)

	// 5. ensureSubprocess with invalid consumer kernel ref
	_ = h.ensureSubprocess(ctx, "lease-1", "NONEXISTENT-KERNEL")
}

func TestExtended_Notifications_DeepCoverage(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ndIface := NewNotificationDisplay(logger)
	nd := ndIface.(*NotificationDisplay)

	// 1. formatDuration helper
	if formatDuration(10*time.Second) != "10s" {
		t.Errorf("expected 10s, got %s", formatDuration(10*time.Second))
	}
	if formatDuration(90*time.Second) != "1m30s" {
		t.Errorf("expected 1m30s, got %s", formatDuration(90*time.Second))
	}
	if formatDuration(3700*time.Second) != "1h1m40s" {
		t.Errorf("expected 1h1m40s, got %s", formatDuration(3700*time.Second))
	}

	// 2. CreateJobNotification
	notif := CreateJobNotification(
		"SCH-job-notif-1",
		"run_wrapper",
		"testing",
		"completed",
		PriorityHigh,
		15*time.Second,
		nil,
		map[string]any{
			KeyCommand: "go test ./...",
		},
	)
	if notif == nil || notif.JobID != "SCH-job-notif-1" {
		t.Fatalf("CreateJobNotification failed")
	}

	// 3. Display methods
	nd.Display(notif)
	nd.DisplayTerminalNotification(notif)
	nd.DisplayDesktopNotification(notif)

	// Notification with error and critical priority
	critNotif := CreateJobNotification(
		"SCH-job-crit",
		"periodic",
		"maintenance",
		"failed",
		PriorityCritical,
		5*time.Second,
		errors.New("fatal command failure"),
		map[string]any{
			"is_test_failure": true,
		},
	)
	nd.Display(critNotif)

	// Notification with low priority
	lowNotif := CreateJobNotification(
		"SCH-job-low",
		"periodic",
		"cleanup",
		"status_update",
		PriorityLow,
		0,
		nil,
		nil,
	)
	nd.Display(lowNotif)

	// 4. CreateCompletionMessage
	job := &ScheduledJob{
		ID:      "SCH-complete-msg",
		JobType: "run_wrapper",
		Title:   "Test Job",
	}
	msg := CreateCompletionMessage(job, 2*time.Second, "stdout preview", "stderr preview", 0)
	_ = msg
}
