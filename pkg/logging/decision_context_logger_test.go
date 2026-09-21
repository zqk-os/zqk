package logging

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestDestinationCacheKey_sameFilePathDedupesLogicalNames(t *testing.T) {
	root := "/project"
	p1 := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, "components", "foo-events.json")
	p2 := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, "components", "bar", "..", "foo-events.json")

	a := &pkgctx.LogDestination{FilePath: p1, Name: "component_foo"}
	b := &pkgctx.LogDestination{FilePath: p2, Name: "component_bar"}

	ka := destinationCacheKey("component_foo", a)
	kb := destinationCacheKey("component_bar", b)
	if ka != kb {
		t.Fatalf("expected same cache key for same normalized path: %q vs %q (paths %q %q)", ka, kb, p1, p2)
	}
}

func TestDedupeWriter(t *testing.T) {
	dcl := &decisionContextLogger{
		destCache:   make(map[string]*destination),
		projectRoot: "/project",
	}

	path := "/project/test.log"
	destConfig := &pkgctx.LogDestination{FilePath: path, Name: "test", Formatter: "json"}

	// Ensure destCache is initialized as in getOrCreateDestination
	dcl.destCache = make(map[string]*destination)

	d1 := dcl.getOrCreateDestination("test1", destConfig)
	d2 := dcl.getOrCreateDestination("test2", destConfig)

	if d1 != d2 {
		t.Fatal("expected same destination object")
	}
}

func TestDestinationCacheKey_nonFileUsesLogicalName(t *testing.T) {
	d := &pkgctx.LogDestination{Writer: "stdout", Formatter: "text"}
	k := destinationCacheKey("stdout", d)
	if k != "stdout" {
		t.Fatalf("got %q want stdout", k)
	}
}
