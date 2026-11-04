package paths

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCycle154_CLICommandNameAndInvocations(t *testing.T) {
	if bName := BrandExecutableName(); bName != brand.ExecutableName() {
		t.Fatalf("expected BrandExecutableName %q, got %q", brand.ExecutableName(), bName)
	}

	cli := CLIName()
	if cli == "" {
		t.Fatal("expected non-empty CLIName")
	}

	usage := CLIUsage("goal", "list", "", "  ")
	if !strings.Contains(usage, "goal list") {
		t.Fatalf("unexpected CLIUsage output: %s", usage)
	}

	chained := CLIInvocation("zqk goal list && git status && zqk req list")
	if !strings.Contains(chained, "goal list") || !strings.Contains(chained, "git status") {
		t.Fatalf("unexpected CLIInvocation chained output: %s", chained)
	}

	if empty := CLIInvocation(""); empty != "" {
		t.Fatalf("expected empty string for empty invocation, got %q", empty)
	}

	gitCmd := CLIInvocation("git commit -m 'test'")
	if gitCmd != "git commit -m 'test'" {
		t.Fatalf("expected git command unchanged, got %q", gitCmd)
	}

	if rew := RewriteCanonicalCLIInvocations(""); rew != "" {
		t.Fatalf("expected empty string from RewriteCanonicalCLIInvocations, got %q", rew)
	}

	rewritten := RewriteCanonicalCLIInvocations("run zqk goal list now")
	if !strings.Contains(rewritten, "goal list") {
		t.Fatalf("unexpected rewritten prose: %s", rewritten)
	}
}

