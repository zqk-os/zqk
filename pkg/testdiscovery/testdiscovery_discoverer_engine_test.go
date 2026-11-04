// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testdiscovery

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDiscoveryCache_EmptyPathAndCleanState(t *testing.T) {
	t.Parallel()

	cache := LoadCache("")
	if cache == nil {
		t.Fatal("expected non-nil cache for empty path")
	}

	targets, ok := cache.Get("nonexistent", time.Now(), nil)
	if ok || targets != nil {
		t.Fatalf("expected cache miss on empty cache, got ok=%v, targets=%v", ok, targets)
	}

	err := cache.Save()
	if err != nil {
		t.Fatalf("expected nil error on saving clean empty-path cache, got %v", err)
	}
}

func TestDiscoveryCache_CorruptJSONHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupt_cache.json")

	err := fileutil.WriteFile(corruptPath, []byte("{invalid-json-data"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write corrupt cache file: %v", err)
	}

	cache := LoadCache(corruptPath)
	if cache == nil {
		t.Fatal("expected non-nil cache even when file is corrupt")
	}

	targets, ok := cache.Get("any/path", time.Now(), []byte("data"))
	if ok || targets != nil {
		t.Fatalf("expected cache miss on corrupt cache, got ok=%v, targets=%v", ok, targets)
	}
}

func TestDiscoveryCache_MtimeAndHashBoundaries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "sub", "cache.json")
	cache := LoadCache(cachePath)

	modTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	content := []byte("func TestSample(t *testing.T) {}")
	relPath := "pkg/sample/sample_test.go"

	targets := []DiscoveredTarget{
		{
			ID:       "go:pkg/sample/sample_test.go:TestSample",
			Path:     relPath,
			Language: "go",
			Function: "TestSample",
			Line:     1,
		},
	}

	cache.Set(relPath, modTime, content, targets)

	gotMtime, okMtime := cache.Get(relPath, modTime, nil)
	if !okMtime || len(gotMtime) != 1 {
		t.Fatalf("expected mtime hit, got ok=%v, len=%d", okMtime, len(gotMtime))
	}

	newModTime := modTime.Add(2 * time.Hour)
	gotHash, okHash := cache.Get(relPath, newModTime, content)
	if !okHash || len(gotHash) != 1 {
		t.Fatalf("expected hash hit, got ok=%v, len=%d", okHash, len(gotHash))
	}

	emptyTargets, okEmpty := cache.Get(relPath, newModTime, nil)
	if okEmpty || emptyTargets != nil {
		t.Fatalf("expected miss when mtime differs and content is empty, got %v", emptyTargets)
	}

	diffTargets, okDiff := cache.Get(relPath, newModTime, []byte("different content"))
	if okDiff || diffTargets != nil {
		t.Fatalf("expected miss when mtime differs and content differs, got %v", diffTargets)
	}

	err := cache.Save()
	if err != nil {
		t.Fatalf("expected successful save, got %v", err)
	}

	errClean := cache.Save()
	if errClean != nil {
		t.Fatalf("expected nil error saving non-dirty cache, got %v", errClean)
	}
}

func TestDiscoveryCache_SaveDirectoryCreationFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	blockingFilePath := filepath.Join(tmpDir, "file_as_dir")
	err := fileutil.WriteFile(blockingFilePath, []byte("blocker"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write blocking file: %v", err)
	}

	cachePath := filepath.Join(blockingFilePath, "cache.json")
	cache := LoadCache(cachePath)
	cache.Set("dummy", time.Now(), []byte("data"), nil)

	errSave := cache.Save()
	if errSave == nil {
		t.Fatal("expected error saving cache where parent is an existing file, got nil")
	}
}

func TestCargoDiscoverer_LanguageAndExtensions(t *testing.T) {
	t.Parallel()

	d := NewCargoDiscoverer()
	if d.Language() != "rust" {
		t.Fatalf("expected rust, got %s", d.Language())
	}
	if !d.CanHandle("src/lib.rs") || !d.CanHandle("tests/integration_test.rs") {
		t.Fatal("expected CanHandle to accept .rs files")
	}
	if d.CanHandle("src/lib.go") || d.CanHandle("tests/main.py") {
		t.Fatal("expected CanHandle to reject non-.rs files")
	}
}

