package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestResolveSchedulerDaemonBinary_prefersStableOverZqk(t *testing.T) {
	root := t.TempDir()
	stable := filepath.Join(root, binDirName, zqkSchedulerBinaryName)
	zqk := filepath.Join(root, binDirName, zqkBinaryName)
	if err := fileutil.EnsureDir(filepath.Dir(stable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(stable, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqk, []byte{0}, paths.DirPerm755); err != nil {
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
	if err := fileutil.WriteFile(override, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, binDirName, zqkSchedulerBinaryName)
	if err := fileutil.EnsureDir(filepath.Dir(stable)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(stable, []byte{0}, paths.DirPerm755); err != nil {
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
	if err := fileutil.WriteFile(stable, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqk, []byte{0}, paths.DirPerm755); err != nil {
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

	if err := fileutil.WriteFile(zqkStable, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zqkScheduler, []byte{0}, paths.DirPerm755); err != nil {
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

func TestResolveSchedulerCLIBinary_communityEdition(t *testing.T) {
	orig := zqkenv.IsCommunityEdition
	t.Cleanup(func() { zqkenv.IsCommunityEdition = orig })
	zqkenv.IsCommunityEdition = true

	root := t.TempDir()
	zcom := filepath.Join(root, binDirName, "zcom")
	if err := fileutil.EnsureDir(filepath.Dir(zcom)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zcom, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.Bin().Name(), "")

	got := resolveSchedulerCLIBinary(root)
	if got != zcom {
		t.Fatalf("got %q, want community binary %q", got, zcom)
	}

	// Test fallback when no root binary exists
	emptyRoot := t.TempDir()
	fallback := resolveSchedulerCLIBinary(emptyRoot)
	if fallback != "zcom" {
		t.Fatalf("got fallback %q, want 'zcom'", fallback)
	}
}

func TestResolveSchedulerDaemonBinary_communityEdition(t *testing.T) {
	orig := zqkenv.IsCommunityEdition
	t.Cleanup(func() { zqkenv.IsCommunityEdition = orig })
	zqkenv.IsCommunityEdition = true

	root := t.TempDir()
	zcom := filepath.Join(root, binDirName, "zcom")
	if err := fileutil.EnsureDir(filepath.Dir(zcom)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(zcom, []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.SchedulerDaemonBin().Name(), "")
	t.Setenv(zqkenv.Bin().Name(), "")

	got, err := ResolveSchedulerDaemonBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != zcom {
		t.Fatalf("got daemon path %q, want %q", got, zcom)
	}
}

func TestResolveSchedulerCLIBinary_EmptyRoot(t *testing.T) {
	got := resolveSchedulerCLIBinary("")
	if got == "" {
		t.Fatal("expected non-empty binary fallback")
	}
}
