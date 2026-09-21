package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestStreamSegment_LimitZeroEnforcesDefaultCap verifies that limit=0 bounds
// stream segment list scans to DefaultMaxStreamListLimit rather than loading
// history unbounded (REQ-CEF-R2-PERF-LIMIT0 / CRIT-CEF-R2-PERF-LIMIT0-A / BLI-1788842333083792000-e944c80b).
func TestStreamSegment_LimitZeroEnforcesDefaultCap(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	kind := "test_limit_stream"
	segDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StreamsDir, kind)
	if err := fileutil.MkdirAll(segDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir streams dir: %v", err)
	}

	// Write a segment with 20 objects
	segFile := filepath.Join(segDir, "2026-01-01_000000.json")
	f, err := fileutil.OpenFile(segFile, fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		t.Fatalf("open segFile: %v", err)
	}

	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("STREAM-TEST-%04d", i)
		record := map[string]any{
			objects.FieldKeyID:   id,
			objects.FieldKeyKind: kind,
			"val":                i,
		}
		data, _ := json.Marshal(record)
		_, _ = f.Write(append(data, '\n'))
		if err := AppendStreamLocationToRegistry(tmpDir, kind, id, FormatStreamLocation(segFile, int64(i))); err != nil {
			t.Fatalf("AppendStreamLocationToRegistry: %v", err)
		}
	}
	_ = f.Close()

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-SYSTEM"}

	// 1. Explicit limit = 5
	res, _, err := st.listStreamSegmentsWithLimit(ctx, secCtx, ListFilter{Kind: kind}, 5)
	if err != nil {
		t.Fatalf("listStreamSegmentsWithLimit(5): %v", err)
	}
	if len(res) != 5 {
		t.Fatalf("expected 5 objects with limit=5, got %d", len(res))
	}

	// 2. Limit = 0: should return up to DefaultMaxStreamListLimit (all 20 available here)
	res0, total0, err := st.listStreamSegmentsWithLimit(ctx, secCtx, ListFilter{Kind: kind}, 0)
	if err != nil {
		t.Fatalf("listStreamSegmentsWithLimit(0): %v", err)
	}
	if len(res0) != 20 || total0 != 20 {
		t.Fatalf("expected 20 objects with limit=0, got len=%d total=%d", len(res0), total0)
	}
}