func TestCargoDiscoverer_SyntaxAndAttributes(t *testing.T) {
	t.Parallel()

	d := NewCargoDiscoverer()
	content := []byte(`
// criteria: CRIT-RUST-001, CRIT-RUST-002
// req: REQ-RUST-AUTH
#[test]
fn test_standard() {
}

#[tokio::test]
pub async fn test_tokio_async() {
}

// scenario: slow-path
#[test]
#[ignore]
async fn test_ignored() {
}

#[test]

fn test_reset_on_empty_line() {
}
`)

	targets, err := d.Discover(context.Background(), "/root", "tests/integration.rs", content)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	if targets[0].Function != "test_standard" || len(targets[0].CriteriaRefs) != 2 {
		t.Fatalf("unexpected target 0: %+v", targets[0])
	}
	if targets[1].Function != "test_tokio_async" || targets[1].Suite != "integration" {
		t.Fatalf("unexpected target 1: %+v", targets[1])
	}
	if targets[2].Function != "test_ignored" {
		t.Fatalf("unexpected target 2: %+v", targets[2])
	}

	hasIgnore := false
	for _, tag := range targets[2].Tags {
		if tag == "ignore" {
			hasIgnore = true
			break
		}
	}
	if !hasIgnore {
		t.Fatalf("expected ignore tag in target 2, got %v", targets[2].Tags)
	}
}

func TestCargoDiscoverer_ModuleSuiteDetection(t *testing.T) {
	t.Parallel()

	d := NewCargoDiscoverer()
	content := []byte(`
pub mod tests {
    #[test]
    pub fn test_in_mod() {}
}

mod custom_tests {
    #[test]
    fn test_custom_mod() {}
}
`)

	targets, err := d.Discover(context.Background(), "/root", "src/auth.rs", content)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Suite != "tests" || targets[0].Function != "test_in_mod" {
		t.Fatalf("unexpected target 0 suite/func: %+v", targets[0])
	}
	if targets[1].Suite != "custom_tests" || targets[1].Function != "test_custom_mod" {
		t.Fatalf("unexpected target 1 suite/func: %+v", targets[1])
	}
}

func TestGoDiscoverer_LanguageAndExtensions(t *testing.T) {
	t.Parallel()

	d := NewGoDiscoverer()
	if d.Language() != "go" {
		t.Fatalf("expected go, got %s", d.Language())
	}
	if !d.CanHandle("pkg/foo/bar_test.go") {
		t.Fatal("expected CanHandle to accept _test.go")
	}
	if d.CanHandle("pkg/foo/bar.go") || d.CanHandle("pkg/foo/test.py") {
		t.Fatal("expected CanHandle to reject non-_test.go files")
	}
}

func TestGoDiscoverer_ParseErrorHandling(t *testing.T) {
	t.Parallel()

	d := NewGoDiscoverer()
	invalidGo := []byte(`package broken; func { incomplete syntax`)
	targets, err := d.Discover(context.Background(), "/root", "pkg/broken/broken_test.go", invalidGo)
	if err == nil {
		t.Fatal("expected error parsing invalid Go syntax, got nil")
	}
	if targets != nil {
		t.Fatalf("expected nil targets on parse error, got %v", targets)
	}
}

func TestGoDiscoverer_BuildTagsAndBenchmarks(t *testing.T) {
	t.Parallel()

	d := NewGoDiscoverer()
	content := []byte(`//go:build unit integration
// +build legacy

package worker_test

import "testing"

// Criteria: CRIT-BENCH-001
// Req: REQ-PERF-001
// Scenario: high-throughput
func BenchmarkProcessBatch(b *testing.B) {
}

func ExampleProcessBatch() {
}

func helperNotTest() {
}
`)

	targets, err := d.Discover(context.Background(), "/root", "pkg/worker/worker_test.go", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets (benchmark + example), got %d", len(targets))
	}

	bench := targets[0]
	if bench.Function != "BenchmarkProcessBatch" {
		t.Fatalf("expected BenchmarkProcessBatch, got %s", bench.Function)
	}
	if len(bench.CriteriaRefs) != 1 || bench.CriteriaRefs[0] != "CRIT-BENCH-001" {
		t.Fatalf("unexpected criteria refs: %v", bench.CriteriaRefs)
	}
	if len(bench.RequirementRefs) != 1 || bench.RequirementRefs[0] != "REQ-PERF-001" {
		t.Fatalf("unexpected req refs: %v", bench.RequirementRefs)
	}

	expectedCmd := "go test -bench '^BenchmarkProcessBatch$' -run '^$' ./pkg/worker"
	if bench.ExecutionCommand != expectedCmd {
		t.Fatalf("expected exec cmd %q, got %q", expectedCmd, bench.ExecutionCommand)
	}

	example := targets[1]
	if example.Function != "ExampleProcessBatch" {
		t.Fatalf("expected ExampleProcessBatch, got %s", example.Function)
	}
}

