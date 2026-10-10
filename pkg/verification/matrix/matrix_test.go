package matrix_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

func createTestEngine(t *testing.T, tmpDir, ledgerName string, reg *matrix.CheckRegistry) (*matrix.Engine, context.Context) {
	t.Helper()
	ledgerPath := filepath.Join(tmpDir, ledgerName)
	engine, err := matrix.NewEngine(tmpDir, ledgerPath, reg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return engine, context.Background()
}

func TestClassifier(t *testing.T) {
	c := matrix.NewClassifier(nil)

	tests := []struct {
		path     string
		expected matrix.FileClass
	}{
		{"cmd/zqk/main.go", matrix.ClassGoProd},
		{"pkg/storage/store.go", matrix.ClassGoProd},
		{"pkg/storage/store_test.go", matrix.ClassGoTest},
		{"scripts/ci/run.sh", matrix.ClassScript},
		{"scripts/build.bash", matrix.ClassScript},
		{"config/settings.yaml", matrix.ClassConfigFile},
		{"config/app.json", matrix.ClassConfigFile},
		{"README.md", matrix.ClassDocs},
		{"LICENSE", matrix.ClassOther},
	}

	for _, tc := range tests {
		actual := c.Classify(tc.path)
		if actual != tc.expected {
			t.Errorf("Classify(%q) = %q, expected %q", tc.path, actual, tc.expected)
		}
	}
}

func TestHardcodedLogicScanner(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Violating file with hardcoded release tag, raw permission octal, and user path
	badFile := filepath.Join(tmpDir, "bad.go")
	badContent := `package main
import "fmt"
const version = "v2.9.4"
const userDir = "/Users/developer/data"
const mode = 0777
func main() { fmt.Println(version) }
`
	if err := fileutil.WriteFile(badFile, []byte(badContent), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}

	// 2. Clean file
	cleanFile := filepath.Join(tmpDir, "clean.go")
	cleanContent := `package main
import "fmt"
func main() { fmt.Println("hello world") }
`
	if err := fileutil.WriteFile(cleanFile, []byte(cleanContent), 0644); err != nil {
		t.Fatalf("failed to write clean file: %v", err)
	}

	scanner := scanners.NewHardcodedLogicScanner()
	ctx := context.Background()

	// Evaluate bad file
	badEntry := &matrix.FileEntry{
		Path:  "bad.go",
		Class: matrix.ClassGoProd,
	}
	resBad, err := scanner.Run(ctx, tmpDir, badEntry)
	if err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if resBad.Status != matrix.CheckStatusFailed {
		t.Errorf("expected bad file to fail check, got %q", resBad.Status)
	}
	if len(resBad.Findings) < 3 {
		t.Errorf("expected at least 3 findings, got %d", len(resBad.Findings))
	}

	// Evaluate clean file
	cleanEntry := &matrix.FileEntry{
		Path:  "clean.go",
		Class: matrix.ClassGoProd,
	}
	resClean, err := scanner.Run(ctx, tmpDir, cleanEntry)
	if err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if resClean.Status != matrix.CheckStatusPassed {
		t.Errorf("expected clean file to pass check, got %q: %v", resClean.Status, resClean.Findings)
	}
}

func TestMatrixEngine_CacheHitAndInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerPath := filepath.Join(tmpDir, "matrix.json")

	// Create test file
	testFileRel := "service.go"
	testFileFull := filepath.Join(tmpDir, testFileRel)
	content1 := "package service\n\nfunc Run() string { return \"ok\" }\n"
	if err := fileutil.WriteFile(testFileFull, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	registry := matrix.NewCheckRegistry()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic Scanner",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassGoProd},
		Runner:        scanners.NewHardcodedLogicScanner(),
	})
	registry.Register(matrix.CheckDefinition{
		ID:            "ast-hygiene",
		Name:          "AST Hygiene Scanner",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassGoProd},
		Runner:        scanners.NewHardcodedLogicScanner(),
	})

	engine, ctx := createTestEngine(t, tmpDir, "matrix.json", registry)

	// First evaluation -> Cache Miss
	entry, hit, err := engine.EvaluateFile(ctx, testFileRel, false)
	if err != nil {
		t.Fatalf("EvaluateFile failed: %v", err)
	}
	if hit {
		t.Errorf("expected initial evaluation to be cache miss, got cache hit")
	}
	if entry.Checks["hardcoded-logic"].Status != matrix.CheckStatusPassed {
		t.Fatalf("expected check to pass, got %v", entry.Checks["hardcoded-logic"].Status)
	}

	// Stamp adversarial czar pass
	err = engine.RecordAgentCheck(testFileRel, "anti-hardcoding-czar", matrix.CheckStatusPassed, "PER-HARDCODING-ERADICATION-CZAR", "Audited and verified clean", nil)
	if err != nil {
		t.Fatalf("failed to record agent check: %v", err)
	}

	// Persist ledger
	if err := engine.SaveLedger(); err != nil {
		t.Fatalf("failed to save ledger: %v", err)
	}

	// Reload engine from disk
	engineReloaded, err := matrix.NewEngine(tmpDir, ledgerPath, registry)
	if err != nil {
		t.Fatalf("failed to reload engine: %v", err)
	}

	// Second evaluation without modification -> Cache Hit!
	entry2, hit2, err := engineReloaded.EvaluateFile(ctx, testFileRel, false)
	if err != nil {
		t.Fatalf("EvaluateFile reloaded failed: %v", err)
	}
	if !hit2 {
		t.Errorf("expected second evaluation to be cache hit, got cache miss")
	}
	if entry2.ContentHash != entry.ContentHash {
		t.Errorf("content hash changed unexpectedly")
	}

	// Now modify the file -> SHA-256 hash changes!
	content2 := "package service\n\nfunc Run() string { return \"modified\" }\n"
	if err := fileutil.WriteFile(testFileFull, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	// Third evaluation -> Cache Miss (invalidated by hash difference)
	entry3, hit3, err := engineReloaded.EvaluateFile(ctx, testFileRel, false)
	if err != nil {
		t.Fatalf("EvaluateFile after mutation failed: %v", err)
	}
	if hit3 {
		t.Errorf("expected mutated file to be cache miss, got cache hit")
	}
	if entry3.ContentHash == entry.ContentHash {
		t.Errorf("expected new content hash, got old hash")
	}
	// The czar check should be pending again because content hash changed
	if entry3.Checks["anti-hardcoding-czar"].Status != matrix.CheckStatusPending {
		t.Errorf("expected czar check to reset to pending upon hash change, got %v", entry3.Checks["anti-hardcoding-czar"].Status)
	}
}

