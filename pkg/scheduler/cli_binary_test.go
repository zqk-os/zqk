package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestResolveSchedulerDaemonBinary_prefersStableOverZqk(t *testing.T) {
	root := t.TempDir()
	stable := filepath.Join(root, binDirName, zqkSchedulerBinaryName)
	zqk := filepath.Join(root, binDirName, zqkBinaryName)
	if err := fileutil.EnsureDir(filepath.Dir(stable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(stable, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqk, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.SchedulerDaemonBin().Name(), "")
	t.Setenv(zqkenv.Bin().Name(), "")

	got, err := ResolveSchedulerDaemonBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != stable {
		t.Fatalf("got %q, want stable path %q", got, stable)
	}
}

func TestResolveSchedulerDaemonBinary_schedulerDaemonBinOverride(t *testing.T) {
	root := t.TempDir()
	override := filepath.Join(root, "my-daemon")
	if err := fileutil.WriteFile(override, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, binDirName, zqkSchedulerBinaryName)
	if err := fileutil.EnsureDir(filepath.Dir(stable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(stable, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.SchedulerDaemonBin().Name(), override)
	t.Setenv(zqkenv.Bin().Name(), "")

	got, err := ResolveSchedulerDaemonBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Fatalf("got %q, want override %q", got, override)
	}
}

func TestResolveSchedulerDaemonBinary_zqkBinOverridesStable(t *testing.T) {
	root := t.TempDir()
	stable := filepath.Join(root, binDirName, zqkSchedulerBinaryName)
	zqk := filepath.Join(root, binDirName, "zqk-from-env")
	if err := fileutil.EnsureDir(filepath.Dir(stable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(stable, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqk, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.SchedulerDaemonBin().Name(), "")
	t.Setenv(zqkenv.Bin().Name(), zqk)

	got, err := ResolveSchedulerDaemonBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != zqk {
		t.Fatalf("got %q, want ZQK_BIN %q", got, zqk)
	}
}

func TestResolveSchedulerDaemonBinary_prefersZqkStable(t *testing.T) {
	root := t.TempDir()
	zqkStable := filepath.Join(root, paths.ProjectDataDir, binDirName, brand.ZqkStableName)
	zqkScheduler := filepath.Join(root, binDirName, zqkSchedulerBinaryName)

	if err := fileutil.EnsureDir(filepath.Dir(zqkStable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Dir(zqkScheduler)); err != nil {
		t.Fatal(err)
	}

	if err := fileutil.WriteFile(zqkStable, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqkScheduler, []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.SchedulerDaemonBin().Name(), "")
	t.Setenv(zqkenv.Bin().Name(), "")

	got, err := ResolveSchedulerDaemonBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != zqkStable {
		t.Fatalf("got %q, want stable path %q", got, zqkStable)
	}
}
