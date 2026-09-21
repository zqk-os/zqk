package golang

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

func TestWorktreeBuildCheck_skipsNonGoTree(t *testing.T) {
	t.Parallel()
	if err := (WorktreeBuildCheck{}).Verify(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("non-Go tree must skip, not fail: %v", err)
	}
}

func TestWorktreeBuildCheck_skipsGoModuleWithoutProductCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteGoModule(t, root)
	if err := (WorktreeBuildCheck{}).Verify(context.Background(), root); err != nil {
		t.Fatalf("Go library tree without cmd/%s must skip: %v", brand.CanonicalExecutableToken, err)
	}
}

func TestWorktreeBuildCheck_compilesProductCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "cmd", brand.CanonicalExecutableToken, "main.go"), "package main\n\nfunc main() {}\n")
	if err := (WorktreeBuildCheck{}).Verify(context.Background(), root); err != nil {
		t.Fatalf("product CLI should compile: %v", err)
	}
}

func TestWorktreeBuildCheck_failsBrokenProductCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "cmd", brand.CanonicalExecutableToken, "main.go"), "package main\n\nfunc main() { notASymbol() }\n")
	if err := (WorktreeBuildCheck{}).Verify(context.Background(), root); err == nil {
		t.Fatal("broken product CLI must fail closed")
	}
}

func TestProductCLIPackage_emptyWithoutCmd(t *testing.T) {
	t.Parallel()
	if got := productCLIPackage(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}