func TestMatrixEngine_EvaluateAll_AndRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerPath := filepath.Join(tmpDir, "matrix.json")

	// Create directories and files
	subDir := filepath.Join(tmpDir, "pkg", "core")
	if err := fileutil.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	f1 := filepath.Join(subDir, "core.go")
	if err := fileutil.WriteFile(f1, []byte("package core\nfunc A() {}\n"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	f2 := filepath.Join(subDir, "core_test.go")
	if err := fileutil.WriteFile(f2, []byte("package core_test\nfunc TestA() {}\n"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	f3 := filepath.Join(tmpDir, "README.md")
	if err := fileutil.WriteFile(f3, []byte("# Docs\n"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	registry := matrix.NewCheckRegistry()
	scanner := scanners.NewHardcodedLogicScanner()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassGoProd, matrix.ClassGoTest},
		Runner:        scanner,
	})
	registry.Register(matrix.CheckDefinition{
		ID:            "ast-hygiene",
		Name:          "AST Hygiene",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassGoProd, matrix.ClassGoTest},
		Runner:        scanner,
	})

	defs := registry.All()
	if len(defs) != 2 {
		t.Errorf("expected 2 registered defs, got %d", len(defs))
	}

	engine, err := matrix.NewEngine(tmpDir, ledgerPath, registry)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	summary, err := engine.EvaluateAll(ctx, false)
	if err != nil {
		t.Fatalf("EvaluateAll failed: %v", err)
	}

	if summary.TotalFiles < 3 {
		t.Errorf("expected at least 3 total files, got %d", summary.TotalFiles)
	}
	if summary.Evaluated < 3 {
		t.Errorf("expected at least 3 evaluated, got %d", summary.Evaluated)
	}

	ledger := engine.GetLedger()
	if len(ledger.Files) < 3 {
		t.Errorf("expected at least 3 files in ledger, got %d", len(ledger.Files))
	}

	// Stamp Czar check on core.go
	err = engine.RecordAgentCheck(filepath.Join("pkg", "core", "core.go"), "anti-hardcoding-czar", matrix.CheckStatusPassed, "PER-HARDCODING-ERADICATION-CZAR", "Verified", nil)
	if err != nil {
		t.Fatalf("RecordAgentCheck failed: %v", err)
	}

	// Re-evaluate all -> should hit cache
	summary2, err := engine.EvaluateAll(ctx, false)
	if err != nil {
		t.Fatalf("second EvaluateAll failed: %v", err)
	}
	if summary2.CacheHits == 0 {
		t.Errorf("expected cache hits on second run, got 0")
	}
}

func TestLiteralInventory_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	invPath := filepath.Join(tmpDir, "literal_inventory.json")

	inv, err := matrix.NewLiteralInventory(invPath)
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}

	// 1. First occurrence is tolerated
	count, isDup := inv.Record("starting service daemon", "pkg/daemon/start.go", 42)
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if isDup {
		t.Errorf("expected isDup false for single occurrence, got true")
	}

	// 2. Exact same location re-recorded should not double count
	count, isDup = inv.Record("starting service daemon", "pkg/daemon/start.go", 42)
	if count != 1 || isDup {
		t.Errorf("expected count 1 and isDup false for duplicate location, got %d, %v", count, isDup)
	}

	// 3. Second occurrence in different file or line triggers duplicate violation
	count, isDup = inv.Record("starting service daemon", "pkg/daemon/restart.go", 18)
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
	if !isDup {
		t.Errorf("expected isDup true for 2nd occurrence, got false")
	}

	// 4. Save and reload
	if err := inv.Save(); err != nil {
		t.Fatalf("failed to save inventory: %v", err)
	}

	inv2, err := matrix.NewLiteralInventory(invPath)
	if err != nil {
		t.Fatalf("failed to reload inventory: %v", err)
	}
	if inv2.TotalCount() != 1 {
		t.Errorf("expected 1 total literal, got %d", inv2.TotalCount())
	}
	if inv2.DuplicateCount() != 1 {
		t.Errorf("expected 1 duplicate, got %d", inv2.DuplicateCount())
	}

	dups := inv2.FindDuplicates()
	if len(dups) != 1 || dups[0].Literal != "starting service daemon" {
		t.Fatalf("expected duplicate record for 'starting service daemon', got %+v", dups)
	}
}

