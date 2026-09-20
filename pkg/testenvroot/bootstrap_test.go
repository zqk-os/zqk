package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	src := filepath.Join(mod, paths.ProcessInternalLifecyclesDir, "pm", "backlog_item_lifecycle.yaml")
	if _, err := fileutil.Stat(src); err != nil {
		t.Skipf("module lifecycles not present: %v", err)
	}
	root := t.TempDir()
	if err := BootstrapRoot(root, mod); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	dst := filepath.Join(root, paths.ProcessInternalLifecyclesDir, "pm", "backlog_item_lifecycle.yaml")
	if _, err := fileutil.Stat(dst); err != nil {
		t.Fatalf("expected lifecycle copy at %s: %v", dst, err)
	}
	spec := filepath.Join(root, paths.ProcessInternalObjectSpecsDir, "pm", "backlog_item.yaml")
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

func TestLinkOrCopyYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.yaml")
	dst := filepath.Join(tmpDir, "target.yaml")

	content := []byte("key: value\n")
	if err := fileutil.WriteFile(src, content, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := linkOrCopyYAML(src, dst); err != nil {
		t.Fatalf("linkOrCopyYAML failed: %v", err)
	}

	got, err := fileutil.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile dst failed: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", string(got), string(content))
	}
}