func TestCycle154_YAMLIndexAndVariants(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	if !isHexHashStem(validHex) {
		t.Fatal("expected 64-char hex to be valid")
	}
	if isHexHashStem(strings.Repeat("a", 63)) {
		t.Fatal("expected 63-char hex to be invalid")
	}
	if isHexHashStem(strings.Repeat("a", 65)) {
		t.Fatal("expected 65-char hex to be invalid")
	}
	invalidHex := strings.Repeat("a", 63) + "z"
	if isHexHashStem(invalidHex) {
		t.Fatal("expected non-hex char in 64-char string to be invalid")
	}

	tmpDir := t.TempDir()
	p1 := filepath.Join(tmpDir, "test_variant.yaml")
	if err := fileutil.WriteFile(p1, []byte("data: 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if hit := ResolveYAMLVariant(tmpDir, "test-variant"); hit != p1 {
		t.Fatalf("expected ResolveYAMLVariant to find %s, got %s", p1, hit)
	}
	if hit := ResolveYAMLVariant(tmpDir, "nonexistent"); hit != "" {
		t.Fatalf("expected empty for missing variant, got %s", hit)
	}

	dotFile := filepath.Join(tmpDir, ".hidden.yaml")
	if err := fileutil.WriteFile(dotFile, []byte("hidden: 1"), 0644); err != nil {
		t.Fatalf("write dotfile: %v", err)
	}
	hexFile := filepath.Join(tmpDir, validHex+".yaml")
	if err := fileutil.WriteFile(hexFile, []byte("cas: 1"), 0644); err != nil {
		t.Fatalf("write hexfile: %v", err)
	}

	idx := IndexYAMLNames(tmpDir)
	if idx == nil || idx["test_variant"] == "" {
		t.Fatal("expected IndexYAMLNames to contain test_variant")
	}
	if _, ok := idx[".hidden"]; ok {
		t.Fatal("expected dotfile to be skipped in IndexYAMLNames")
	}
	if _, ok := idx[validHex]; ok {
		t.Fatal("expected hex CAS file to be skipped in IndexYAMLNames")
	}
	if emptyIdx := IndexYAMLNames(""); len(emptyIdx) != 0 {
		t.Fatalf("expected empty map for empty dir, got %v", emptyIdx)
	}
}

func TestCycle154_SpecLayoutAndCandidates(t *testing.T) {
	if ObjectSpecFileName("") != "" {
		t.Fatal("expected empty filename for empty kind")
	}
	if ObjectSpecFileName("goal") != "goal.yaml" {
		t.Fatalf("expected goal.yaml, got %s", ObjectSpecFileName("goal"))
	}

	if DomainPathCandidates("", "foo") != nil || DomainPathCandidates("dir", "") != nil {
		t.Fatal("expected nil candidates for empty arguments")
	}

	tmpDir := t.TempDir()
	cands := DomainPathCandidates(tmpDir, "sample.yaml")
	if len(cands) != 1+len(ObjectSpecDomainDirs) {
		t.Fatalf("expected %d candidates, got %d", 1+len(ObjectSpecDomainDirs), len(cands))
	}

	specCands := ObjectSpecPathCandidates(tmpDir, "goal")
	if len(specCands) != len(cands) {
		t.Fatalf("unexpected ObjectSpecPathCandidates count: %d", len(specCands))
	}

	if DomainTreeStamp("") != 0 {
		t.Fatal("expected 0 stamp for empty dir")
	}

	kernelDir := filepath.Join(tmpDir, "kernel")
	if err := fileutil.MkdirAll(kernelDir, 0755); err != nil {
		t.Fatalf("mkdir kernelDir: %v", err)
	}
	targetSpec := filepath.Join(kernelDir, "system.yaml")
	if err := fileutil.WriteFile(targetSpec, []byte("kind: system"), 0644); err != nil {
		t.Fatalf("write targetSpec: %v", err)
	}

	foundDomain := FindDomainFile(tmpDir, "system.yaml")
	if foundDomain != targetSpec {
		t.Fatalf("expected FindDomainFile %s, got %s", targetSpec, foundDomain)
	}
	if FindDomainFile("", "system.yaml") != "" {
		t.Fatal("expected empty for empty dir")
	}

	specHit, err := FindObjectSpecFile(tmpDir, "system")
	if err != nil || specHit != targetSpec {
		t.Fatalf("FindObjectSpecFile failed: hit=%s, err=%v", specHit, err)
	}
	if _, err := FindObjectSpecFile("", "system"); err == nil {
		t.Fatal("expected error for empty specsDir")
	}
	if _, err := FindObjectSpecFile(tmpDir, "missing_kind"); err == nil {
		t.Fatal("expected error for missing object spec")
	}

	lcTarget := filepath.Join(kernelDir, "system_lifecycle.yaml")
	if err := fileutil.WriteFile(lcTarget, []byte("lifecycle: test"), 0644); err != nil {
		t.Fatalf("write lcTarget: %v", err)
	}
	if lchit := FindLifecycleFile(tmpDir, "system"); lchit != lcTarget {
		t.Fatalf("expected FindLifecycleFile %s, got %s", lcTarget, lchit)
	}
	if FindLifecycleFile(tmpDir, "") != "" {
		t.Fatal("expected empty for empty kind")
	}
}

func TestCycle154_ModuleRootAndImportPaths(t *testing.T) {
	if _, err := ModuleRootFromPath("/nonexistent/path/that/cannot/exist"); err == nil {
		t.Fatal("expected error for non-existent path in ModuleRootFromPath")
	}

	tmpDir := t.TempDir()
	goModFile := filepath.Join(tmpDir, "go.mod")
	modContent := "// comment line\n\nmodule \"example.com/company/project\" // inline comment\n\ngo 1.24\n"
	if err := fileutil.WriteFile(goModFile, []byte(modContent), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	root, err := ModuleRootFromPath(goModFile)
	if err != nil || root != tmpDir {
		t.Fatalf("ModuleRootFromPath failed: root=%s, err=%v", root, err)
	}

	if _, err := ObjectsFieldKeysGoPath(""); err == nil {
		t.Fatal("expected error for empty moduleRoot in ObjectsFieldKeysGoPath")
	}
	fkPath, err := ObjectsFieldKeysGoPath(tmpDir)
	if err != nil || !strings.Contains(fkPath, "field_keys.go") {
		t.Fatalf("ObjectsFieldKeysGoPath unexpected: %s, err=%v", fkPath, err)
	}

	if _, err := ModuleImportPath(""); err == nil {
		t.Fatal("expected error for empty moduleRoot in ModuleImportPath")
	}
	modImport, err := ModuleImportPath(tmpDir)
	if err != nil || modImport != "example.com/company/project" {
		t.Fatalf("ModuleImportPath got %q, err=%v", modImport, err)
	}

	if _, err := parseModuleImportPath([]byte("// only comments\n")); err == nil {
		t.Fatal("expected error for missing module directive")
	}
	if _, err := parseModuleImportPath([]byte("module \"\"\n")); err == nil {
		t.Fatal("expected error for empty module path")
	}

	specPkg := SpecbuilderPackageImportPath("example.com/mod", "builders")
	if specPkg != "example.com/mod/pkg/specbuilder/builders" {
		t.Fatalf("unexpected SpecbuilderPackageImportPath: %s", specPkg)
	}
}

func TestCycle154_BrandAliasesAndSettings(t *testing.T) {
	tmpDir := t.TempDir()

	defBrandPath := BrandSettingsPath(tmpDir)
	if defBrandPath != filepath.Join(tmpDir, "config", "zqk.yaml") {
		t.Fatalf("unexpected default BrandSettingsPath: %s", defBrandPath)
	}

	cfgDir := filepath.Join(tmpDir, "config")
	if err := fileutil.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("mkdir cfgDir: %v", err)
	}
	localCfg := filepath.Join(cfgDir, "zqk-local.yaml")
	cfgData := "paths:\n  project_root: \"/custom/project/root\"\n  aliases:\n    custom_alias: \"pkg/custom\"\ncli:\n  binary_path: \"./bin/custom-cli\"\n"
	if err := fileutil.WriteFile(localCfg, []byte(cfgData), 0644); err != nil {
		t.Fatalf("write localCfg: %v", err)
	}

	if hit := BrandSettingsPath(tmpDir); hit != localCfg {
		t.Fatalf("expected BrandSettingsPath to return localCfg: %s, got %s", localCfg, hit)
	}

	if root := LoadBrandSettingsProjectRoot(""); root != "" {
		t.Fatalf("expected empty for empty projectRoot, got %s", root)
	}
	if root := LoadBrandSettingsProjectRoot(tmpDir); root != "/custom/project/root" {
		t.Fatalf("expected /custom/project/root, got %s", root)
	}

	if bin := LoadBrandCLIBinaryPath(""); bin != "" {
		t.Fatalf("expected empty bin for empty projectRoot, got %s", bin)
	}
	if bin := LoadBrandCLIBinaryPath(tmpDir); bin != "./bin/custom-cli" {
		t.Fatalf("expected ./bin/custom-cli, got %s", bin)
	}

	if aliases := LoadBrandPathAliases(""); aliases != nil {
		t.Fatalf("expected nil aliases for empty projectRoot, got %v", aliases)
	}
	aliases := LoadBrandPathAliases(tmpDir)
	if aliases == nil || aliases["custom_alias"] != "pkg/custom" {
		t.Fatalf("unexpected aliases: %v", aliases)
	}

	testCfg := filepath.Join(cfgDir, "zqk-test.yaml")
	if err := fileutil.WriteFile(testCfg, []byte("paths:\n  project_root: \"/test/root\"\n"), 0644); err != nil {
		t.Fatalf("write testCfg: %v", err)
	}
	prevTestRoot := zqkenv.TestRoot().Get()
	zqkenv.TestRoot().Set(tmpDir)
	defer zqkenv.TestRoot().Set(prevTestRoot)

	if sp := SettingsPathForRoot(tmpDir); sp != testCfg {
		t.Fatalf("expected SettingsPathForRoot %s, got %s", testCfg, sp)
	}
}

func TestCycle154_ProjectRootAndWorktreeResolvers(t *testing.T) {
	tmpDir := t.TempDir()

	if res := ReadPersistedCurrentRoot(tmpDir); res != "" {
		t.Fatalf("expected empty persisted root for empty dir, got %s", res)
	}

	zqkDir := filepath.Join(tmpDir, ".zqk")
	if err := fileutil.MkdirAll(zqkDir, 0755); err != nil {
		t.Fatalf("mkdir zqkDir: %v", err)
	}
	currentRootFile := filepath.Join(zqkDir, "current_root")
	if err := fileutil.WriteFile(currentRootFile, []byte("   \n"), 0644); err != nil {
		t.Fatalf("write currentRootFile: %v", err)
	}
	if res := ReadPersistedCurrentRoot(tmpDir); res != "" {
		t.Fatalf("expected empty persisted root for whitespace, got %s", res)
	}

	subProj := filepath.Join(tmpDir, "sub_proj")
	subZqk := filepath.Join(subProj, ".zqk")
	if err := fileutil.MkdirAll(subZqk, 0755); err != nil {
		t.Fatalf("mkdir subZqk: %v", err)
	}
	if err := fileutil.WriteFile(currentRootFile, []byte("sub_proj"), 0644); err != nil {
		t.Fatalf("write sub_proj current_root: %v", err)
	}
	if res := ReadPersistedCurrentRoot(tmpDir); res != subProj {
		t.Fatalf("expected persisted root %s, got %s", subProj, res)
	}

	if err := fileutil.WriteFile(currentRootFile, []byte("/nonexistent/external/path"), 0644); err != nil {
		t.Fatalf("write external current_root: %v", err)
	}
	if res := ReadPersistedCurrentRoot(tmpDir); res != "" {
		t.Fatalf("expected empty for external root escape, got %s", res)
	}

	if IsValidProjectRoot(tmpDir) {
		// tmpDir is under /tmp or /private/tmp so isIgnoredNestedProjectRoot is true
	}
	if IsValidProjectRoot("/nonexistent/directory/path") {
		t.Fatal("expected false for nonexistent path")
	}

	if res := resolveGitWorktreeFile(tmpDir, filepath.Join(tmpDir, "missing.git")); res != "" {
		t.Fatalf("expected empty for missing git file, got %s", res)
	}
	gitFile := filepath.Join(tmpDir, "gitfile")
	if err := fileutil.WriteFile(gitFile, []byte("not_a_gitdir"), 0644); err != nil {
		t.Fatalf("write gitfile: %v", err)
	}
	if res := resolveGitWorktreeFile(tmpDir, gitFile); res != "" {
		t.Fatalf("expected empty for invalid gitfile prefix, got %s", res)
	}

	if wtRoot := FindWorktreeProjectRoot(tmpDir); wtRoot != "" {
		t.Fatalf("expected empty for normal tmpDir, got %s", wtRoot)
	}
}

func TestCycle154_AgentWorktreeLifecycle(t *testing.T) {
	k1 := worktreeRepoKey("/")
	if !strings.HasPrefix(k1, "repo-") {
		t.Fatalf("expected root key to start with repo-, got %s", k1)
	}
	k2 := worktreeRepoKey("")
	if !strings.HasPrefix(k2, "repo-") {
		t.Fatalf("expected empty key to start with repo-, got %s", k2)
	}

	if !pathUnderRoot("/a/b", "/a/b/c/d") {
		t.Fatal("expected /a/b/c/d under /a/b")
	}
	if !pathUnderRoot("/a/b", "/a/b") {
		t.Fatal("expected /a/b under /a/b")
	}
	if pathUnderRoot("/a/b", "/a/c") {
		t.Fatal("expected /a/c not under /a/b")
	}

	if IsAgentWorktreePath("") || IsAgentWorktreePath(".") {
		t.Fatal("expected empty or dot not to be agent worktree path")
	}
	sep := string(filepath.Separator)
	pBucket := sep + "zqk-worktrees" + sep + "task-1"
	if !IsAgentWorktreePath(pBucket) {
		t.Fatalf("expected %s to be agent worktree path", pBucket)
	}
	pLegacy := sep + ".zqk" + sep + "worktrees" + sep + "task-1"
	if !IsAgentWorktreePath(pLegacy) {
		t.Fatalf("expected %s to be agent worktree path", pLegacy)
	}

	tmpDir := t.TempDir()
	prevWtRoot := zqkenv.AgentWorktreeRoot().Get()
	zqkenv.AgentWorktreeRoot().Set(tmpDir)
	defer zqkenv.AgentWorktreeRoot().Set(prevWtRoot)

	customWt := filepath.Join(tmpDir, "worker-task")
	if !IsAgentWorktreePath(customWt) {
		t.Fatalf("expected %s under override to be agent worktree path", customWt)
	}

	if _, err := AgentWorktreeMainRepo(""); err == nil {
		t.Fatal("expected error for empty worktree root in AgentWorktreeMainRepo")
	}
	if _, err := gitCommonWorkingTree(tmpDir); err == nil {
		t.Fatal("expected error for non-git dir in gitCommonWorkingTree")
	}

	wtDir := t.TempDir()
	seatedDir := t.TempDir()
	if err := BootstrapWorktreeConfig(wtDir, seatedDir); err != nil {
		t.Fatalf("BootstrapWorktreeConfig failed: %v", err)
	}
	loadedSeated := LoadBrandSettingsProjectRoot(wtDir)
	if loadedSeated != seatedDir {
		t.Fatalf("expected seatedDir %s, got %s", seatedDir, loadedSeated)
	}
}

func TestCycle154_ProjectRootNestingValidation(t *testing.T) {
	if err := ValidateProjectRootNesting(""); err == nil {
		t.Fatal("expected error for empty candidate root in ValidateProjectRootNesting")
	}

	tmpDir := t.TempDir()
	if err := ValidateProjectRootNesting(tmpDir); err != nil {
		t.Fatalf("expected clean dir to pass ValidateProjectRootNesting, got %v", err)
	}

	childDir := filepath.Join(tmpDir, "child_module")
	childZqk := filepath.Join(childDir, ".zqk")
	if err := fileutil.MkdirAll(childZqk, 0755); err != nil {
		t.Fatalf("mkdir childZqk: %v", err)
	}

	nested, err := FindNestedProjectRoots(tmpDir)
	if err != nil {
		t.Fatalf("FindNestedProjectRoots failed: %v", err)
	}
	if len(nested) != 1 || nested[0] != childDir {
		t.Fatalf("expected nested root %s, got %v", childDir, nested)
	}

	if isIgnoredScanDir(".zqk", filepath.Join(tmpDir, ".zqk")) != true {
		t.Fatal("expected .zqk to be ignored in scan")
	}
	if isIgnoredScanDir("node_modules", filepath.Join(tmpDir, "node_modules")) != true {
		t.Fatal("expected node_modules to be ignored in scan")
	}

	purged, err := PurgeNestedProjectRoots(tmpDir)
	if err != nil {
		t.Fatalf("PurgeNestedProjectRoots failed: %v", err)
	}
	if len(purged) != 1 {
		t.Fatalf("expected 1 purged dir, got %v", purged)
	}

	nestedAfter, err := FindNestedProjectRoots(tmpDir)
	if err != nil || len(nestedAfter) != 0 {
		t.Fatalf("expected 0 nested roots after purge, got %v (err=%v)", nestedAfter, err)
	}
}

func TestCycle154_ProductCLICandidatesAndMCPServe(t *testing.T) {
	if cands := ProductCLICandidates(""); cands != nil {
		t.Fatalf("expected nil candidates for empty projectRoot, got %v", cands)
	}

	tmpDir := t.TempDir()
	cands := ProductCLICandidates(tmpDir)
	if len(cands) == 0 {
		t.Fatal("expected non-empty ProductCLICandidates for valid root")
	}

	if existingFile("") || existingFile(tmpDir) {
		t.Fatal("expected false for empty path or directory in existingFile")
	}
	dummyFile := filepath.Join(tmpDir, "dummy")
	if err := fileutil.WriteFile(dummyFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write dummyFile: %v", err)
	}
	if !existingFile(dummyFile) {
		t.Fatal("expected true for existing regular file")
	}

	cmd1 := MCPServeCommandLine("custom-binary --flag 123", tmpDir)
	if cmd1 != "custom-binary --flag 123" {
		t.Fatalf("unexpected MCPServeCommandLine with override: %s", cmd1)
	}

	cmd2 := MCPServeCommandLine("", tmpDir)
	if !strings.Contains(cmd2, "mcp serve") {
		t.Fatalf("unexpected MCPServeCommandLine default: %s", cmd2)
	}

	prevBin := zqkenv.Bin().Get()
	zqkenv.Bin().Set(dummyFile)
	defer zqkenv.Bin().Set(prevBin)

	resolved := resolveProductCLI(tmpDir)
	if resolved != dummyFile {
		t.Fatalf("expected resolveProductCLI to return ZQK_BIN %s, got %s", dummyFile, resolved)
	}
}

func TestCycle154_ResolverCacheAndFallbacks(t *testing.T) {
	if p := ResolvePath("", ""); p != "" {
		t.Fatalf("expected empty for empty ResolvePath, got %s", p)
	}
	if p := ResolvePath("", "some_rel_path"); p != "some_rel_path" {
		t.Fatalf("expected some_rel_path, got %s", p)
	}
	if p := ResolvePath("/root", "web:https://example.com/spec"); p != "https://example.com/spec" {
		t.Fatalf("unexpected web resolution: %s", p)
	}
	if p := ResolvePath("/root", "abs:/system/path"); p != "/system/path" {
		t.Fatalf("unexpected abs resolution: %s", p)
	}

	s1, err := ResolvePathStrict("", "")
	if err != nil || s1 != "" {
		t.Fatalf("expected empty, nil from ResolvePathStrict('', ''), got %q, %v", s1, err)
	}
	s2, err := ResolvePathStrict("", "rel_path")
	if err != nil || s2 != "rel_path" {
		t.Fatalf("expected rel_path, nil from ResolvePathStrict('', 'rel_path'), got %q, %v", s2, err)
	}
	sWeb, err := ResolvePathStrict("/root", "web:https://example.com/api")
	if err != nil || sWeb != "https://example.com/api" {
		t.Fatalf("unexpected web ResolvePathStrict: %s, %v", sWeb, err)
	}
	sAbs, err := ResolvePathStrict("/root", "abs:/var/log")
	if err != nil || sAbs != "/var/log" {
		t.Fatalf("unexpected abs ResolvePathStrict: %s, %v", sAbs, err)
	}

	tmpDir := t.TempDir()
	if IsPathCacheBuilt("") || IsPathCacheBuilt(tmpDir) {
		t.Fatal("expected IsPathCacheBuilt false for uninitialized root")
	}
	if !IsPathCacheStale("", nil) || !IsPathCacheStale(tmpDir, nil) {
		t.Fatal("expected IsPathCacheStale true for unbuilt cache")
	}

	BuildPathAliasCache("", nil)
	BuildPathAliasCache(tmpDir, map[string]string{"foo": "sub/foo"})
	if !IsPathCacheBuilt(tmpDir) {
		t.Fatal("expected IsPathCacheBuilt true after BuildPathAliasCache")
	}

	AddPathAlias("", "", "")
	AddPathAlias(tmpDir, "bar", "sub/bar")
	if GetPathAlias(tmpDir, "bar") != "sub/bar" {
		t.Fatalf("expected sub/bar, got %s", GetPathAlias(tmpDir, "bar"))
	}

	RemovePathAlias("", "")
	RemovePathAlias(tmpDir, "bar")
	if GetPathAlias(tmpDir, "bar") != "" {
		t.Fatal("expected empty alias after RemovePathAlias")
	}

	ReplacePathCache(tmpDir, map[string]string{"baz": "sub/baz"})
	if GetPathAlias(tmpDir, "baz") != "sub/baz" {
		t.Fatalf("expected sub/baz, got %s", GetPathAlias(tmpDir, "baz"))
	}

	if ResolvePathFromCacheOrConstant("", "baz", "fallback") != "" {
		t.Fatal("expected empty for empty projectRoot")
	}
	hit := ResolvePathFromCacheOrConstant(tmpDir, "baz", "fallback")
	if hit != filepath.Join(tmpDir, "sub/baz") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmpDir, "sub/baz"), hit)
	}
	miss := ResolvePathFromCacheOrConstant(tmpDir, "unknown", "fallback")
	if miss != filepath.Join(tmpDir, "fallback") {
		t.Fatalf("expected fallback %s, got %s", filepath.Join(tmpDir, "fallback"), miss)
	}
}

