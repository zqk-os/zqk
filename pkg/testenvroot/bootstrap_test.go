package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestBootstrapRootCopiesLifecycles(t *testing.T) {
	t.Parallel()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mod := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(mod, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(mod)
		if parent == mod {
			t.Fatal("go.mod not found walking up from package dir")
		}
		mod = parent
	}
	src := filepath.Join(mod, paths.ProcessInternalLifecyclesDir, "backlog_item_lifecycle.yaml")
	if _, err := fileutil.Stat(src); err != nil {
		t.Skipf("module lifecycles not present: %v", err)
	}
	root := t.TempDir()
	if err := BootstrapRoot(root, mod); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	dst := filepath.Join(root, paths.ProcessInternalLifecyclesDir, "backlog_item_lifecycle.yaml")
	if _, err := fileutil.Stat(dst); err != nil {
		t.Fatalf("expected lifecycle copy at %s: %v", dst, err)
	}
	spec := filepath.Join(root, paths.ProcessInternalObjectSpecsDir, "backlog_item.yaml")
	if _, err := fileutil.Stat(spec); err != nil {
		t.Fatalf("expected spec copy at %s: %v", spec, err)
	}
	cfg := filepath.Join(root, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	if _, err := fileutil.Stat(cfg); err != nil {
		t.Fatalf("expected config copy at %s: %v", cfg, err)
	}
	trait := filepath.Join(root, paths.ProcessInternalTraitsDir, "completable.yaml")
	if _, err := fileutil.Stat(trait); err != nil {
		t.Fatalf("expected trait copy at %s: %v", trait, err)
	}
}
