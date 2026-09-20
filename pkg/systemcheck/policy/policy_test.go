package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDocLinksGate(t *testing.T) {
	tempDir := t.TempDir()

	// Setup fake docs structure
	docsDir := filepath.Join(tempDir, "docs")
	_ = os.MkdirAll(docsDir, 0755)

	quadrants := []string{"tutorials", "howto", "manual", "explanation"}
	for _, q := range quadrants {
		qDir := filepath.Join(docsDir, q)
		_ = os.MkdirAll(qDir, 0755)
		_ = os.WriteFile(filepath.Join(qDir, "guide.md"), []byte("# Guide"), 0644)
	}

	_ = os.WriteFile(filepath.Join(tempDir, "CONTRIBUTING.md"), []byte("# Contrib"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "CODE_OF_CONDUCT.md"), []byte("# COC"), 0644)
	_ = os.MkdirAll(filepath.Join(tempDir, ".github"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, ".github", "PULL_REQUEST_TEMPLATE.md"), []byte("# PR"), 0644)

	// Valid INDEX.md
	_ = os.WriteFile(filepath.Join(docsDir, "INDEX.md"), []byte(`
[Tutorial](tutorials/guide.md)
[External](https://example.com)
[Anchor](#section)
`), 0644)

	gate := &DocLinksGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected passed, got: %v", res.Violations)
	}

	// Break link
	_ = os.WriteFile(filepath.Join(docsDir, "INDEX.md"), []byte(`
[Broken](nonexistent/file.md)
`), 0644)

	resBroken, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resBroken.Passed {
		t.Fatal("expected failure for broken link, but passed")
	}
}

func TestSecretsGate(t *testing.T) {
	tempDir := t.TempDir()

	cleanFile := filepath.Join(tempDir, "clean.txt")
	_ = os.WriteFile(cleanFile, []byte("regular text with no secrets"), 0644)

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
	_ = os.WriteFile(leakFile, []byte("api_key = "+"AKIA"+"IOSFODNN7EXAMPLE\n"), 0644)

	resLeak, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resLeak.Passed {
		t.Fatal("expected secret gate to catch leak, but passed")
	}
	if len(resLeak.Violations) == 0 {
		t.Fatal("expected at least 1 violation reported")
	}
}

func TestStorageBoundariesGate(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "pkg", "storage")
	_ = os.MkdirAll(storageDir, 0755)

	gate := &StorageBoundariesGate{}
	res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected empty storage dir to pass, got: %v", res.Violations)
	}

	// Create subpackage core importing sibling file
	coreDir := filepath.Join(storageDir, "core")
	_ = os.MkdirAll(coreDir, 0755)
	badFile := filepath.Join(coreDir, "bad.go")
	_ = os.WriteFile(badFile, []byte(`package core

import "github.com/zqk-os/zqk/pkg/storage/file"
`), 0644)

	resBad, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
	if resBad.Passed {
		t.Fatal("expected storage boundary violation for core importing file, but passed")
	}
}

func TestGoroutinesGate(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "myfeature")
	_ = os.MkdirAll(pkgDir, 0755)

	cleanFile := filepath.Join(pkgDir, "clean.go")
	_ = os.WriteFile(cleanFile, []byte(`package myfeature
func Run() {
	// do nothing
}
`), 0644)

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
	_ = os.WriteFile(badFile, []byte(badCode), 0644)

	resBad, _ := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir, Files: []string{badFile}})
	if resBad.Passed {
		t.Fatal("expected goroutines gate to fail on bare go func, but passed")
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
