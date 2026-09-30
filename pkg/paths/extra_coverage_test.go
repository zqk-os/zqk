package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExtraCoverage_AgentRuntimeAndWorktree(t *testing.T) {
	tmpDir := t.TempDir()

	// ResolveAgentRuntimePath
	p := ResolveAgentRuntimePath(tmpDir, "alias", "sync.json")
	if p == "" {
		t.Error("expected ResolveAgentRuntimePath non-empty")
	}

	// AgentWorktreeLookupDirs
	dirs := AgentWorktreeLookupDirs(tmpDir, "task-123")
	if len(dirs) == 0 {
		t.Error("expected lookup dirs")
	}

	// BootstrapWorktreeSettings
	err := BootstrapWorktreeSettings(tmpDir, tmpDir)
	if err != nil {
		t.Errorf("BootstrapWorktreeSettings failed: %v", err)
	}

	// SettingsPathForRoot
	sp := SettingsPathForRoot(tmpDir)
	if sp != filepath.Join(tmpDir, "config", "zqk-local.yaml") {
		t.Errorf("SettingsPathForRoot got %s", sp)
	}
}

func TestExtraCoverage_CLIAndModule(t *testing.T) {
	// IsProductCLIVerb
	if !IsProductCLIVerb("object") || !IsProductCLIVerb("system") {
		t.Error("expected 'object' and 'system' to be product CLI verbs")
	}
	if IsProductCLIVerb("nonexistentverb123") {
		t.Error("expected unknown verb to be false")
	}

	// isUnsafeProductCLI
	if !isUnsafeProductCLI("/tmp/foo.test") || !isUnsafeProductCLI("/tmp/main") {
		t.Error("expected .test and main to be unsafe")
	}
	if isUnsafeProductCLI("./bin/zqk") {
		t.Error("expected ./bin/zqk to be safe")
	}

	// ConfigImportPath & ObjectsImportPath
	cip := ConfigImportPath("/tmp/some/module")
	if cip != "/tmp/some/module/pkg/config" {
		t.Errorf("ConfigImportPath got %s", cip)
	}
	oip := ObjectsImportPath("/tmp/some/module")
	if oip != "/tmp/some/module/pkg/objects" {
		t.Errorf("ObjectsImportPath got %s", oip)
	}

	// SetNamespacePrefix
	origPrefix := NamespacePrefix
	SetNamespacePrefix("custom_prefix")
	if NamespacePrefix != "custom_prefix" {
		t.Errorf("expected NamespacePrefix custom_prefix, got %s", NamespacePrefix)
	}
	SetNamespacePrefix(origPrefix)
}

func TestExtraCoverage_ProjectRootDiscovery(t *testing.T) {
	// isIgnoredNestedProjectRoot
	ignoredPath := filepath.Join("foo", "cmd", "zqk", "system")
	if !isIgnoredNestedProjectRoot(ignoredPath) {
		t.Error("expected foo/cmd/zqk/system to be ignored")
	}
	if !isIgnoredNestedProjectRoot(filepath.Join("pkg", "mytest", "testdata", "fixture")) {
		t.Error("expected testdata path to be ignored")
	}
	if !isIgnoredNestedProjectRoot(filepath.Join("pkg", ".tmp_build", "sub")) {
		t.Error("expected .tmp path to be ignored")
	}
	if isIgnoredNestedProjectRoot(filepath.Join("foo", "cmd", "zqk", "other")) {
		t.Error("expected foo/cmd/zqk/other not to be ignored")
	}

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub1", "sub2")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir subDir failed: %v", err)
	}

	// Create .zqk directly in tmpDir so tmpDir is closest candidate
	zqkDir := filepath.Join(tmpDir, ".zqk")
	if err := os.MkdirAll(zqkDir, 0755); err != nil {
		t.Fatalf("mkdir zqkDir failed: %v", err)
	}

	nearest := FindNearestProjectRoot(subDir)
	if nearest != tmpDir {
		t.Errorf("FindNearestProjectRoot got %s, want %s", nearest, tmpDir)
	}

	// Create workspace marker .zqk-workspace in tmpDir
	markerPath := filepath.Join(tmpDir, ".zqk-workspace")
	if err := os.WriteFile(markerPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write marker failed: %v", err)
	}

	foundWs := FindWorkspaceRoot(subDir)
	if foundWs != "" && foundWs != tmpDir {
		t.Errorf("FindWorkspaceRoot got %s, want %s", foundWs, tmpDir)
	}

	// ReadPersistedCurrentRoot
	_ = ReadPersistedCurrentRoot(subDir)

	// ResolveProjectRoot with env overrides
	prevPR := zqkenv.ProjectRoot().Get()
	zqkenv.ProjectRoot().Set(tmpDir)
	if pr := ResolveProjectRoot(subDir); pr != tmpDir {
		t.Errorf("ResolveProjectRoot with ProjectRoot env got %s, want %s", pr, tmpDir)
	}
	zqkenv.ProjectRoot().Set(prevPR)

	prevTR := zqkenv.TestRoot().Get()
	zqkenv.TestRoot().Set(tmpDir)
	if tr := ResolveProjectRoot(subDir); tr != tmpDir {
		t.Errorf("ResolveProjectRoot with TestRoot env got %s, want %s", tr, tmpDir)
	}
	zqkenv.TestRoot().Set(prevTR)

	// PathResolver nil receiver coverage
	var nilResolver *pathResolver
	if nilResolver.ProjectRoot() != "" {
		t.Error("expected empty ProjectRoot for nilResolver")
	}
	if _, err := nilResolver.ResolveStrict("ref"); err != ErrPathAliasNotInCache {
		t.Errorf("expected ErrPathAliasNotInCache for nilResolver.ResolveStrict, got %v", err)
	}
	if nilResolver.ResolveFromCacheOrConstant("alias", "rel") != "" {
		t.Error("expected empty string for nilResolver.ResolveFromCacheOrConstant")
	}
}

