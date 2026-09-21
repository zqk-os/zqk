package clihooks

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProfile_defaultsAndRoundTrip(t *testing.T) {
	root := t.TempDir()
	p := NewProfile(root)
	if err := p.Load(); err != nil {
		t.Fatal(err)
	}
	if !p.IsEnabled(HookPostCommitScanTests) {
		t.Fatal("expected post_commit_scan_tests enabled by default")
	}
	if err := p.SetEnabled(HookPostCommitScanTests, false); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	p2 := NewProfile(root)
	if err := p2.Load(); err != nil {
		t.Fatal(err)
	}
	if p2.IsEnabled(HookPostCommitScanTests) {
		t.Fatal("expected disabled after reload")
	}
}

func TestProfile_mergePreservesBuiltinDescription(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	// Partial file: only toggle enabled
	path := filepath.Join(cfgDir, paths.CLIHookProfileFile)
	doc := map[string]any{
		objects.FieldKeyVersion: 1,
		"hooks": []map[string]any{{
			objects.FieldKeyID:      HookPostCommitScanTests,
			objects.FieldKeyEnabled: false,
		}},
	}
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(path, content); err != nil {
		t.Fatal(err)
	}
	p := NewProfile(root)
	if err := p.Load(); err != nil {
		t.Fatal(err)
	}
	h := p.Get(HookPostCommitScanTests)
	if h == nil || h.Description == "" {
		t.Fatalf("expected builtin description merged: %+v", h)
	}
}
