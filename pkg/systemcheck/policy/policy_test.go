package policy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDocLinksGate(t *testing.T) {
	tempDir := t.TempDir()

	// Setup fake docs structure
	docsDir := filepath.Join(tempDir, "docs")
	_ = fileutil.MkdirAll(docsDir, paths.DirPerm755)

	quadrants := []string{"tutorials", "howto", "manual", "explanation"}
	for _, q := range quadrants {
		qDir := filepath.Join(docsDir, q)
		_ = fileutil.MkdirAll(qDir, paths.DirPerm755)
		_ = fileutil.WriteFile(filepath.Join(qDir, "guide.md"), []byte("# Guide"), paths.FilePerm644)
	}

	_ = fileutil.WriteFile(filepath.Join(tempDir, "CONTRIBUTING.md"), []byte("# Contrib"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(tempDir, "CODE_OF_CONDUCT.md"), []byte("# COC"), paths.FilePerm644)
	_ = fileutil.MkdirAll(filepath.Join(tempDir, ".github"), paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(tempDir, ".github", "PULL_REQUEST_TEMPLATE.md"), []byte("# PR"), paths.FilePerm644)

	// Valid INDEX.md
	_ = fileutil.WriteFile(filepath.Join(docsDir, "INDEX.md"), []byte(`
[Tutorial](tutorials/guide.md)
[External](https://example.com)
[Anchor](#section)
`), paths.FilePerm644)

	gate := &DocLinksGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected passed, got: %v", res.Violations)
	}

	// Break link
	_ = fileutil.WriteFile(filepath.Join(docsDir, "INDEX.md"), []byte(`
[Broken](nonexistent/file.md)
`), paths.FilePerm644)

	resBroken, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resBroken.Passed {
		t.Fatal("expected failure for broken link, but passed")
	}
}

func TestSecretsGate(t *testing.T) {
	tempDir := t.TempDir()

	cleanFile := filepath.Join(tempDir, "clean.txt")
	_ = fileutil.WriteFile(cleanFile, []byte("regular text with no secrets"), paths.FilePerm644)

	gate := &SecretsGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected clean run to pass, got: %v", res.Violations)
	}

	// Inject secret (constructed dynamically to avoid tripping scanner on test source)
	leakFile := filepath.Join(tempDir, "leak.txt")
	_ = fileutil.WriteFile(leakFile, []byte("api_key = "+"AKIA"+"IOSFODNN7EXAMPLE\n"), paths.FilePerm644)

	resLeak, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resLeak.Passed {
		t.Fatal("expected secret gate to catch leak, but passed")
	}
	if len(resLeak.Violations) == 0 {
		t.Fatal("expected at least 1 violation reported")
	}
}

func TestIsSecretScannerExemptTestFixture(t *testing.T) {
	cases := []struct {
		path   string
		exempt bool
	}{
		{"pkg/systemcheck/policy/secrets_test.go", true},
		{"pkg/auth/policy_test.go", true},
		{"scripts/test-escalation.sh", true},
		{"pkg/systemcheck/policy/secrets.go", false},
		{"cmd/zqk/main.go", false},
		{"README.md", false},
		{"test_secret.txt", false},
	}
	for _, tc := range cases {
		if got := isSecretScannerExemptTestFixture(tc.path); got != tc.exempt {
			t.Errorf("isSecretScannerExemptTestFixture(%q) = %v, want %v", tc.path, got, tc.exempt)
		}
	}
}

func TestStorageBoundariesGate(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "pkg", "storage")
	_ = fileutil.MkdirAll(storageDir, paths.DirPerm755)

	gate := &StorageBoundariesGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected empty storage dir to pass, got: %v", res.Violations)
	}

	// Create subpackage wal importing sibling file
	walDir := filepath.Join(storageDir, "wal")
	_ = fileutil.MkdirAll(walDir, paths.DirPerm755)
	badFile := filepath.Join(walDir, "bad.go")
	_ = fileutil.WriteFile(badFile, []byte(`package wal

import "github.com/zqk-os/zqk/pkg/storage/file"
`), paths.FilePerm644)

	resBad, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resBad.Passed {
		t.Fatal("expected storage boundary violation for wal importing file, but passed")
	}
}

func TestGoroutinesGate(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "myfeature")
	_ = fileutil.MkdirAll(pkgDir, paths.DirPerm755)

	cleanFile := filepath.Join(pkgDir, "clean.go")
	_ = fileutil.WriteFile(cleanFile, []byte(`package myfeature
func Run() {
	// do nothing
}
`), paths.FilePerm644)

	gate := &GoroutinesGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected clean file to pass, got: %v", res.Violations)
	}

	// Bare goroutine
	badFile := filepath.Join(pkgDir, "bad.go")
	badCode := "package myfeature\nfunc Bad() {\n\t" + "go" + " func() {\n\t\t// bare\n\t}()\n}\n"
	_ = fileutil.WriteFile(badFile, []byte(badCode), paths.FilePerm644)

	resBad, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir, Files: []string{badFile}})
	if resBad.Passed {
		t.Fatal("expected goroutines gate to fail on bare go func, but passed")
	}

	// Bare errgroup .Go spawn
	badErrgroupFile := filepath.Join(pkgDir, "bad_errgroup.go")
	badErrgroupCode := "package myfeature\nfunc BadEG(g interface{ Go(func() error) }) {\n\tg.Go(func() error {\n\t\treturn nil\n\t})\n}\n"
	_ = fileutil.WriteFile(badErrgroupFile, []byte(badErrgroupCode), paths.FilePerm644)

	resBadEG, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir, Files: []string{badErrgroupFile}})
	if resBadEG.Passed {
		t.Fatal("expected goroutines gate to fail on unmonitored g.Go, but passed")
	}
}

func TestRunGates(t *testing.T) {
	tempDir := t.TempDir()
	results, passed := RunGates(context.Background(), RunOptions{ProjectRoot: tempDir}, "invalid-gate")
	if passed {
		t.Fatal("expected RunGates with invalid gate to fail")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
