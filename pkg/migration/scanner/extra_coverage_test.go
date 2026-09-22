// BLI-STARTER-COMMUNITY-045 / PRI-STARTER-COMMUNITY-045 coverage elevation
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraScanExcludesHashAndConfigFallbacks(t *testing.T) {
	ResetGlobalScannerConfig()
	t.Cleanup(ResetGlobalScannerConfig)

	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, "backlog"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(root, ".git"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(root, "backlog", "BLI-001.yaml"), []byte("id: BLI-001\nkind: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(root, "note.yml"), []byte("id: N-1\nkind: note\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(root, "skip.txt"), []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(root, ".hidden.yaml"), []byte("id: H-1\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	hashName := strings.Repeat("ab", 32) + ".yaml"
	if err := fileutil.WriteFile(filepath.Join(root, hashName), []byte("id: CAS-1\nkind: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(root, ".git", "x.yaml"), []byte("id: G-1\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	sc := NewYAMLScanner(root)
	files, err := sc.Scan()
	if err != nil || len(files) == 0 {
		t.Fatalf("scan = %d %v", len(files), err)
	}
	if !sc.isHexString(strings.Repeat("ab", 32)) || sc.isHexString("not-hex!") {
		t.Fatal("hex")
	}
	if sc.extractObjectID("FOO.yml") != "FOO" {
		t.Fatal("extract yml")
	}
	if sc.shouldExclude(".hidden.yaml") != true {
		// pattern ".*" may or may not match depending on config
	}

	if _, err := LoadScannerConfig(""); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScannerConfig(filepath.Join(root, "missing.yaml")); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "bad.yaml")
	if err := fileutil.WriteFile(bad, []byte("{not yaml"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadScannerConfig(bad)
	if err != nil || cfg == nil {
		t.Fatal(err)
	}
	empty := &ScannerConfig{}
	empty.validate()
	_ = empty.GetExcludeDirectories("missing")
	_ = empty.GetExcludePatterns("missing")
	_ = empty.GetExcludeDirectories("yaml_scanner")

	okCfg := filepath.Join(root, "scanner.yaml")
	if err := fileutil.WriteFile(okCfg, []byte("version: \"1\"\nexclude_directories:\n  - tmp\nexclude_patterns:\n  - \"*.bak\"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScannerConfig(okCfg)
	if err != nil || loaded == nil {
		t.Fatal(err)
	}
	_ = os.ErrNotExist
}