func TestComputeDiamondScore(t *testing.T) {
	// Flawless (5 diamonds)
	if score := matrix.ComputeDiamondScore(nil); score != matrix.ScoreFlawless {
		t.Errorf("expected 5 diamonds for nil findings, got %v", score)
	}

	// Minor notices (4 diamonds)
	minor := []matrix.Finding{
		{Severity: "warning", Message: "log string used once"},
	}
	if score := matrix.ComputeDiamondScore(minor); score != matrix.ScoreMinor {
		t.Errorf("expected 4 diamonds for single warning, got %v", score)
	}

	// Multiple notices (3 diamonds)
	backlog := []matrix.Finding{
		{Severity: "warning", Message: "notice 1"},
		{Severity: "warning", Message: "notice 2"},
		{Severity: "warning", Message: "notice 3"},
	}
	if score := matrix.ComputeDiamondScore(backlog); score != matrix.ScoreBacklog {
		t.Errorf("expected 3 diamonds for 3 warnings, got %v", score)
	}

	// High / Error violation (2 diamonds)
	failing := []matrix.Finding{
		{Severity: "error", Message: "raw octal permission 0777"},
	}
	if score := matrix.ComputeDiamondScore(failing); score != matrix.ScoreFailing {
		t.Errorf("expected 2 diamonds for error severity, got %v", score)
	}

	// Critical violation (1 diamond)
	critical := []matrix.Finding{
		{Severity: "critical", Message: "hardcoded release version"},
	}
	if score := matrix.ComputeDiamondScore(critical); score != matrix.ScoreCritical {
		t.Errorf("expected 1 diamond for critical severity, got %v", score)
	}
}

