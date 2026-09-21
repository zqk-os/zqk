package community

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRunExportGate_CleanDir(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(tmp, "README.md"), []byte("# Community"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	res, err := RunExportGate(tmp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected clean pass, got violations: %+v", res.Violations)
	}
	if res.FilesAudited != 1 {
		t.Errorf("expected 1 file audited, got %d", res.FilesAudited)
	}
}

func TestRunExportGate_DetectsProhibitedPattern(t *testing.T) {
	tmp := t.TempDir()
	internalDir := filepath.Join(tmp, "scripts", "zqk-internal")
	if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(internalDir, "daemon.sh"), []byte("echo internal"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	res, err := RunExportGate(tmp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected export gate to fail due to prohibited pattern")
	}
	if len(res.Violations) == 0 {
		t.Fatal("expected at least 1 violation")
	}
	found := false
	for _, v := range res.Violations {
		if v.Rule == "POL-OPENCORE-PROHIBITED-PATH" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected POL-OPENCORE-PROHIBITED-PATH violation, got: %+v", res.Violations)
	}
}

func TestRunExportGate_DetectsProhibitedExtension(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(tmp, "server.key"), []byte("SECRET"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	res, err := RunExportGate(tmp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected export gate to fail due to .key file")
	}
	if len(res.Violations) != 1 || res.Violations[0].Rule != "POL-OPENCORE-PROHIBITED-EXTENSION" {
		t.Fatalf("unexpected violations: %+v", res.Violations)
	}
}

func TestLegacyDecommissionQuarantine(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	legacyDir := filepath.Join(root, "scripts", "legacy-decommission")
	if !fileutil.Exists(legacyDir) {
		t.Fatalf("scripts/legacy-decommission does not exist at %s", legacyDir)
	}
	for _, sub := range []string{"maintenance", "migrators", "process-realignment", "README.md"} {
		p := filepath.Join(legacyDir, sub)
		if !fileutil.Exists(p) {
			t.Errorf("missing expected decommission path: %s", sub)
		}
	}
}
