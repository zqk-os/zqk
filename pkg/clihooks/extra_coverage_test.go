// BLI-STARTER-COMMUNITY-031 / PRI-STARTER-COMMUNITY-031 coverage elevation
package clihooks

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProfile_HasListGetUnknown(t *testing.T) {
	t.Parallel()
	p := NewProfile(t.TempDir())
	if err := p.Load(); err != nil {
		t.Fatal(err)
	}
	if !p.Has(HookPostCommitScanTests) {
		t.Fatal("has")
	}
	if p.Has("nope") {
		t.Fatal("unknown has")
	}
	if p.Get("nope") != nil {
		t.Fatal("get unknown")
	}
	if len(p.List()) == 0 {
		t.Fatal("list")
	}
	if err := p.SetEnabled("nope", true); err == nil {
		t.Fatal("set unknown")
	}
	if err := p.SetTrayEntry("nope", "x"); err == nil {
		t.Fatal("tray unknown")
	}
	if err := p.SetTrayEntry(HookPostCommitScanTests, "scan"); err != nil {
		t.Fatal(err)
	}
	h := p.Get(HookPostCommitScanTests)
	if h == nil || h.TrayEntry != "scan" {
		t.Fatalf("%+v", h)
	}
	if ProtocolVersion == "" {
		t.Fatal("protocol")
	}
}

func TestProfile_LoadBadJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := NewProfile(root)
	dir := filepath.Dir(p.filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(p.filePath, []byte("{"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(); err == nil {
		t.Fatal("bad json")
	}
}
