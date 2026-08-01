package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestDetermineCheckTarget_UsesCacheKindsWhenPopulated is a regression test for the system check pipeline.
//
// Per docs/architecture/system-check-pipeline.md and system-check-performance-targets.md (REQ-200):
// when the object ID cache is populated for the project, kinds MUST come from ObjectIDCache.GetKinds()
// only; do not call discoverObjectKinds(processDir) (field registry LoadFields + GetAllKinds).
//
// This test ensures that when the cache is populated, determineCheckTarget correctly resolves
// the first argument as a kind (so the implementation is using cache kinds). If someone changes
// determineCheckTarget to always call discoverObjectKinds first or to ignore the cache,
// this test may still pass (if field registry returns same kinds), but the test documents the
// requirement; combined with the implementation comment in determineCheckTarget, regressions
// should be caught by code review. For stronger guarantees, consider adding a build-time
// or runtime assertion that discoverObjectKinds is not called when cache is populated.
func TestDetermineCheckTarget_UsesCacheKindsWhenPopulated(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
	tempDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tempDir)

	projectRoot, err := setupSystemTestEnvironmentRoot(tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		// Global ObjectIDCache + CAS/WAL under .zqk: strip docs/process and .zqk so t.TempDir()
		// cleanup does not race "directory not empty" (see TestObjectIDCache_InvalidateAndUpdate).
		resetDir, rerr := os.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:                       projectRoot,
				StripProcessArtifacts:             true,
				DrainGlobalListingIndexQueueFirst: true,
				WALTimeout:                        20 * time.Second,
				ShutdownTimeout:                   20 * time.Second,
			})
			return
		}
		defer os.RemoveAll(resetDir)
		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:                       projectRoot,
			StripProcessArtifacts:             true,
			DrainGlobalListingIndexQueueFirst: true,
			WALTimeout:                        20 * time.Second,
			ShutdownTimeout:                   20 * time.Second,
			TearDownGlobalAuditBuffer:         true,
			SecCtx:                            secCtx,
			AuditBufferResetRoot:              resetDir,
		})
	})

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := os.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "id: CRIT-PIPE\nkind: criteria\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\nstatus: not_started\ntitle: Pipeline\n"
	if err := os.WriteFile(filepath.Join(criteriaDir, "CRIT-PIPE.yaml"), []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Populate the global object ID cache for this project (same as system check does in setupAsyncValidationFunction).
	cache := GetGlobalObjectIDCache()
	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}
	if !cache.IsPopulatedForProject(projectRoot) {
		t.Fatal("cache should be populated for projectRoot after BuildCache")
	}
	cacheKinds := cache.GetKinds()
	if len(cacheKinds) == 0 {
		t.Fatal("cache.GetKinds() should be non-empty after building with criteria dir")
	}
	hasCriteria := false
	for _, k := range cacheKinds {
		if k == "criteria" {
			hasCriteria = true
			break
		}
	}
	if !hasCriteria {
		t.Fatalf("cache.GetKinds() should include \"criteria\", got %v", cacheKinds)
	}

	// Build AsyncCheckContext and call determineCheckTarget with args ["criteria"].
	// When cache is populated, determineCheckTarget must use GetKinds() and thus recognize "criteria" as a kind.
	cmd := &cobra.Command{}
	cmd.SetContext(pkgctx.NewSystemContext())
	ctx := cli.ContextForProjectAndProfile(projectRoot, "system")
	checkCtx := &AsyncCheckContext{
		Cmd:         cmd,
		Ctx:         ctx,
		ProjectRoot: projectRoot,
		Logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	if err := determineCheckTarget(checkCtx, []string{"criteria"}); err != nil {
		t.Fatalf("determineCheckTarget: %v", err)
	}
	if checkCtx.TargetKind != "criteria" {
		t.Errorf("when cache is populated and args=[\"criteria\"], TargetKind want \"criteria\", got %q (pipeline requires using ObjectIDCache.GetKinds() when cache populated)", checkCtx.TargetKind)
	}
	if len(checkCtx.TargetIDs) != 0 {
		t.Errorf("TargetIDs want empty when first arg is kind, got %v", checkCtx.TargetIDs)
	}
}