func TestStampFileCheck_HashVerification(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")
	content := []byte("package sample\n\nfunc Sample() {}\n")
	if err := fileutil.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	engine, ctx := createTestEngine(t, tmpDir, "matrix.json", nil)
	actualHash, err := matrix.ComputeFileHash(filePath)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// 1. Stamping with incorrect hash must fail
	_, err = engine.StampFileCheck(ctx, matrix.StampRequest{
		Path:        "sample.go",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: "0000000000000000000000000000000000000000000000000000000000000000",
		Evaluator:   "PER-HARDCODING-ERADICATION-CZAR",
	})
	if err == nil {
		t.Fatalf("expected error for mismatched hash, got nil")
	}

	// 2. Stamping with correct hash must succeed
	res, err := engine.StampFileCheck(ctx, matrix.StampRequest{
		Path:        "sample.go",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: actualHash,
		Evaluator:   "PER-HARDCODING-ERADICATION-CZAR",
		Status:      matrix.CheckStatusPassed,
	})
	if err != nil {
		t.Fatalf("failed to stamp file check: %v", err)
	}
	if res.Status != matrix.CheckStatusPassed {
		t.Errorf("expected passed status, got %s", res.Status)
	}
	if res.DiamondScore != matrix.ScoreFlawless {
		t.Errorf("expected flawless score, got %v", res.DiamondScore)
	}
}

func TestDefaultDimensions(t *testing.T) {
	dims := matrix.DefaultDimensions()
	if len(dims) < 6 {
		t.Errorf("expected at least 6 canonical dimensions, got %d", len(dims))
	}
	foundHCODE := false
	for _, d := range dims {
		if d.Code == matrix.DimensionHCODE {
			foundHCODE = true
			if d.PolicyID == "" {
				t.Errorf("expected HCODE to have bound policy ID")
			}
		}
	}
	if !foundHCODE {
		t.Errorf("HCODE dimension not found in default dimensions")
	}
}

func TestEvaluateScorecard_ThresholdsAndScorecards(t *testing.T) {
	tmpDir := t.TempDir()
	engine, ctx := createTestEngine(t, tmpDir, "matrix.json", nil)

	// 1. Clean scorecard evaluation
	cleanSc := &matrix.FileScorecard{
		SchemaVersion: "1.0.0",
		FilePath:      "pkg/clean.go",
		ContentHash:   "abc12345",
		FileClass:     matrix.ClassGoProd,
		Dimension:     matrix.DimensionHCODE,
		Findings:      nil,
	}

	evalClean, err := engine.EvaluateScorecard(ctx, cleanSc, false)
	if err != nil {
		t.Fatalf("failed to evaluate clean scorecard: %v", err)
	}
	if !evalClean.Passed {
		t.Errorf("expected clean scorecard to pass, got failed: %v", evalClean.ThresholdFailures)
	}
	if cleanSc.DiamondScore != matrix.ScoreFlawless {
		t.Errorf("expected 5 diamonds for clean scorecard, got %v", cleanSc.DiamondScore)
	}

	// 2. Scorecard save and reload
	loaded, err := engine.LoadScorecard("pkg/clean.go")
	if err != nil {
		t.Fatalf("failed to load saved scorecard: %v", err)
	}
	if loaded.FilePath != cleanSc.FilePath || loaded.DiamondScore != cleanSc.DiamondScore {
		t.Errorf("loaded scorecard mismatch: %+v vs %+v", loaded, cleanSc)
	}

	// 3. Violating scorecard evaluation (with autoMint=false to test policy threshold logic)
	badSc := &matrix.FileScorecard{
		SchemaVersion: "1.0.0",
		FilePath:      "pkg/bad.go",
		ContentHash:   "def67890",
		FileClass:     matrix.ClassGoProd,
		Dimension:     matrix.DimensionHCODE,
		Findings: []matrix.Finding{
			{
				Line:     42,
				RuleID:   "no-raw-permission-octal",
				Severity: "error",
				Message:  "Raw permission octal 0777",
			},
		},
	}

	evalBad, err := engine.EvaluateScorecard(ctx, badSc, false)
	if err != nil {
		t.Fatalf("failed to evaluate bad scorecard: %v", err)
	}
	if evalBad.Passed {
		t.Errorf("expected bad scorecard to fail policy check")
	}
	if !evalBad.RemediationRequired {
		t.Errorf("expected RemediationRequired to be true")
	}
	if len(evalBad.ThresholdFailures) == 0 {
		t.Errorf("expected threshold failures to be populated")
	}
	if badSc.Status != matrix.CheckStatusFailed {
		t.Errorf("expected scorecard status to be failed, got %s", badSc.Status)
	}
}