func TestCycle154_LayoutAndDiscover(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "a_file")
	if err := fileutil.WriteFile(invalidFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write invalidFile: %v", err)
	}

	layout := LayoutUnder(invalidFile).Dir("sub", 0755)
	if layout.Err() == nil {
		t.Fatal("expected layout error creating sub under regular file")
	}
	layout2 := layout.Dir("another", 0755)
	if layout2.Err() == nil {
		t.Fatal("expected error to persist after chained Dir call on errored layout")
	}

	if FirstExistingFromCwd("") != "" {
		t.Fatal("expected empty for empty rel in FirstExistingFromCwd")
	}
	if FirstExistingFromCwd("nonexistent_subfile_cycle154_random") != "" {
		t.Fatal("expected empty for non-existent rel in FirstExistingFromCwd")
	}

	if FirstExistingFromCwdAny(nil) != "" {
		t.Fatal("expected empty for nil slice in FirstExistingFromCwdAny")
	}
	if FirstExistingFromCwdAny([]string{"nonexistent1", "nonexistent2"}) != "" {
		t.Fatal("expected empty for missing candidates in FirstExistingFromCwdAny")
	}

	ResetCwdDiscovery()
}

func TestCycle154_WorktreeTrunkLocks(t *testing.T) {
	if !IsGitTrunkBranch("main") || !IsGitTrunkBranch("master") {
		t.Fatal("expected main and master to be trunk branches")
	}
	if IsGitTrunkBranch("feature/test") || IsGitTrunkBranch("") {
		t.Fatal("expected feature and empty branch not to be trunk branches")
	}

	if ShouldEvictLinkedTrunkLock("/wt", "feature", "/primary") {
		t.Fatal("expected false for non-trunk branch")
	}
	if ShouldEvictLinkedTrunkLock("", "main", "/primary") || ShouldEvictLinkedTrunkLock("/wt", "main", "") {
		t.Fatal("expected false for empty path in ShouldEvictLinkedTrunkLock")
	}

	tmpDir := t.TempDir()
	if ShouldEvictLinkedTrunkLock(tmpDir, "main", tmpDir) {
		t.Fatal("expected false when worktreePath == primaryRoot")
	}

	otherDir := t.TempDir()
	if !ShouldEvictLinkedTrunkLock(tmpDir, "main", otherDir) {
		t.Fatal("expected true when worktreePath != primaryRoot on main")
	}

	if canonPath("") != "" {
		t.Fatal("expected empty for empty path in canonPath")
	}
	if canon := canonPath(tmpDir); canon == "" {
		t.Fatal("expected non-empty canonPath for tmpDir")
	}
}