func TestGoDiscoverer_RootDirectoryTarget(t *testing.T) {
	t.Parallel()

	d := NewGoDiscoverer()
	content := []byte(`package root_test

import "testing"

func TestRootLevel(t *testing.T) {}
`)

	targets, err := d.Discover(context.Background(), "/root", "root_test.go", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	expectedCmd := "go test -run '^TestRootLevel$' ."
	if targets[0].ExecutionCommand != expectedCmd {
		t.Fatalf("expected exec cmd %q, got %q", expectedCmd, targets[0].ExecutionCommand)
	}
}

func TestPythonDiscoverer_LanguageAndExtensions(t *testing.T) {
	t.Parallel()

	d := NewPythonDiscoverer()
	if d.Language() != "python" {
		t.Fatalf("expected python, got %s", d.Language())
	}
	if !d.CanHandle("tests/test_service.py") || !d.CanHandle("tests/service_test.py") {
		t.Fatal("expected CanHandle to accept valid python test files")
	}
	if d.CanHandle("tests/service.py") || d.CanHandle("tests/test_service.go") {
		t.Fatal("expected CanHandle to reject non-python test files")
	}
}

func TestPythonDiscoverer_ClassesAndDecorators(t *testing.T) {
	t.Parallel()

	d := NewPythonDiscoverer()
	content := []byte(`
# Validates: CRIT-PY-AUTH-001
# Requirement: REQ-PY-AUTH
# tag: auth-suite
@pytest.mark.security
class TestAuthSecurity:
    @pytest.mark.crypto
    def test_cipher_negotiation(self):
        pass

def test_global_helper_test():
    pass
`)

	targets, err := d.Discover(context.Background(), "/root", "tests/test_security.py", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	methodTarget := targets[0]
	if methodTarget.Suite != "TestAuthSecurity" || methodTarget.Function != "test_cipher_negotiation" {
		t.Fatalf("unexpected method target: %+v", methodTarget)
	}
	expectedMethodCmd := "pytest tests/test_security.py::TestAuthSecurity::test_cipher_negotiation"
	if methodTarget.ExecutionCommand != expectedMethodCmd {
		t.Fatalf("expected cmd %q, got %q", expectedMethodCmd, methodTarget.ExecutionCommand)
	}

	globalTarget := targets[1]
	if globalTarget.Function != "test_global_helper_test" {
		t.Fatalf("unexpected global target: %+v", globalTarget)
	}
	expectedGlobalCmd := "pytest tests/test_security.py::test_global_helper_test"
	if globalTarget.ExecutionCommand != expectedGlobalCmd {
		t.Fatalf("expected cmd %q, got %q", expectedGlobalCmd, globalTarget.ExecutionCommand)
	}
}

func TestTypeScriptDiscoverer_LanguageAndExtensions(t *testing.T) {
	t.Parallel()

	d := NewTypeScriptDiscoverer()
	if d.Language() != "typescript" {
		t.Fatalf("expected typescript, got %s", d.Language())
	}

	accepted := []string{
		"auth.test.ts", "auth.spec.ts",
		"component.test.tsx", "component.spec.tsx",
		"legacy.test.js", "legacy.spec.js",
	}
	for _, path := range accepted {
		if !d.CanHandle(path) {
			t.Fatalf("expected CanHandle to accept %s", path)
		}
	}

	if d.CanHandle("index.ts") || d.CanHandle("component.tsx") || d.CanHandle("app.js") {
		t.Fatal("expected CanHandle to reject non-test files")
	}
}

func TestTypeScriptDiscoverer_SuitesAndBlocks(t *testing.T) {
	t.Parallel()

	d := NewTypeScriptDiscoverer()
	content := []byte(`
/*
 * Criteria: CRIT-TS-UI-001
 * Req: REQ-TS-UI
 * Scenario: button-click
 */
describe('ButtonComponent', () => {
    it('renders with primary variant', () => {});
    test('triggers onClick callback', () => {});
});

suite('LegacySuite', () => {
    it('legacy test case', () => {});
});
`)

	targets, err := d.Discover(context.Background(), "/root", "src/components/Button.test.tsx", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	t0 := targets[0]
	if t0.Suite != "ButtonComponent" || t0.Function != "ButtonComponent > renders with primary variant" {
		t.Fatalf("unexpected target 0: %+v", t0)
	}
	expectedCmd := "npm test -- -t 'renders with primary variant'"
	if t0.ExecutionCommand != expectedCmd {
		t.Fatalf("expected exec cmd %q, got %q", expectedCmd, t0.ExecutionCommand)
	}

	t1 := targets[1]
	if t1.Suite != "ButtonComponent" || t1.Function != "ButtonComponent > triggers onClick callback" {
		t.Fatalf("unexpected target 1: %+v", t1)
	}

	t2 := targets[2]
	if t2.Suite != "LegacySuite" || t2.Function != "LegacySuite > legacy test case" {
		t.Fatalf("unexpected target 2: %+v", t2)
	}
}

type mockDiscoverer struct {
	lang      string
	extension string
	shouldErr bool
}

func (m *mockDiscoverer) Language() string {
	return m.lang
}

func (m *mockDiscoverer) CanHandle(relPath string) bool {
	return filepath.Ext(relPath) == m.extension
}

func (m *mockDiscoverer) Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error) {
	if m.shouldErr {
		return nil, errors.New("simulated mock discoverer error")
	}
	return []DiscoveredTarget{
		{
			ID:       "mock:" + relPath,
			Path:     relPath,
			Language: m.lang,
			Function: "TestMockGenerated",
			Line:     1,
		},
	}, nil
}

func TestEngine_CustomDiscovererAndLanguageFilter(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mockFile := filepath.Join(tmpDir, "custom.mock")
	err := fileutil.WriteFile(mockFile, []byte("mock file body"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write mock file: %v", err)
	}

	e := NewEngine()
	mockDisc := &mockDiscoverer{
		lang:      "mocklang",
		extension: ".mock",
		shouldErr: false,
	}
	e.RegisterDiscoverer(mockDisc)

	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Languages:   []string{"mocklang"},
		Workers:     1,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 || targets[0].Language != "mocklang" {
		t.Fatalf("expected 1 mock target, got %v", targets)
	}

	optsExclude := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Languages:   []string{"go"},
		Workers:     1,
	}
	targetsExclude, errExclude := e.Discover(context.Background(), optsExclude)
	if errExclude != nil {
		t.Fatalf("unexpected error: %v", errExclude)
	}
	if len(targetsExclude) != 0 {
		t.Fatalf("expected 0 targets when filtered out, got %d", len(targetsExclude))
	}
}

func TestEngine_CustomDiscovererErrorToleration(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	errFile := filepath.Join(tmpDir, "failing.mockerr")
	err := fileutil.WriteFile(errFile, []byte("failing content"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	e := NewEngine()
	failingDisc := &mockDiscoverer{
		lang:      "mockerr",
		extension: ".mockerr",
		shouldErr: true,
	}
	e.RegisterDiscoverer(failingDisc)

	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Languages:   []string{"mockerr"},
		Workers:     2,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("expected discoverer errors to be tolerated, got error %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected 0 targets from failing discoverer, got %d", len(targets))
	}
}

func TestEngine_ContextCancellationHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for i := 0; i < 5; i++ {
		p := filepath.Join(tmpDir, "sub_test.go")
		err := fileutil.WriteFile(p, []byte("package sub\nfunc TestA(t *testing.T){}\n"), paths.FilePerm600)
		if err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e := NewEngine()
	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Workers:     2,
	}

	cancelledTargets, err := e.Discover(ctx, opts)
	if err == nil {
		t.Fatal("expected context error on pre-cancelled context, got nil")
	}
	if cancelledTargets != nil {
		t.Fatalf("expected nil targets on error, got %v", cancelledTargets)
	}
}

func TestEngine_DefaultWorkersAndProjectRoot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "root_test.go")
	err := fileutil.WriteFile(goFile, []byte("package root\nfunc TestDefault(t *testing.T){}\n"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	e := NewEngine()
	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Workers:     -5,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 discovered target, got %d", len(targets))
	}
}

func TestEngine_DirectoryExclusionsAndSpecialDirs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	nodeDir := filepath.Join(tmpDir, "node_modules")
	err := fileutil.MkdirAll(nodeDir, paths.DirPerm750)
	if err != nil {
		t.Fatalf("failed to mkdir node_modules: %v", err)
	}
	err = fileutil.WriteFile(filepath.Join(nodeDir, "ignored.test.js"), []byte("test('skip', ()=>{})"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write file in node_modules: %v", err)
	}

	customDir := filepath.Join(tmpDir, "custom_skip")
	err = fileutil.MkdirAll(customDir, paths.DirPerm750)
	if err != nil {
		t.Fatalf("failed to mkdir custom_skip: %v", err)
	}
	err = fileutil.WriteFile(filepath.Join(customDir, "skipped_test.go"), []byte("package skip\nfunc TestSkip(t *testing.T){}"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write file in custom_skip: %v", err)
	}

	validDir := filepath.Join(tmpDir, "valid")
	err = fileutil.MkdirAll(validDir, paths.DirPerm750)
	if err != nil {
		t.Fatalf("failed to mkdir valid: %v", err)
	}
	err = fileutil.WriteFile(filepath.Join(validDir, "valid_test.go"), []byte("package valid\nfunc TestValid(t *testing.T){}"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write valid test: %v", err)
	}

	e := NewEngine()
	opts := DiscoveryOptions{
		ProjectRoot:  tmpDir,
		ExcludePaths: []string{"custom_skip"},
		Workers:      2,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(targets) != 1 || targets[0].Function != "TestValid" {
		t.Fatalf("expected exactly 1 target TestValid, got %v", targets)
	}
}

func TestEngine_ExplicitPathsAndSymlinkBoundaries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	extDir := t.TempDir()

	// External test file outside project root
	extFile := filepath.Join(extDir, "outside_test.go")
	err := fileutil.WriteFile(extFile, []byte("package ext\nfunc TestOutside(t *testing.T){}"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write external file: %v", err)
	}

	// Internal valid path
	subDir := filepath.Join(tmpDir, "scoped")
	err = fileutil.MkdirAll(subDir, paths.DirPerm750)
	if err != nil {
		t.Fatalf("failed to create subDir: %v", err)
	}
	scopedFile := filepath.Join(subDir, "scoped_test.go")
	err = fileutil.WriteFile(scopedFile, []byte("package scoped\nfunc TestScoped(t *testing.T){}"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write scoped file: %v", err)
	}

	// Symlinked directory pointing outside project root
	symDirLink := filepath.Join(tmpDir, "symlinked_ext_dir")
	err = fileutil.Symlink(extDir, symDirLink)
	if err != nil {
		t.Fatalf("failed to symlink external dir: %v", err)
	}

	// Symlinked file pointing outside project root
	symFileLink := filepath.Join(tmpDir, "symlinked_ext_file_test.go")
	err = fileutil.Symlink(extFile, symFileLink)
	if err != nil {
		t.Fatalf("failed to symlink external file: %v", err)
	}

	e := NewEngine()
	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Paths:       []string{"scoped", "symlinked_ext_dir"},
		Workers:     2,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error discovering scoped and symlinked paths: %v", err)
	}

	// Outside symlinks should be skipped, only scoped_test.go discovered
	if len(targets) != 1 || targets[0].Function != "TestScoped" {
		t.Fatalf("expected only 1 target from scoped path, got %v", targets)
	}
}

func TestEngine_IncrementalCacheExecution(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache", "test_cache.json")

	testFile := filepath.Join(tmpDir, "sample_test.go")
	err := fileutil.WriteFile(testFile, []byte("package sample\nfunc TestCached(t *testing.T){}\n"), paths.FilePerm600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	e := NewEngine()
	opts := DiscoveryOptions{
		ProjectRoot: tmpDir,
		Incremental: true,
		CachePath:   cachePath,
		Workers:     2,
	}

	t1, err1 := e.Discover(context.Background(), opts)
	if err1 != nil {
		t.Fatalf("run 1 failed: %v", err1)
	}
	if len(t1) != 1 {
		t.Fatalf("expected 1 target in run 1, got %d", len(t1))
	}

	t2, err2 := e.Discover(context.Background(), opts)
	if err2 != nil {
		t.Fatalf("run 2 failed: %v", err2)
	}
	if len(t2) != 1 || t2[0].Function != "TestCached" {
		t.Fatalf("expected 1 cached target in run 2, got %v", t2)
	}
}

func verifyTestCaseFields(t *testing.T, obj map[string]any, expectedCat string, expectSuites bool) {
	t.Helper()

	if obj[objects.FieldKeyKind] != objects.KindTestCase {
		t.Fatalf("expected kind test_case, got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyCategory] != expectedCat {
		t.Fatalf("expected category %s, got %v", expectedCat, obj[objects.FieldKeyCategory])
	}

	suites, hasSuites := obj[objects.FieldKeyVerificationSuites]
	if expectSuites {
		if !hasSuites || len(suites.([]string)) != 1 {
			t.Fatalf("expected 1 verification suite, got %v", suites)
		}
	} else if hasSuites {
		t.Fatalf("expected no verification suites when execCmd empty, got %v", suites)
	}
}

func TestEngine_GenerateTestCaseObject_Categories(t *testing.T) {
	t.Parallel()

	e := NewEngine()

	cases := []struct {
		name        string
		tags        []string
		execCmd     string
		category    string
		expectSuite bool
	}{
		{"unit", []string{"fast", "core"}, "go test ./pkg/core", "unit", true},
		{"integration", []string{"integration", "db"}, "cargo test integration", "integration", true},
		{"e2e", []string{"E2E-browser"}, "npm test", "e2e", true},
		{"perf", []string{"benchmark", "load"}, "go test -bench .", "performance", true},
		{"empty-cmd", nil, "", "unit", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			target := DiscoveredTarget{
				Language:         "go",
				Path:             "pkg/demo/demo_test.go",
				Function:         "TestDemoFunc",
				Line:             42,
				Tags:             tc.tags,
				CriteriaRefs:     []string{"CRIT-DEMO-001"},
				RequirementRefs:  []string{"REQ-DEMO-001"},
				ExecutionCommand: tc.execCmd,
			}

			obj, err := e.GenerateTestCaseObject(target, "/root")
			if err != nil {
				t.Fatalf("unexpected error generating test case object: %v", err)
			}
			verifyTestCaseFields(t, obj, tc.category, tc.expectSuite)
		})
	}
}

func TestTypes_CommentMetadataExtraction(t *testing.T) {
	t.Parallel()

	crit, req, tags := extractCommentMetadata("// criteria: C1, C2\n// req: R1\n// tag: security")
	if len(crit) != 2 || crit[0] != "C1" || crit[1] != "C2" {
		t.Fatalf("unexpected criteria: %v", crit)
	}
	if len(req) != 1 || req[0] != "R1" {
		t.Fatalf("unexpected req: %v", req)
	}
	if len(tags) != 1 || tags[0] != "security" {
		t.Fatalf("unexpected tags: %v", tags)
	}

	critEmpty, reqEmpty, tagsEmpty := extractCommentMetadata("// regular comment without metadata")
	if len(critEmpty) != 0 || len(reqEmpty) != 0 || len(tagsEmpty) != 0 {
		t.Fatalf("expected empty metadata, got crit=%v, req=%v, tags=%v", critEmpty, reqEmpty, tagsEmpty)
	}
}

func TestDiscovery_ConcurrencySafety(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "concurrent_cache.json")
	cache := LoadCache(cachePath)

	const workerCount = 8
	const iterations = 50
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("testdiscovery.test_worker", "concurrent cache read/write").StartSimple(func() {
			defer wg.Done()
			now := time.Now()
			content := []byte("func TestConcurrency(t *testing.T){}")
			relPath := filepath.Join("pkg", "test", "concurrent_test.go")

			for j := 0; j < iterations; j++ {
				if workerID%2 == 0 {
					cache.Set(relPath, now, content, []DiscoveredTarget{
						{Function: "TestConcurrency", Language: "go", Path: relPath},
					})
				} else {
					gotTargets, gotOk := cache.Get(relPath, now, content)
					if gotOk && len(gotTargets) == 0 {
						t.Errorf("unexpected empty targets for hit")
					}
				}
			}
		})
	}

	wg.Wait()

	err := cache.Save()
	if err != nil {
		t.Fatalf("failed to save cache after concurrent access: %v", err)
	}
}
