package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutUnder_Err_nilWhenEmptyChain(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if err := LayoutUnder(tmp).Err(); err != nil {
		t.Fatalf("empty chain: %v", err)
	}
}

func TestLayoutUnder_Dir_createsTree(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if err := LayoutUnder(tmp).
		Dir(ProjectDataDir, DirPerm755).
		Dir(ProcessInternalObjectSpecsDir, DirPerm755).
		Err(); err != nil {
		t.Fatal(err)
	}
	specs := filepath.Join(tmp, ProcessInternalObjectSpecsDir)
	if st, err := os.Stat(specs); err != nil || !st.IsDir() {
		t.Fatalf("specs dir missing: %v", err)
	}
}

func TestEnsureProcessAndObjectSpecsLayout(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if err := EnsureProcessAndObjectSpecsLayout(tmp); err != nil {
		t.Fatal(err)
	}
	specs := filepath.Join(tmp, ProcessInternalObjectSpecsDir)
	if st, err := os.Stat(specs); err != nil || !st.IsDir() {
		t.Fatalf("specs dir: %v", err)
	}
	lc := filepath.Join(tmp, ProcessInternalLifecyclesDir)
	if st, err := os.Stat(lc); err != nil || !st.IsDir() {
		t.Fatalf("lifecycles dir: %v", err)
	}
}

func TestLayoutUnder_Dir_stopsOnFirstError(t *testing.T) {
	t.Parallel()
	// Non-existent parent path segment cannot happen with Join; use invalid root by making
	// a file where a directory should be.
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(bad, []byte("x"), FilePerm644); err != nil {
		t.Fatal(err)
	}
	err := LayoutUnder(bad).Dir("child", DirPerm755).Err()
	if err == nil {
		t.Fatal("expected error when root is a file")
	}
}
