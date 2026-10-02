package hostservice_test

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRootIDStableAcrossSamePath(t *testing.T) {
	t.Parallel()
	a := hostservice.RootID("/tmp/proj-a")
	b := hostservice.RootID("/tmp/proj-a")
	if a != b || a == "" {
		t.Fatalf("RootID unstable: %q vs %q", a, b)
	}
	c := hostservice.RootID("/tmp/proj-b")
	if a == c {
		t.Fatal("different roots must not share root_id")
	}
}

func TestRegistryUpsertFindOrphans(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	present := filepath.Join(dir, "alive")
	if err := fileutil.MkdirAll(present, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	reg := &hostservice.Registry{}
	reg.UpsertEntry(hostservice.Entry{
		RootID:       "r1",
		AbsRoot:      missing,
		UnitLabel:    "u1",
		DesiredState: hostservice.DesiredStateEnabled,
	})
	reg.UpsertEntry(hostservice.Entry{
		RootID:       "r2",
		AbsRoot:      present,
		UnitLabel:    "u2",
		DesiredState: hostservice.DesiredStateEnabled,
	})
	e, ok := reg.FindByRootID("r1")
	if !ok || e.AbsRoot != missing {
		t.Fatalf("FindByRootID failed: %+v", e)
	}
	// Orphans uses LoadRegistry from home — unit-test Upsert/Find only here.
	if _, ok := reg.FindByAbsRoot(present); !ok {
		t.Fatal("FindByAbsRoot present")
	}
}

func TestResolveServiceDaemonBinary_RoleDifferentiators(t *testing.T) {
	origExe := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(origExe) })

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := fileutil.MkdirAll(binDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// 1. With standard brand (zqk), create base and role links
	brand.SetExecutableName("zqk")
	zqkBin := filepath.Join(binDir, "zqk")
	if err := fileutil.WriteFile(zqkBin, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}
	zqkSched := filepath.Join(binDir, "zqk-sched")
	if err := fileutil.Symlink(zqkBin, zqkSched); err != nil {
		t.Fatal(err)
	}
	zqkAmb := filepath.Join(binDir, "zqk-amb")
	if err := fileutil.Symlink(zqkBin, zqkAmb); err != nil {
		t.Fatal(err)
	}
	zqkPW := filepath.Join(binDir, "zqk-pw")
	if err := fileutil.Symlink(zqkBin, zqkPW); err != nil {
		t.Fatal(err)
	}
	zqkOverseer := filepath.Join(binDir, "zqk-overseer")
	if err := fileutil.Symlink(zqkBin, zqkOverseer); err != nil {
		t.Fatal(err)
	}

	// Default role should resolve zqk-sched
	if got := hostservice.ResolveServiceDaemonBinary(dir); got != zqkSched {
		t.Fatalf("ResolveServiceDaemonBinary(default) = %q, want %q", got, zqkSched)
	}

	// Explicit roles
	if got := hostservice.ResolveServiceDaemonBinary(dir, "amb"); got != zqkAmb {
		t.Fatalf("ResolveServiceDaemonBinary(amb) = %q, want %q", got, zqkAmb)
	}
	if got := hostservice.ResolveServiceDaemonBinary(dir, "pw"); got != zqkPW {
		t.Fatalf("ResolveServiceDaemonBinary(pw) = %q, want %q", got, zqkPW)
	}
	if got := hostservice.ResolveServiceDaemonBinary(dir, "overseer"); got != zqkOverseer {
		t.Fatalf("ResolveServiceDaemonBinary(overseer) = %q, want %q", got, zqkOverseer)
	}

	// 2. Dynamic brand name: BRAND_NAME=foo
	brand.SetExecutableName("foo")
	fooBin := filepath.Join(binDir, "foo")
	if err := fileutil.WriteFile(fooBin, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}
	fooSched := filepath.Join(binDir, "foo-sched")
	if err := fileutil.Symlink(fooBin, fooSched); err != nil {
		t.Fatal(err)
	}
	fooAmb := filepath.Join(binDir, "foo-amb")
	if err := fileutil.Symlink(fooBin, fooAmb); err != nil {
		t.Fatal(err)
	}
	fooPW := filepath.Join(binDir, "foo-pw")
	if err := fileutil.Symlink(fooBin, fooPW); err != nil {
		t.Fatal(err)
	}

	if got := hostservice.ResolveServiceDaemonBinary(dir); got != fooSched {
		t.Fatalf("ResolveServiceDaemonBinary(foo default) = %q, want %q", got, fooSched)
	}
	if got := hostservice.ResolveServiceDaemonBinary(dir, "amb"); got != fooAmb {
		t.Fatalf("ResolveServiceDaemonBinary(foo amb) = %q, want %q", got, fooAmb)
	}
	if got := hostservice.ResolveServiceDaemonBinary(dir, "pw"); got != fooPW {
		t.Fatalf("ResolveServiceDaemonBinary(foo pw) = %q, want %q", got, fooPW)
	}

	// 3. Auto-ensuring symlink when only base binary exists
	dir2 := t.TempDir()
	binDir2 := filepath.Join(dir2, "bin")
	if err := fileutil.MkdirAll(binDir2, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	fooBin2 := filepath.Join(binDir2, "foo")
	if err := fileutil.WriteFile(fooBin2, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}
	// foo-sched does not exist yet; ResolveServiceDaemonBinary should auto-create and return it
	gotSched := hostservice.ResolveServiceDaemonBinary(dir2)
	expectedSched := filepath.Join(binDir2, "foo-sched")
	if gotSched != expectedSched {
		t.Fatalf("ResolveServiceDaemonBinary auto-ensure = %q, want %q", gotSched, expectedSched)
	}
	if !fileutil.Exists(expectedSched) {
		t.Fatalf("expected symlink %q to be created", expectedSched)
	}
}

func TestEnsureAllServiceRoleSymlinks(t *testing.T) {
	origExe := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(origExe) })

	// Test with BRAND_NAME=foo
	brand.SetExecutableName("foo")
	dir := t.TempDir()

	// Simulate standalone binary existing in an external download location
	extDir := t.TempDir()
	extBin := filepath.Join(extDir, "foo")
	if err := fileutil.WriteFile(extBin, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}

	// Call EnsureAllServiceRoleSymlinks pointing to the standalone binary
	if err := hostservice.EnsureAllServiceRoleSymlinks(dir, extBin); err != nil {
		t.Fatalf("EnsureAllServiceRoleSymlinks failed: %v", err)
	}

	binDir := filepath.Join(dir, "bin")
	expectedLinks := []string{
		"foo",
		"foo-sched",
		"foo-amb",
		"foo-pw",
		"foo-overseer",
		"foo-mcp-ide-adapter",
	}

	for _, name := range expectedLinks {
		linkPath := filepath.Join(binDir, name)
		if !fileutil.Exists(linkPath) {
			t.Errorf("expected link %s to exist in %s", name, binDir)
		}
		fi, err := fileutil.Lstat(linkPath)
		if err != nil {
			t.Errorf("failed to lstat %s: %v", name, err)
			continue
		}
		if fi.Mode()&fileutil.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink", name)
		}
	}
}