func TestGenericMultiLanguageClassifier(t *testing.T) {
	c := matrix.NewClassifier(nil)

	tests := []struct {
		path     string
		expected matrix.FileClass
	}{
		// Python
		{"backend/service.py", matrix.ClassSource},
		{"backend/test_service.py", matrix.ClassTest},
		{"backend/service_test.py", matrix.ClassTest},
		// TypeScript / JavaScript
		{"frontend/src/app.ts", matrix.ClassSource},
		{"frontend/src/app.spec.ts", matrix.ClassTest},
		{"frontend/src/app.test.js", matrix.ClassTest},
		// Rust
		{"src/main.rs", matrix.ClassSource},
		{"tests/integration_test.rs", matrix.ClassTest},
		// Config & Docs
		{"config/app.toml", matrix.ClassConfigFile},
		{"docs/ARCHITECTURE.md", matrix.ClassDocs},
		{"deploy/run.ps1", matrix.ClassScript},
	}

	for _, tc := range tests {
		actual := c.Classify(tc.path)
		if actual != tc.expected {
			t.Errorf("Classify(%q) = %q, expected %q", tc.path, actual, tc.expected)
		}
	}
}

func TestMatrixConfig_YamlAndJson(t *testing.T) {
	tmpDir := t.TempDir()

	yamlCfgPath := filepath.Join(tmpDir, "matrix.yaml")
	yamlContent := `name: "custom-matrix"
description: "Specialized microservice matrix"
storage_dir: ".custom_matrix"
dimensions:
  - code: "CUSTOM_SEC"
    name: "Custom Security"
    category: "security"
    description: "Custom security rules"
    severity: "critical"
commands:
  - id: "echo-check"
    name: "Echo Check"
    description: "Runs echo command"
    command: "echo {{file}}"
    target_classes: ["source"]
`
	if err := fileutil.WriteFile(yamlCfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml config: %v", err)
	}

	cfg, err := matrix.LoadConfig(yamlCfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != "custom-matrix" {
		t.Errorf("expected name 'custom-matrix', got %q", cfg.Name)
	}
	if cfg.StorageDir != ".custom_matrix" {
		t.Errorf("expected storage_dir '.custom_matrix', got %q", cfg.StorageDir)
	}
	if len(cfg.Dimensions) != 1 || cfg.Dimensions[0].Code != "CUSTOM_SEC" {
		t.Errorf("expected dimension CUSTOM_SEC")
	}
	if len(cfg.Commands) != 1 || cfg.Commands[0].ID != "echo-check" {
		t.Errorf("expected command echo-check")
	}
}

func TestCommandCheckRunner(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "example.txt")
	if err := fileutil.WriteFile(testFile, []byte("sample content\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	runner := matrix.NewCommandCheckRunner("echo-test", "echo checking:{{file}}", []matrix.FileClass{matrix.ClassSource})
	entry := &matrix.FileEntry{Path: "example.txt", Class: matrix.ClassSource}

	res, err := runner.Run(context.Background(), tmpDir, entry)
	if err != nil {
		t.Fatalf("CommandCheckRunner failed: %v", err)
	}
	if res.Status != matrix.CheckStatusPassed {
		t.Errorf("expected command to pass, got %s", res.Status)
	}
}

func TestPolicyInvariant_ZeroASTImports(t *testing.T) {
	// Enforces POL-CODE-1791602200765493000-982b84b3 Invariant 1:
	// Zero language AST/compiler imports (e.g. no go/parser, no go/ast, no go/token) in pkg/verification/matrix.
	bannedImports := []string{
		"go" + "/" + "ast",
		"go" + "/" + "parser",
		"go" + "/" + "token",
		"golang.org" + "/" + "x" + "/tools/go/ast",
	}

	root := "."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		inImportBlock := false

		for lineNum, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "import (") {
				inImportBlock = true
				continue
			}
			if inImportBlock && trimmed == ")" {
				inImportBlock = false
				continue
			}

			isImportLine := inImportBlock || strings.HasPrefix(trimmed, "import ")
			if !isImportLine {
				continue
			}

			for _, banned := range bannedImports {
				if strings.Contains(trimmed, `"`+banned+`"`) {
					t.Errorf("POL-CODE Invariant Violation: %s:%d imports banned package %s", path, lineNum+1, banned)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to scan matrix directory: %v", err)
	}
}

func TestProfile_CustomAndDeclarativeGlobs(t *testing.T) {
	customProfile := &matrix.Profile{
		Name:        "polyglot-enterprise",
		Description: "Enterprise polyglot profile with custom extensions and globs",
		Classes: map[matrix.FileClass]matrix.ClassConfig{
			"proto": {
				Name:           "proto",
				Description:    "Protobuf definitions",
				Extensions:     []string{".proto"},
				RequiredChecks: []string{"hardcoded-logic"},
			},
			matrix.ClassTest: {
				Name:        matrix.ClassTest,
				Description: "Polyglot tests",
				Extensions:  []string{".py", ".ts", ".go", ".rs"},
				MatchPatterns: []string{
					"**/tests/*",
					"*_test.*",
					"*.spec.*",
				},
				RequiredChecks: []string{"hardcoded-logic"},
			},
			matrix.ClassSource: {
				Name:        matrix.ClassSource,
				Description: "Production source files",
				Extensions:  []string{".py", ".ts", ".go", ".rs"},
				ExcludePatterns: []string{
					"**/tests/*",
					"*_test.*",
					"*.spec.*",
				},
				RequiredChecks: []string{"hardcoded-logic"},
			},
		},
	}

	c := matrix.NewClassifier(customProfile)
	if c.Profile().Name != "polyglot-enterprise" {
		t.Errorf("expected profile name 'polyglot-enterprise', got %q", c.Profile().Name)
	}
	if len(c.Configs()) != 3 {
		t.Errorf("expected 3 configs, got %d", len(c.Configs()))
	}

	testCases := []struct {
		path     string
		expected matrix.FileClass
	}{
		{"api/v1/user.proto", "proto"},
		{"services/auth/tests/test_login.py", matrix.ClassTest},
		{"services/auth/user_service.py", matrix.ClassSource},
		{"web/src/components/button.spec.ts", matrix.ClassTest},
		{"web/src/components/button.ts", matrix.ClassSource},
		{"crates/core/src/lib.rs", matrix.ClassSource},
		{"crates/core/tests/integration.rs", matrix.ClassTest},
		{"random/file.unknown", matrix.ClassOther},
	}

	for _, tc := range testCases {
		got := c.Classify(tc.path)
		if got != tc.expected {
			t.Errorf("Classify(%q) = %q, expected %q", tc.path, got, tc.expected)
		}
	}
}

func TestFileScorecard_Serialization_JSON_Markdown(t *testing.T) {
	sc := &matrix.FileScorecard{
		SchemaVersion: "1.0.0",
		FilePath:      "pkg/auth/token.go",
		ContentHash:   "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		FileClass:     matrix.ClassSource,
		EvaluatedAt:   time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
		Evaluator:     "PER-HARDCODING-ERADICATION-CZAR",
		Dimension:     matrix.DimensionHCODE,
		PolicyID:      "POL-CODE-HARDCODING-001",
		Status:        matrix.CheckStatusFailed,
		DiamondScore:  matrix.ScoreFailing,
		DiamondLabel:  matrix.ScoreFailing.String(),
		Findings: []matrix.Finding{
			{
				Line:     25,
				Severity: "error",
				Message:  "Raw octal permission literal 0777",
				RuleID:   "no-raw-permission-octal",
			},
		},
		PolicyEvaluation: matrix.PolicyEvaluation{
			EvaluatedAt:         time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
			Passed:              false,
			ThresholdFailures:   []string{"High violation at line 25: Raw octal permission literal 0777"},
			RemediationRequired: true,
			Remediation: &matrix.RemediationBlock{
				TechnicalDebtID: "TDE-12345",
				BacklogItemID:   "BLI-67890",
				AssignedPersona: "PER-COMMUNITY-SOFTWARE-ENGINEER",
				Status:          "pending",
			},
		},
	}

	// 1. JSON Serialization
	jsonStr, err := sc.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"file_path": "pkg/auth/token.go"`) {
		t.Errorf("json missing file_path: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"diamond_score": 2`) {
		t.Errorf("json missing diamond_score: %s", jsonStr)
	}

	// 2. Markdown Serialization
	mdStr := sc.ToMarkdown()
	if !strings.Contains(mdStr, "# File Verification Scorecard: `pkg/auth/token.go`") {
		t.Errorf("markdown missing title: %s", mdStr)
	}
	if !strings.Contains(mdStr, "no-raw-permission-octal") {
		t.Errorf("markdown missing rule id: %s", mdStr)
	}
	if !strings.Contains(mdStr, "TDE-12345") {
		t.Errorf("markdown missing technical debt id: %s", mdStr)
	}
	if !strings.Contains(mdStr, "**Result**: `FAILED`") {
		t.Errorf("markdown missing failed result: %s", mdStr)
	}

	// 3. CSV Findings Export
	csvStr := sc.FindingsCSV()
	if !strings.Contains(csvStr, `"pkg/auth/token.go",25,"error","no-raw-permission-octal"`) {
		t.Errorf("csv missing row: %s", csvStr)
	}
}

func TestContentAddressedLedger_RecordsAndO1Caching(t *testing.T) {
	tmpDir := t.TempDir()

	fileRel := "src/calc.py"
	fileFull := filepath.Join(tmpDir, fileRel)
	if err := fileutil.MkdirAll(filepath.Dir(fileFull), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(fileFull, []byte("def add(a, b):\n    return a + b\n"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	registry := matrix.NewCheckRegistry()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic",
		Kind:          matrix.CheckKindPattern,
		TargetClasses: []matrix.FileClass{matrix.ClassSource},
		Runner:        scanners.NewHardcodedLogicScanner(),
	})

	engine, ctx := createTestEngine(t, tmpDir, "ledger.json", registry)

	// Initial evaluation
	_, hit1, err := engine.EvaluateFile(ctx, fileRel, false)
	if err != nil {
		t.Fatalf("EvaluateFile failed: %v", err)
	}
	if hit1 {
		t.Errorf("expected initial pass to be a cache miss")
	}

	// Stamped check
	actualHash, err := matrix.ComputeFileHash(fileFull)
	if err != nil {
		t.Fatalf("ComputeFileHash failed: %v", err)
	}
	_, err = engine.StampFileCheck(ctx, matrix.StampRequest{
		Path:        fileRel,
		CheckID:     "anti-hardcoding-czar",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: actualHash,
		Status:      matrix.CheckStatusPassed,
		Evaluator:   "PER-HARDCODING-ERADICATION-CZAR",
	})
	if err != nil {
		t.Fatalf("failed to stamp: %v", err)
	}

	// Verify Content-Addressed Records query
	records := engine.Records()
	if len(records) < 2 {
		t.Fatalf("expected at least 2 records, got %d", len(records))
	}
	foundStamped := false
	for _, r := range records {
		if r.FilePath == fileRel && r.StampedBy == "PER-HARDCODING-ERADICATION-CZAR" {
			foundStamped = true
			if r.SHA256 != actualHash {
				t.Errorf("expected record SHA %s, got %s", actualHash, r.SHA256)
			}
			if r.Status != matrix.CheckStatusPassed {
				t.Errorf("expected record status passed, got %s", r.Status)
			}
		}
	}
	if !foundStamped {
		t.Errorf("stamped record not found in engine.Records()")
	}

	// Second evaluation on unchanged file must be O(1) cache hit
	_, hit2, err := engine.EvaluateFile(ctx, fileRel, false)
	if err != nil {
		t.Fatalf("second EvaluateFile failed: %v", err)
	}
	if !hit2 {
		t.Errorf("expected second evaluation to be cache hit")
	}
}

func TestPluggableCheckPrimitives_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerPath := filepath.Join(tmpDir, "ledger.json")

	fileRel := "main.sh"
	fileFull := filepath.Join(tmpDir, fileRel)
	if err := fileutil.WriteFile(fileFull, []byte("#!/bin/sh\necho 'hello world'\n"), 0755); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	registry := matrix.NewCheckRegistry()

	// 1. Primitive: pattern
	registry.Register(matrix.CheckDefinition{
		ID:            "check-pattern",
		Kind:          matrix.CheckKindPattern,
		TargetClasses: []matrix.FileClass{matrix.ClassScript},
		Runner: scanners.NewPatternScanner("check-pattern", scanners.PatternRule{
			ID:       "rule-echo",
			Message:  "echo used",
			Severity: "warning",
			Pattern:  `echo`,
		}),
	})

	// 2. Primitive: command
	registry.Register(matrix.CheckDefinition{
		ID:            "check-command",
		Kind:          matrix.CheckKindCommand,
		TargetClasses: []matrix.FileClass{matrix.ClassScript},
		Runner:        scanners.NewCommandScanner("check-command", "echo {{path}}", []matrix.FileClass{matrix.ClassScript}),
	})

	// 3. Primitive: agent
	evaluator := scanners.AgentEvaluatorFunc(func(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error) {
		return matrix.ScoreFlawless, "Rubric passed with 5 diamonds", nil, nil
	})
	registry.Register(matrix.CheckDefinition{
		ID:            "check-agent",
		Kind:          matrix.CheckKindAgent,
		TargetClasses: []matrix.FileClass{matrix.ClassScript},
		Runner:        scanners.NewAgentScanner("check-agent", "Security rubric", evaluator, []matrix.FileClass{matrix.ClassScript}),
	})

	// Configure profile with these 3 required checks on ClassScript
	profile := matrix.DefaultProfile()
	scriptCfg := profile.Classes[matrix.ClassScript]
	scriptCfg.RequiredChecks = []string{"check-pattern", "check-command", "check-agent"}
	profile.Classes[matrix.ClassScript] = scriptCfg

	engine, err := matrix.NewEngine(tmpDir, ledgerPath, registry)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	engine.SetClassifier(matrix.NewClassifier(profile))

	ctx := context.Background()
	entry, _, err := engine.EvaluateFile(ctx, fileRel, false)
	if err != nil {
		t.Fatalf("EvaluateFile failed: %v", err)
	}

	// Verify all 3 primitives ran and produced results
	resPat, ok := entry.Checks["check-pattern"]
	if !ok || resPat.Status != matrix.CheckStatusPassed {
		t.Errorf("pattern check failed or missing: %+v", resPat)
	}

	resCmd, ok := entry.Checks["check-command"]
	if !ok || resCmd.Status != matrix.CheckStatusPassed {
		t.Errorf("command check failed or missing: %+v", resCmd)
	}

	resAgt, ok := entry.Checks["check-agent"]
	if !ok || resAgt.Status != matrix.CheckStatusPassed {
		t.Errorf("agent check failed or missing: %+v", resAgt)
	}
	if resAgt.DiamondScore != matrix.ScoreFlawless {
		t.Errorf("expected agent check score 5, got %v", resAgt.DiamondScore)
	}
}

