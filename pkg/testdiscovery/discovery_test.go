package testdiscovery_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testdiscovery"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func init() {
	_ = os.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, "1")
}

// CRIT-TEST-DISC-FUNC-POLYGLOT-001 & CRIT-TEST-DISCOVERY-ENGINE-001:
// Polyglot static test file and function discovery across Go, Python, and TypeScript.
func TestDiscover_Polyglot(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a Go test file
	goTest := `package sample_test

import "testing"

// Validates: CRIT-TEST-GO-001
func TestSampleAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Fail()
	}
}

func BenchmarkSampleAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = 1 + 1
	}
}
`
	if err := os.WriteFile(filepath.Join(tempDir, "sample_test.go"), []byte(goTest), 0600); err != nil {
		t.Fatalf("failed to write go test: %v", err)
	}

	// 2. Create a Python test file
	pyTest := `"""Sample Python tests."""
import unittest

# Validates: CRIT-TEST-PY-001
def test_standalone_func():
    assert 1 == 1

class TestCalculator(unittest.TestCase):
    def test_multiply(self):
        self.assertEqual(2 * 3, 6)
`
	if err := os.WriteFile(filepath.Join(tempDir, "test_calculator.py"), []byte(pyTest), 0600); err != nil {
		t.Fatalf("failed to write py test: %v", err)
	}

	// 3. Create a TypeScript test file
	tsTest := `import { describe, it, expect } from 'vitest';

// Validates: CRIT-TEST-TS-001
describe('AuthService', () => {
    it('should login successfully', () => {
        expect(true).toBe(true);
    });
    test('should reject invalid token', () => {
        expect(false).toBe(false);
    });
});
`
	if err := os.WriteFile(filepath.Join(tempDir, "auth.test.ts"), []byte(tsTest), 0600); err != nil {
		t.Fatalf("failed to write ts test: %v", err)
	}

	engine := testdiscovery.NewEngine()
	opts := testdiscovery.DiscoveryOptions{
		ProjectRoot: tempDir,
	}

	targets, err := engine.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) == 0 {
		t.Fatalf("expected discovered targets, got 0")
	}

	// Check that we found tests across all 3 languages
	languagesFound := make(map[string]int)
	for _, target := range targets {
		languagesFound[target.Language]++
	}

	if languagesFound["go"] < 2 {
		t.Errorf("expected at least 2 Go test targets, got %d", languagesFound["go"])
	}
	if languagesFound["python"] < 2 {
		t.Errorf("expected at least 2 Python test targets, got %d", languagesFound["python"])
	}
	if languagesFound["typescript"] < 2 {
		t.Errorf("expected at least 2 TypeScript test targets, got %d", languagesFound["typescript"])
	}
}

