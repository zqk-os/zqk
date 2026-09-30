package vet

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefault(t *testing.T) {
	tempDir := t.TempDir()
	cfg, err := LoadConfig(tempDir, "")
	if err != nil {
		t.Fatalf("unexpected error loading default config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.Hygiene.CheckPaths {
		t.Errorf("expected CheckPaths to be true by default")
	}
	if len(cfg.TreePolice.ForbiddenPaths) == 0 {
		t.Errorf("expected default forbidden paths")
	}
}

func TestCheckTreePolice(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		TreePolice: TreePoliceConfig{
			ForbiddenPaths: []string{"docs/commercial"},
			ForbiddenFiles: []string{"ack.txt"},
		},
	}

	if err := os.MkdirAll(filepath.Join(tempDir, "docs", "commercial"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "ack.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckTreePolice(tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) < 2 {
		t.Errorf("expected at least 2 findings, got %d", len(findings))
	}
}

func TestCheckPayload(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		Payload: PayloadConfig{
			RequiredArtifacts: []string{"README.md", "MISSING.md"},
			ForbiddenArtifacts: []string{"FORBIDDEN.md"},
		},
	}

	if err := os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "FORBIDDEN.md"), []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckPayload(tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) != 2 {
		t.Errorf("expected 2 findings, got %d: %+v", len(findings), findings)
	}
}

func TestCheckPathsAndPerms(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckPaths: true,
			CheckPerms: true,
		},
	}

	badGo := `package test
import "os"
func Bad() {
	_ = os.WriteFile("foo.txt", []byte("hi"), 0644)
	_ = os.Open(".zqk/test")
}
`
	badDir := filepath.Join(tempDir, "pkg", "bad")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "bad.go"), []byte(badGo), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckPathsAndPerms(tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) != 2 {
		t.Errorf("expected 2 findings (magic perm and hardcoded path), got %d: %+v", len(findings), findings)
	}
}

func TestCheckPathsAndPerms_NonCallStringLiterals(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckPaths: true,
			CheckPerms: false,
		},
	}

	badGo := `package test
var (
	myPath = ".zqk"
	mySlice = []string{".zqk/foo", "safe"}
)
type Config struct {
	Dir string
}
var myConfig = Config{Dir: ".zqk/bar"}
`
	badDir := filepath.Join(tempDir, "pkg", "badlit")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "bad.go"), []byte(badGo), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckPathsAndPerms(tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) != 3 {
		t.Errorf("expected 3 findings for non-call path literals, got %d: %+v", len(findings), findings)
	}
}

func TestCheckCLINames(t *testing.T) {
	tempDir := t.TempDir()
	badGo := `package test
func BadCLI() string {
	return "zqk version is required"
}
`
	filePath := filepath.Join(tempDir, "cli_test_file.go")
	if err := os.WriteFile(filePath, []byte(badGo), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckCLINames(tempDir, []string{filePath}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestRunner(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckPaths: false,
			CheckPerms: false,
		},
		Payload: PayloadConfig{
			RequiredArtifacts: []string{"README.md"},
		},
	}
	if err := os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(tempDir, cfg)
	report, err := runner.Run(RunOptions{Suites: []string{"payload"}})
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	if !report.Passed {
		t.Errorf("expected runner report to pass")
	}

	var buf bytes.Buffer
	runner.PrintReport(&buf, report)
	if !bytes.Contains(buf.Bytes(), []byte("RESULT: PASS")) {
		t.Errorf("expected output to contain RESULT: PASS, got:\n%s", buf.String())
	}
}

func TestCheckTreePolice_DocHygiene(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{}

	// 1. Stub file < 50 bytes
	stubDir := filepath.Join(tempDir, "docs", "guide")
	_ = os.MkdirAll(stubDir, 0755)
	_ = os.WriteFile(filepath.Join(stubDir, "stub.md"), []byte("short"), 0644)

	// 2. Forbidden placeholder string (test content)
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	_ = os.MkdirAll(pkgDir, 0755)
	_ = os.WriteFile(filepath.Join(pkgDir, "README.md"), []byte("# Sample Package\n\nThis is test content that must fail."), 0644)

	// 3. Unstructured docs dir inside pkg/
	pkgDocsDir := filepath.Join(tempDir, "pkg", "sample", "docs")
	_ = os.MkdirAll(pkgDocsDir, 0755)
	_ = os.WriteFile(filepath.Join(pkgDocsDir, "readme.md"), []byte("test content"), 0644)

	// 4. Unexpanded template marker
	intDir := filepath.Join(tempDir, "internal", "foo")
	_ = os.MkdirAll(intDir, 0755)
	_ = os.WriteFile(filepath.Join(intDir, "README.md"), []byte("# Foo Internal\n\nTODO_OVERWRITE with real content.\nThis line adds bytes."), 0644)

	findings, err := CheckTreePolice(tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundStub := false
	foundRubbish := false
	foundPkgDocs := false
	foundTemplate := false

	for _, f := range findings {
		switch f.CheckID {
		case "tree/doc-stub":
			foundStub = true
		case "tree/doc-placeholder-rubbish":
			foundRubbish = true
		case "tree/forbidden-pkg-docs-dir":
			foundPkgDocs = true
		case "tree/doc-unexpanded-template":
			foundTemplate = true
		}
	}

	if !foundStub {
		t.Errorf("expected tree/doc-stub finding")
	}
	if !foundRubbish {
		t.Errorf("expected tree/doc-placeholder-rubbish finding")
	}
	if !foundPkgDocs {
		t.Errorf("expected tree/forbidden-pkg-docs-dir finding")
	}
	if !foundTemplate {
		t.Errorf("expected tree/doc-unexpanded-template finding")
	}
}
