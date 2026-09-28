package objects

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraSpecRoots(t *testing.T) {
	dummy := filepath.Join(t.TempDir(), "dummy_specs")
	AddSpecRoot(dummy)
	t.Cleanup(func() {
		RemoveSpecRoot(dummy)
	})

	roots := ExtraSpecRoots()
	var found bool
	for _, r := range roots {
		if r == filepath.Clean(dummy) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dummy root %s in %v", dummy, roots)
	}

	RemoveSpecRoot(dummy)
	rootsAfter := ExtraSpecRoots()
	for _, r := range rootsAfter {
		if r == filepath.Clean(dummy) {
			t.Fatalf("expected dummy root %s removed from %v", dummy, rootsAfter)
		}
	}
}

func TestExtraLifecycleRoots(t *testing.T) {
	dummy := filepath.Join(t.TempDir(), "dummy_lifecycles")
	AddLifecycleRoot(dummy)
	t.Cleanup(func() {
		RemoveLifecycleRoot(dummy)
	})

	roots := ExtraLifecycleRoots()
	var found bool
	for _, r := range roots {
		if r == filepath.Clean(dummy) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dummy lifecycle root %s in %v", dummy, roots)
	}

	RemoveLifecycleRoot(dummy)
	rootsAfter := ExtraLifecycleRoots()
	for _, r := range rootsAfter {
		if r == filepath.Clean(dummy) {
			t.Fatalf("expected dummy lifecycle root %s removed from %v", dummy, rootsAfter)
		}
	}
}

func TestFindRegisteredSpecFile_packRootThenKernelWins(t *testing.T) {
	specs := filepath.Join(t.TempDir(), "objects")
	if err := fileutil.EnsureDir(filepath.Join(specs, "kernel")); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(t.TempDir(), "pack-specs")
	if err := fileutil.EnsureDir(pack); err != nil {
		t.Fatal(err)
	}
	packRole := filepath.Join(pack, "role.yaml")
	if err := fileutil.WriteStandardFile(packRole, []byte("kind: role\n")); err != nil {
		t.Fatal(err)
	}
	AddSpecRoot(pack)
	t.Cleanup(func() { RemoveSpecRoot(pack) })

	got, err := FindRegisteredSpecFile(specs, "role")
	if err != nil {
		t.Fatal(err)
	}
	if got != packRole {
		t.Fatalf("got %q want pack file %q", got, packRole)
	}

	kernelRole := filepath.Join(specs, "kernel", "role.yaml")
	if err := fileutil.WriteStandardFile(kernelRole, []byte("kind: role\n")); err != nil {
		t.Fatal(err)
	}
	got, err = FindRegisteredSpecFile(specs, "role")
	if err != nil {
		t.Fatal(err)
	}
	if got != kernelRole {
		t.Fatalf("got %q want kernel file %q", got, kernelRole)
	}
}