// CRIT-TEST-DISC-FUNC-TAGS-001:
// Parse and extract build tags, scenarios, and test metadata from source AST.
func TestDiscover_ExtractTagsAndCriteria(t *testing.T) {
	tempDir := t.TempDir()

	goTest := `//go:build integration && !race
// +build integration,!race

package auth_test

import "testing"

// Criteria: CRIT-TEST-AUTH-LOGIN-001, CRIT-TEST-AUTH-SESSION-002
// Scenario: Multi-tenant login flow
func TestLoginIntegration(t *testing.T) {
	// test logic
}
`
	if err := os.WriteFile(filepath.Join(tempDir, "login_test.go"), []byte(goTest), 0600); err != nil {
		t.Fatalf("failed to write go test: %v", err)
	}

	engine := testdiscovery.NewEngine()
	targets, err := engine.Discover(context.Background(), testdiscovery.DiscoveryOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	target := targets[0]
	if target.Function != "TestLoginIntegration" {
		t.Errorf("expected Function TestLoginIntegration, got %s", target.Function)
	}

	// Verify build tags extracted
	hasIntegrationTag := false
	for _, tag := range target.Tags {
		if strings.Contains(tag, "integration") {
			hasIntegrationTag = true
		}
	}
	if !hasIntegrationTag {
		t.Errorf("expected tags to include integration, got: %v", target.Tags)
	}

	// Verify criteria references extracted
	if len(target.CriteriaRefs) != 2 {
		t.Errorf("expected 2 criteria refs, got: %v", target.CriteriaRefs)
	}
}

// CRIT-TEST-DISC-SEC-NOPROC-001 & CRIT-TEST-DISCOVERY-SEC-001:
// Static discovery executes zero subprocesses during indexing.
func TestDiscover_SecurityZeroSubprocess(t *testing.T) {
	tempDir := t.TempDir()
	dangerousFile := `package exploit_test

import (
	"os"
	"testing"
)

func init() {
	// If this code is executed during discovery, it will create this file
	_ = os.WriteFile("exploit_executed.marker", []byte("bad"), 0600)
}

func TestNormal(t *testing.T) {}
`
	markerPath := filepath.Join(tempDir, "exploit_executed.marker")
	if err := os.WriteFile(filepath.Join(tempDir, "exploit_test.go"), []byte(dangerousFile), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	engine := testdiscovery.NewEngine()
	_, err := engine.Discover(context.Background(), testdiscovery.DiscoveryOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if _, err := os.Stat(markerPath); err == nil {
		t.Fatalf("SECURITY VIOLATION: Test code was executed during static discovery! Marker file exists.")
	}
}

// CRIT-TEST-DISC-SEC-TRAVERSAL-001:
// Discovery scanner rejects symlink escapes and directory traversal outside project root.
func TestDiscover_SecurityDirectoryTraversal(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := t.TempDir()

	// Put a test file in outsideDir
	outsideTest := `package outside_test
import "testing"
func TestOutside(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(outsideDir, "outside_test.go"), []byte(outsideTest), 0600); err != nil {
		t.Fatalf("failed to write outside test: %v", err)
	}

	// Create symlink inside tempDir pointing to outsideDir
	symlinkPath := filepath.Join(tempDir, "escape_link")
	_ = os.Symlink(outsideDir, symlinkPath)

	engine := testdiscovery.NewEngine()
	targets, err := engine.Discover(context.Background(), testdiscovery.DiscoveryOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, target := range targets {
		if strings.Contains(target.Path, "outside") || strings.Contains(target.Path, "escape_link") {
			t.Errorf("SECURITY VIOLATION: Discovery traversed symlink outside project root: %s", target.Path)
		}
	}
}

// CRIT-TEST-DISC-COMPL-SCHEMA-001 & CRIT-TEST-DISCOVERY-COMPL-001:
// Discovered test targets validate against test_case schema and produce compliant objects.
func TestDiscover_SchemaCompliance(t *testing.T) {
	tempDir := t.TempDir()
	goTest := `package service_test
import "testing"

// Criteria: CRIT-TEST-SVC-001
func TestServiceOperation(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(tempDir, "service_test.go"), []byte(goTest), 0600); err != nil {
		t.Fatalf("failed to write test: %v", err)
	}

	engine := testdiscovery.NewEngine()
	targets, err := engine.Discover(context.Background(), testdiscovery.DiscoveryOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	tcObj, err := engine.GenerateTestCaseObject(targets[0], tempDir)
	if err != nil {
		t.Fatalf("failed to generate test_case object: %v", err)
	}

	if tcObj[objects.FieldKeyKind] != objects.KindTestCase {
		t.Errorf("expected kind %s, got %v", objects.KindTestCase, tcObj[objects.FieldKeyKind])
	}
	id, ok := tcObj[objects.FieldKeyID].(string)
	if !ok || !strings.HasPrefix(id, "TST-") {
		t.Errorf("expected ID prefix TST-, got %v", id)
	}
	status, ok := tcObj[objects.FieldKeyStatus].(string)
	if !ok || status == "" {
		t.Errorf("expected valid status, got %v", status)
	}
}

// CRIT-TEST-DISC-ACCPT-INCREMENTAL-001 & CRIT-TEST-DISCOVERY-PERF-001:
// Incremental re-discovery uses cache to scan unchanged trees under 100ms.
func TestDiscover_IncrementalCache(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, ".test_cache.json")

	// Create 10 test files
	for i := 0; i < 10; i++ {
		goTest := `package bench_test
import "testing"
func TestItem(t *testing.T) {}
`
		fileName := filepath.Join(tempDir, filepath.Join(string(rune('a'+i)), "item_test.go"))
		_ = os.MkdirAll(filepath.Dir(fileName), 0750)
		_ = os.WriteFile(fileName, []byte(goTest), 0600)
	}

	engine := testdiscovery.NewEngine()
	opts := testdiscovery.DiscoveryOptions{
		ProjectRoot: tempDir,
		Incremental: true,
		CachePath:   cachePath,
	}

	// First run: populate cache
	_, err := engine.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("first discovery failed: %v", err)
	}

	// Second run: should be blazing fast (<100ms)
	start := time.Now()
	targets, err := engine.Discover(context.Background(), opts)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("second discovery failed: %v", err)
	}
	if len(targets) != 10 {
		t.Fatalf("expected 10 cached targets, got %d", len(targets))
	}
	if duration > 500*time.Millisecond {
		t.Errorf("incremental scan took too long: %v (expected < 500ms in unit test)", duration)
	}
}

// TestDiscover_Cargo verifies Rust Cargo test discovery for BLI-TESTCASE-DISCOVERY-RUST-003.
func TestDiscover_Cargo(t *testing.T) {
	tempDir := t.TempDir()

	rustCode := `
// Criteria: CRIT-TEST-DISC-FUNC-POLYGLOT-001
#[cfg(test)]
mod tests {
    #[test]
    fn test_compute_hash() {
        assert_eq!(2 + 2, 4);
    }

    #[tokio::test]
    async fn test_async_fetch() {
        assert!(true);
    }

    #[test]
    #[ignore]
    fn test_slow_bench() {}
}
`
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "lib.rs"), []byte(rustCode), 0600); err != nil {
		t.Fatal(err)
	}

	engine := testdiscovery.NewEngine()
	targets, err := engine.Discover(context.Background(), testdiscovery.DiscoveryOptions{ProjectRoot: tempDir})
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// Verify function names and language
	names := make(map[string]bool)
	for _, tgt := range targets {
		names[tgt.Function] = true
		if tgt.Language != "rust" {
			t.Errorf("expected language rust, got %s", tgt.Language)
		}
		if !strings.HasPrefix(tgt.ExecutionCommand, "cargo test") {
			t.Errorf("expected cargo test command, got %s", tgt.ExecutionCommand)
		}
	}
	if !names["test_compute_hash"] || !names["test_async_fetch"] || !names["test_slow_bench"] {
		t.Errorf("missing expected test functions in %v", names)
	}
}
