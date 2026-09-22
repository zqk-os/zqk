// BLI-STARTER-COMMUNITY-040 / PRI-STARTER-COMMUNITY-040 coverage elevation
package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraBootstrapAndGuard(t *testing.T) {
	root, err := Setup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := BootstrapRoot(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(src, paths.ProcessInternalObjectSpecsDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(src, paths.ProcessInternalObjectSpecsDir, "n.yml"), []byte("kind: x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(src, paths.ProcessInternalDir, "spec_index.json"), []byte("{}"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := BootstrapRoot(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := CopyObjectSpecsFromProject(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := CopyLifecyclesFromProject(dst, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := CopyTraitsFromProject(dst, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := CopyConfigsFromProject(dst, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := CopySpecIndexFromProject(dst, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	snap, err := SnapshotRepoState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.VerifyNoMutations(); err != nil {
		t.Fatal(err)
	}
	if err := (*GuardStateSnapshot)(nil).VerifyNoMutations(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNoRepoStateMutation(filepath.Join(root, "x.txt"), root, root); err != nil && err.Error() == "" {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(root, "f.txt"), []byte("ok"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := MkdirAll(filepath.Join(root, "d"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	proc := filepath.Join(root, paths.ProcessDir, "goals")
	if err := fileutil.WriteFile(filepath.Join(proc, "touched.yaml"), []byte("id: x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := snap.VerifyNoMutations(); err == nil {
		t.Fatal("expected mutation")
	}
}

func TestExtraLinkOrCopyYAML(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.yaml")
	dst := filepath.Join(t.TempDir(), "b.yaml")
	if err := fileutil.WriteFile(src, []byte("k: v\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopyYAML(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopyYAML(filepath.Join(t.TempDir(), "missing.yaml"), dst); err == nil {
		t.Fatal("expected missing source")
	}
}