func TestCycle154_RuntimeLayoutAndProjectYAML(t *testing.T) {
	if casYAMLPath("", "hash") != "" || casYAMLPath("dir", "") != "" {
		t.Fatal("expected empty for empty arguments in casYAMLPath")
	}
	casPath := casYAMLPath("/cas", "abcdef")
	if casPath != "/cas/abcdef.yaml" {
		t.Fatalf("unexpected casYAMLPath: %s", casPath)
	}

	relYml := WorktreeLocalConfigRelYml()
	if !strings.HasSuffix(relYml, ".yml") {
		t.Fatalf("expected .yml suffix on WorktreeLocalConfigRelYml, got %s", relYml)
	}

	if !IsWorktreeRuntimePorcelain(WorktreeLocalConfigRel()) {
		t.Fatal("expected WorktreeLocalConfigRel to be porcelain")
	}
	if !IsWorktreeRuntimePorcelain(WorktreeLocalConfigRelYml()) {
		t.Fatal("expected WorktreeLocalConfigRelYml to be porcelain")
	}
	if !IsWorktreeRuntimePorcelain(ObjectDraftsRel()) {
		t.Fatal("expected ObjectDraftsRel to be porcelain")
	}
	if IsWorktreeRuntimePorcelain("cmd/zqk/main.go") {
		t.Fatal("expected cmd/zqk/main.go not to be porcelain")
	}

	tmpDir := t.TempDir()
	if FirstProjectYAMLConfig(tmpDir) != "" {
		t.Fatal("expected empty for dir without zqk.yaml")
	}
	cfgDir := filepath.Join(tmpDir, "config")
	if err := fileutil.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("mkdir cfgDir: %v", err)
	}
	zqkFile := filepath.Join(cfgDir, "zqk.yaml")
	if err := fileutil.WriteFile(zqkFile, []byte("zqk: config"), 0644); err != nil {
		t.Fatalf("write zqk.yaml: %v", err)
	}
	if hit := FirstProjectYAMLConfig(tmpDir); hit != zqkFile {
		t.Fatalf("expected %s, got %s", zqkFile, hit)
	}

	if _, err := ProjectPath(""); err != ErrPathEscapesProject {
		t.Fatalf("expected ErrPathEscapesProject for empty root, got %v", err)
	}
	if _, err := ProjectPath(tmpDir, "..", "..", "escaped"); err == nil {
		t.Fatal("expected error for escaping path in ProjectPath")
	}
	validProjPath, err := ProjectPath(tmpDir, "sub", "file.txt")
	if err != nil || validProjPath != filepath.Join(tmpDir, "sub", "file.txt") {
		t.Fatalf("ProjectPath failed: path=%s, err=%v", validProjPath, err)
	}

	SetNamespacePrefix("")
	origPrefix := NamespacePrefix
	SetNamespacePrefix("zen")
	if NamespacePrefix != "zen" || KernelNamespaceID != "zen:kernel" {
		t.Fatalf("unexpected namespace prefix update: %s, %s", NamespacePrefix, KernelNamespaceID)
	}
	SetNamespacePrefix(origPrefix)
}