func TestExtraCoverage_ResolverAndCache(t *testing.T) {
	tmpDir := t.TempDir()

	// BuildPathAliasCache
	BuildPathAliasCache(tmpDir, map[string]string{"process": ".zqk/process"})

	// DefaultStalenessCheckDirs
	staleDirs := DefaultStalenessCheckDirs()
	if len(staleDirs) == 0 {
		t.Error("expected non-empty DefaultStalenessCheckDirs")
	}

	// IsPathCacheStale
	stale := IsPathCacheStale(tmpDir, staleDirs)
	if stale {
		t.Error("newly initialized path cache should not be stale")
	}

	// ResolvePath
	resolved := ResolvePath(tmpDir, "process")
	if resolved == "" {
		t.Error("expected non-empty ResolvePath for process")
	}
	// resolvePrefix
	pref := resolvePrefix(tmpDir, "process")
	if pref == "" {
		t.Error("expected non-empty resolvePrefix for process")
	}

	// ResolvePathStrict with various schemes
	_, _ = ResolvePathStrict(tmpDir, "abs:"+tmpDir)
	_, _ = ResolvePathStrict(tmpDir, "prefix:process")
	_, _ = ResolvePathStrict(tmpDir, "invalid_scheme:foo")
	_, _ = ResolvePathStrict("", "process")

	// Layout helpers
	_ = CredentialsPath(tmpDir)
	_ = ProjectDataDirPath(tmpDir)
	_ = WorktreeLocalConfigRelYml()
}

func TestExtraCoverage_RuntimeLayoutYAMLPaths(t *testing.T) {
	tmpDir := t.TempDir()

	cas := casYAMLPath(tmpDir, "test-cas")
	if cas == "" {
		t.Error("expected casYAMLPath non-empty")
	}

	persona := PersonaYAMLPath(tmpDir, "PER-123")
	if persona == "" {
		t.Error("expected PersonaYAMLPath non-empty")
	}

	accIndex := AccountIndexPath(tmpDir)
	if accIndex == "" {
		t.Error("expected AccountIndexPath non-empty")
	}

	accYAML := AccountYAMLPath(tmpDir, "ACC-123")
	if accYAML == "" {
		t.Error("expected AccountYAMLPath non-empty")
	}

	roleIndex := RoleIndexPath(tmpDir)
	if roleIndex == "" {
		t.Error("expected RoleIndexPath non-empty")
	}

	roleYAML := RoleYAMLPath(tmpDir, "ROL-123")
	if roleYAML == "" {
		t.Error("expected RoleYAMLPath non-empty")
	}

	// DomainTreeStamp
	stamp := DomainTreeStamp(tmpDir)
	_ = stamp

	// StableBinaryCandidates
	if cands := StableBinaryCandidates(tmpDir); len(cands) != 2 {
		t.Errorf("expected 2 stable binary candidates, got %d", len(cands))
	}
	if cands := StableBinaryCandidates(""); cands != nil {
		t.Errorf("expected nil candidates for empty root")
	}

	// CredentialsPath & ProjectDataDirPath
	if cp := CredentialsPath(tmpDir); cp == "" {
		t.Error("expected non-empty CredentialsPath")
	}
	if cp := CredentialsPath(""); cp != "" {
		t.Error("expected empty CredentialsPath for empty root")
	}
	if pd := ProjectDataDirPath(tmpDir); pd == "" {
		t.Error("expected non-empty ProjectDataDirPath")
	}
	if pd := ProjectDataDirPath(""); pd != "" {
		t.Error("expected empty ProjectDataDirPath for empty root")
	}

	// ObjectSpecFileName
	if fn := ObjectSpecFileName("goal"); fn != "goal.yaml" {
		t.Errorf("ObjectSpecFileName got %s, want goal.yaml", fn)
	}
	if fn := ObjectSpecFileName(""); fn != "" {
		t.Errorf("ObjectSpecFileName got %s for empty kind", fn)
	}

	// PathResolver nil and valid methods
	var nilResolver *pathResolver
	if nilResolver.ProjectRoot() != "" {
		t.Error("expected empty project root for nilResolver")
	}
	if _, err := nilResolver.ResolveStrict("prefix:foo"); err == nil {
		t.Error("expected error for nilResolver.ResolveStrict")
	}
	if res := nilResolver.ResolveFromCacheOrConstant("foo", "bar"); res != "" {
		t.Error("expected empty for nilResolver.ResolveFromCacheOrConstant")
	}
	validResolver := NewPathResolver(tmpDir)
	if validResolver.ProjectRoot() != tmpDir {
		t.Errorf("validResolver ProjectRoot got %s", validResolver.ProjectRoot())
	}
	_ = validResolver.ResolveFromCacheOrConstant("process", "fallback")
}
