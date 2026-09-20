package ambient

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestNewWaveCmd(t *testing.T) {
	cmd := newWaveCmd()
	if cmd == nil {
		t.Fatal("expected wave cmd, got nil")
	}
	if cmd.Use != "wave" {
		t.Errorf("expected use 'wave', got %q", cmd.Use)
	}
}

func TestRunWaveLogic(t *testing.T) {
	t.Parallel()
	env := object.SetupTestEnvironment(t)
	tmpDir := env.TestRoot
	cliBinary := env.CLIBinary

	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	// 1. Create a command_metric
	if err := storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "CMD-1",
		objects.FieldKeyKind:         "command_metric",
		objects.FieldKeyFailureCount: 2,
	}); err != nil {
		t.Fatalf("failed to create cmd metric: %v", err)
	}

	// 2. Create scheduler_health_metric
	if err := storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "SHM-1",
		objects.FieldKeyKind:   "scheduler_health_metric",
		objects.FieldKeyStatus: objects.ObjectStatusError,
	}); err != nil {
		t.Fatalf("failed to create scheduler metric: %v", err)
	}

	// 3. Create audit_aggregation_metric
	if err := storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:   "AAM-1",
		objects.FieldKeyKind: "audit_aggregation_metric",
	}); err != nil {
		t.Fatalf("failed to create audit metric: %v", err)
	}

	// Create system-check summary (for KernelAmbience)
	sysCheckPath := filepath.Join(tmpDir, paths.ProjectDataDir, "logs", "system-check.json")
	_ = fileutil.MkdirAll(filepath.Dir(sysCheckPath), 0755)
	_ = fileutil.WriteFile(sysCheckPath, []byte(`{
		"summary": {
			"total_objects": 100,
			"total_issues": 5,
			"blocking_issues": 3,
			"ghost_ref_count": 4
		}
	}`), 0644)

	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("command_metric", 2*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_health_metric", 2*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_aggregation_metric", 2*time.Second)
	_ = storageProvider.Shutdown(context.Background())

	// Run ambient wave
	cmd := execwrap.Command(cliBinary, "ambient", "wave")
	cmd.Dir = tmpDir
	cmd.Env = object.EnvWithTestRoot(tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ambient wave command failed: %v\nOutput: %s", err, string(output))
	}

	// Verify metrics-rollup.json
	rollupPath := filepath.Join(tmpDir, paths.ProjectDataDir, "state", "ambient", "metrics-rollup.json")
	b, err := fileutil.ReadFile(rollupPath)
	if err != nil {
		t.Fatalf("failed to read rollup: %v", err)
	}

	var rollup whatsnext.MetricsRollupSnapshot
	if err := json.Unmarshal(b, &rollup); err != nil {
		t.Fatalf("failed to parse rollup: %v", err)
	}

	if rollup.CommandErrors != 2 {
		t.Errorf("expected 2 command errors, got %d", rollup.CommandErrors)
	}
	if rollup.SchedulerStuckCount != 1 {
		t.Errorf("expected 1 scheduler stuck, got %d", rollup.SchedulerStuckCount)
	}
	if rollup.AuditEventsCount != 1 {
		t.Errorf("expected 1 audit event, got %d", rollup.AuditEventsCount)
	}
	if rollup.GhostRefCount != 4 {
		t.Errorf("expected 4 ghost refs, got %d", rollup.GhostRefCount)
	}
	if rollup.SystemCheckIssues != 5 {
		t.Errorf("expected 5 system check issues, got %d", rollup.SystemCheckIssues)
	}

	if rollup.NextAdminAction == "" {
		t.Error("expected NextAdminAction to be populated")
	}

	// Human-log clusters (optional fixture)
	logDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
	_ = fileutil.MkdirAll(logDir, 0755)
	_ = fileutil.WriteFile(filepath.Join(logDir, paths.LogEventsPrefix+"human.log"), []byte(
		"2026-08-13T04:17:33Z [error] Ambient probe failed code=1\n"+
			"2026-08-13T04:17:34Z [error] Ambient probe failed code=2\n"+
			"2026-08-13T04:17:35Z [warn] Disk pressure high pct=91\n",
	), 0644)

	cmd2 := execwrap.Command(cliBinary, "ambient", "wave")
	cmd2.Dir = tmpDir
	cmd2.Env = object.EnvWithTestRoot(tmpDir)
	if out2, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("ambient wave (with human log) failed: %v\nOutput: %s", err, string(out2))
	}
	b2, err := fileutil.ReadFile(rollupPath)
	if err != nil {
		t.Fatalf("failed to re-read rollup: %v", err)
	}
	var rollup2 whatsnext.MetricsRollupSnapshot
	if err := json.Unmarshal(b2, &rollup2); err != nil {
		t.Fatalf("failed to parse rollup2: %v", err)
	}
	if len(rollup2.TopErrorClusters) == 0 || rollup2.TopErrorClusters[0].Message != "Ambient probe failed" {
		t.Fatalf("expected top_error_clusters Ambient probe failed, got %#v", rollup2.TopErrorClusters)
	}
	joined := strings.Join(rollup2.RankedActions, " | ")
	if !strings.Contains(joined, "triage-log-errors") {
		t.Fatalf("ranked_actions missing triage-log-errors: %s", joined)
	}
}
