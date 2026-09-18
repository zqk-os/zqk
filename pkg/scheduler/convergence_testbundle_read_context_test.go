package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadTestBundleHealthTailLines_AlreadyCanceled(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(root, paths.ProjectDataDir, "logs", "scheduler", "cvs", "test-bundles")); err != nil {
		t.Fatal(err)
	}
	p := TestBundlesHealthFilePath(root)
	if err := fileutil.WriteSecureFile(p, []byte(`{"a":1}`+"\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadTestBundleHealthTailLines(ctx, root, 500)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled before read, got %v", err)
	}
}

func TestReadTestBundleHealthTailLines_CanceledMidScan(t *testing.T) {
	// Uses package-level test hooks; must not run parallel with other hook tests.
	root := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(root, paths.ProjectDataDir, "logs", "scheduler", "cvs", "test-bundles")); err != nil {
		t.Fatal(err)
	}
	p := TestBundlesHealthFilePath(root)
	var b strings.Builder
	for range 70 {
		b.WriteString(`{}` + "\n")
	}
	if err := fileutil.WriteSecureFile(p, []byte(b.String())); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prevHook := testHookAfterHealthJSONLScanLine
	prevYield := healthJSONLScanYieldInterval
	t.Cleanup(func() {
		testHookAfterHealthJSONLScanLine = prevHook
		healthJSONLScanYieldInterval = prevYield
	})

	healthJSONLScanYieldInterval = 64
	testHookAfterHealthJSONLScanLine = func(line int) {
		if line == 64 {
			cancel()
		}
	}

	_, err := ReadTestBundleHealthTailLines(ctx, root, 500)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled mid-scan, got %v", err)
	}
}