func TestCycle154_DocEntryAndLookupDirs(t *testing.T) {
	tmpDir := t.TempDir()

	pEmpty, err := ResolveDocEntryPath(tmpDir, "")
	if err != nil || pEmpty != "" {
		t.Fatalf("expected empty, nil from ResolveDocEntryPath: %q, %v", pEmpty, err)
	}

	pWeb, err := ResolveDocEntryPath(tmpDir, "web:https://example.com/doc")
	if err != nil || pWeb != "https://example.com/doc" {
		t.Fatalf("unexpected web doc entry: %s, %v", pWeb, err)
	}

	pAbs, err := ResolveDocEntryPath(tmpDir, "abs:/system/doc.md")
	if err != nil || pAbs != "/system/doc.md" {
		t.Fatalf("unexpected abs doc entry: %s, %v", pAbs, err)
	}

	pBare, err := ResolveDocEntryPath(tmpDir, "docs/manual.md")
	if err != nil || pBare != filepath.Join(tmpDir, "docs", "manual.md") {
		t.Fatalf("unexpected bare doc entry: %s, %v", pBare, err)
	}

	dirsUnnamed := AgentWorktreeLookupDirs(tmpDir, "")
	if len(dirsUnnamed) == 0 || !strings.Contains(dirsUnnamed[0], "unnamed") {
		t.Fatalf("expected unnamed task lookup dir: %v", dirsUnnamed)
	}
}

func TestCycle154_WorktreeSeatedProjectRootResolution(t *testing.T) {
	seatedRoot := t.TempDir()
	wtContainer := t.TempDir()
	prevWtRoot := zqkenv.AgentWorktreeRoot().Get()
	zqkenv.AgentWorktreeRoot().Set(wtContainer)
	defer zqkenv.AgentWorktreeRoot().Set(prevWtRoot)

	wtPath := filepath.Join(wtContainer, "task-isolated")
	if err := BootstrapWorktreeConfig(wtPath, seatedRoot); err != nil {
		t.Fatalf("BootstrapWorktreeConfig: %v", err)
	}

	prevPR := zqkenv.ProjectRoot().Get()
	zqkenv.ProjectRoot().Set(wtPath)
	defer zqkenv.ProjectRoot().Set(prevPR)

	res := ResolveProjectRoot(wtPath)
	if res != seatedRoot {
		t.Fatalf("expected ResolveProjectRoot to return seatedRoot %s, got %s", seatedRoot, res)
	}
}
