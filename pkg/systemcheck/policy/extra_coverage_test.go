package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestFieldKeysGate_Coverage(t *testing.T) {
	gate := &FieldKeysGate{}
	if gate.Name() != "field-keys" {
		t.Errorf("got name %q, want field-keys", gate.Name())
	}
	if gate.Description() == "" {
		t.Error("expected non-empty description")
	}

	tempDir := t.TempDir()
	ctx := context.Background()

	// 1. Missing field_keys.go
	res, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected pass when field_keys.go is missing")
	}

	// 2. field_keys.go with no keys
	objectsDir := filepath.Join(tempDir, "pkg", "objects")
	if err := os.MkdirAll(objectsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	fkFile := filepath.Join(objectsDir, "field_keys.go")
	if err := os.WriteFile(fkFile, []byte("// no keys here\npackage objects\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	res, err = gate.Run(ctx, RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected pass when no keys extracted")
	}

	// 3. field_keys.go with keys, but no filesToScan
	validFK := `package objects
const (
	FieldKeyTitle = "title"
	FieldKeyStatus = "status"
)
`
	if err := os.WriteFile(fkFile, []byte(validFK), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	res, err = gate.Run(ctx, RunOptions{ProjectRoot: tempDir, Files: []string{}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected pass when no scoped files")
	}

	// 4. Scoped files with violations and non-violations
	cleanFile := filepath.Join(tempDir, "clean.go")
	cleanContent := `package main
// comment "title"
func Foo() {
	var jsonTag = ` + "`" + `json:"title"` + "`" + `
	_ = jsonTag
	_ = objects.FieldKeyTitle
}
`
	if err := os.WriteFile(cleanFile, []byte(cleanContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	resClean, err := gate.Run(ctx, RunOptions{
		ProjectRoot: tempDir,
		Files:       []string{cleanFile, "skip.txt", "pkg/objects/field_keys.go", "foo_test.go"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !resClean.Passed {
		t.Errorf("expected clean file to pass, got: %v", resClean.Violations)
	}

	// 5. Scoped file with a raw literal violation
	badFile := filepath.Join(tempDir, "bad.go")
	badContent := `package main
func Bad() {
	m := map[string]any{}
	m["title"] = "hello"
}
`
	if err := os.WriteFile(badFile, []byte(badContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	resBad, err := gate.Run(ctx, RunOptions{
		ProjectRoot: tempDir,
		Files:       []string{badFile},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resBad.Passed {
		t.Error("expected failure for raw literal violation")
	}
	if len(resBad.Violations) == 0 {
		t.Error("expected at least 1 violation")
	}
}

func TestPolicyGates_Descriptions_And_Redaction(t *testing.T) {
	docGate := &DocLinksGate{}
	if docGate.Description() == "" {
		t.Error("expected non-empty description for DocLinksGate")
	}

	goroGate := &GoroutinesGate{}
	if goroGate.Description() == "" {
		t.Error("expected non-empty description for GoroutinesGate")
	}

	secGate := &SecretsGate{}
	if secGate.Description() == "" {
		t.Error("expected non-empty description for SecretsGate")
	}

	storageGate := &StorageBoundariesGate{}
	if storageGate.Description() == "" {
		t.Error("expected non-empty description for StorageBoundariesGate")
	}

	// Test redactSecret
	if redactSecret("short") != "*****" {
		t.Errorf("got %q, want *****", redactSecret("short"))
	}
	longSec := "supersecretlongvalue"
	redacted := redactSecret(longSec)
	if redacted == longSec || len(redacted) == 0 {
		t.Errorf("unexpected redaction: %q", redacted)
	}

	// Test AvailableGates
	avail := AvailableGates()
	if len(avail) < 4 {
		t.Errorf("expected at least 4 available gates, got %d", len(avail))
	}
}

func TestGoroutinesGate_ExtendedPatterns(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// 1. Test getStagedGoFiles fallback
	staged := getStagedGoFiles(tempDir)
	if staged != nil {
		t.Errorf("expected nil for empty tempdir git index")
	}

	// 2. Create complex Go file with comments, command strings, nolint
	pkgDir := filepath.Join(tempDir, "pkg", "worker")
	if err := os.MkdirAll(pkgDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	code := `package worker

/*
go func() {
	// in block comment
}()
*/

// go func() { in line comment }()

func Run() {
	go install pkg
	go test ./...
	go func() { // nolint:goroutine
		// allowed with nolint
	}()
	go func() {
		// bare goroutine
	}()
}
`
	srcFile := filepath.Join(pkgDir, "worker.go")
	if err := os.WriteFile(srcFile, []byte(code), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	gate := &GoroutinesGate{}

	// Run with files explicitly (produces violation)
	res, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir, Files: []string{srcFile}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Error("expected violation for bare goroutine")
	}

	// Run repo-wide without files or staged (produces warnings, passed=true)
	resRepo, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resRepo.Passed {
		t.Error("expected repo-wide scan to treat existing goroutines as warnings")
	}
	if len(resRepo.Warnings) == 0 {
		t.Error("expected warnings for repo-wide scan")
	}

	// Run with Scope == "staged"
	resStaged, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir, Scope: "staged"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resStaged.Passed {
		t.Error("expected staged run on empty git index to pass")
	}
}

func TestStorageBoundariesGate_ExtendedRules(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Missing pkg/storage
	gate := &StorageBoundariesGate{}
	res, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Error("expected pass when pkg/storage missing")
	}

	// Create pkg/storage subpackages
	walDir := filepath.Join(tempDir, "pkg", "storage", "wal")
	fileDir := filepath.Join(tempDir, "pkg", "storage", "file")
	graphDir := filepath.Join(tempDir, "pkg", "storage", "graph")
	extraDir := filepath.Join(tempDir, "pkg", "storage", "extra")
	for _, d := range []string{walDir, fileDir, graphDir, extraDir} {
		if err := os.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}

	// extra imports root pkg/storage
	_ = os.WriteFile(filepath.Join(extraDir, "extra.go"), []byte(`package extra
import "github.com/zqk-os/zqk/pkg/storage"
`), paths.FilePerm644)

	// wal imports file
	_ = os.WriteFile(filepath.Join(walDir, "wal.go"), []byte(`package wal
import "github.com/zqk-os/zqk/pkg/storage/file"
`), paths.FilePerm644)

	// file imports graph
	_ = os.WriteFile(filepath.Join(fileDir, "file.go"), []byte(`package file
import "github.com/zqk-os/zqk/pkg/storage/graph"
`), paths.FilePerm644)

	// graph imports file
	_ = os.WriteFile(filepath.Join(graphDir, "graph.go"), []byte(`package graph
import "github.com/zqk-os/zqk/pkg/storage/file"
`), paths.FilePerm644)

	resViolations, err := gate.Run(ctx, RunOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resViolations.Passed {
		t.Error("expected storage boundary violations")
	}
	if len(resViolations.Violations) < 4 {
		t.Errorf("expected at least 4 violations, got %d", len(resViolations.Violations))
	}
}

func TestRunGates_MultipleGates(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Run with valid gate names
	results, passed := RunGates(ctx, RunOptions{ProjectRoot: tempDir}, "secrets", "field-keys")
	if !passed {
		t.Errorf("expected clean tempdir to pass secrets and field-keys gates")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}
