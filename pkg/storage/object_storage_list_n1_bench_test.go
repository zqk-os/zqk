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

// BenchmarkObjectStorage_ListPagination verifies that List with limit
// leverages bounded index reads rather than unbounded N+1 scans
// (REQ-CEF-R2-PERF-LIST-N1 / CRIT-CEF-R2-PERF-LIST-N1-A).
func BenchmarkObjectStorage_ListPagination(b *testing.B) {
	tmpDir := b.TempDir()
	b.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	b.Cleanup(func() {
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil))
	})

	MustEnsureProcessSpecsLayoutForTest(&testing.T{}, tmpDir)
	BuildPathAliasCacheForProject(tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		b.Fatalf("NewFileObjectStorage: %v", err)
	}

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}

	kind := "test_bench_kind"
	segDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StreamsDir, kind)
	_ = fileutil.MkdirAll(segDir, paths.DirPerm755)

	segFile := filepath.Join(segDir, "2026-01-01_000000.json")
	f, _ := fileutil.OpenFile(segFile, fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("STREAM-BENCH-%04d", i)
		record := map[string]any{
			objects.FieldKeyID:   id,
			objects.FieldKeyKind: kind,
			"val":                i,
		}
		data, _ := json.Marshal(record)
		_ , _ = f.Write(append(data, '\n'))
		_ = AppendStreamLocationToRegistry(tmpDir, kind, id, FormatStreamLocation(segFile, int64(i)))
	}
	_ = f.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, total, err := st.listStreamSegmentsWithLimit(ctx, secCtx, ListFilter{Kind: kind}, 10)
		if err != nil {
			b.Fatalf("listStreamSegmentsWithLimit failed: %v", err)
		}
		if len(res) != 10 || total != 500 {
			b.Fatalf("expected 10 objects, got len=%d total=%d", len(res), total)
		}
	}
}

// TestObjectStorage_ListBoundedReads verifies bounded read complexity for pagination.
func TestObjectStorage_ListBoundedReads(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil))
	})

	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	BuildPathAliasCacheForProject(tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}
	sctx := pkgctx.NewStorageContext()

	kind := "test_bounded_kind"
	segDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StreamsDir, kind)
	_ = fileutil.MkdirAll(segDir, paths.DirPerm755)

	segFile := filepath.Join(segDir, "2026-01-01_000000.json")
	f, _ := fileutil.OpenFile(segFile, fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("STREAM-BOUNDED-%04d", i)
		record := map[string]any{
			objects.FieldKeyID:   id,
			objects.FieldKeyKind: kind,
			"val":                i,
		}
		data, _ := json.Marshal(record)
		_ , _ = f.Write(append(data, '\n'))
		_ = AppendStreamLocationToRegistry(tmpDir, kind, id, FormatStreamLocation(segFile, int64(i)))
	}
	_ = f.Close()

	// List with Limit 5
	res, total, err := st.listStreamSegmentsWithLimit(ctx, secCtx, ListFilter{Kind: kind}, 5)
	if err != nil {
		t.Fatalf("listStreamSegmentsWithLimit: %v", err)
	}
	if len(res) != 5 || total != 50 {
		t.Fatalf("expected 5 objects out of 50 total, got len=%d total=%d", len(res), total)
	}

	// Verify listStreamBackedOnly
	queryRes, err := st.listStreamBackedOnly(ctx, secCtx, sctx, ListFilter{Kind: kind}, 5, false, nil)
	if err != nil {
		t.Fatalf("listStreamBackedOnly: %v", err)
	}
	if len(queryRes.Objects) != 5 {
		t.Fatalf("expected 5 objects in QueryResult, got %d", len(queryRes.Objects))
	}
}
