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
