package context

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestContext_CloneYAMLSliceAndMap(t *testing.T) {
	// 1. cloneYAMLSlice proofs
	if cloneYAMLSlice(nil) != nil {
		t.Fatal("expected nil output for nil cloneYAMLSlice")
	}

	nested := []any{
		"simple_string",
		42,
		map[string]any{
			"inner_key": "inner_val",
			"inner_slice": []any{"a", "b"},
		},
		[]any{1, 2, 3},
	}

	clonedSlice := cloneYAMLSlice(nested)
	if len(clonedSlice) != len(nested) {
		t.Fatalf("cloned slice length mismatch: %d vs %d", len(clonedSlice), len(nested))
	}

	// 2. cloneYAMLMap proofs
	emptyMap := cloneYAMLMap(nil)
	if emptyMap == nil || len(emptyMap) != 0 {
		t.Fatal("expected empty initialized map for nil cloneYAMLMap")
	}

	mapInput := map[string]any{
		"str": "val",
		"submap": map[string]any{
			"sub": 123,
		},
		"slice": []any{"x", "y"},
	}
	clonedMap := cloneYAMLMap(mapInput)
	if len(clonedMap) != 3 {
		t.Fatalf("cloned map unexpected length: %d", len(clonedMap))
	}
}

func TestContext_PathResolverAndDerived(t *testing.T) {
	// Nil context safety
	var nilCtx *Context
	if resolver := nilCtx.PathResolver(); resolver == nil {
		t.Fatal("expected non-nil default PathResolver for nil Context")
	}
	if derivedNil := nilCtx.WithPathResolver(nil); derivedNil != nil {
		t.Fatal("expected nil derived Context for nil receiver")
	}

	baseCtx := &Context{
		ProjectRoot: "/tmp/project",
	}
	res1 := baseCtx.PathResolver()
	if res1 == nil {
		t.Fatal("expected non-nil PathResolver")
	}

	customResolver := paths.NewPathResolver("/custom/root")
	derivedCtx := baseCtx.WithPathResolver(customResolver)
	if derivedCtx.PathResolver() != customResolver {
		t.Fatal("expected custom PathResolver to be preserved")
	}

	clearedCtx := derivedCtx.WithPathResolver(nil)
	if clearedCtx.pathResolver != nil {
		t.Fatal("expected cleared pathResolver override")
	}
}

func TestBrandSettings_ProjectRootVariations(t *testing.T) {
	s := &BrandSettings{}

	// Dir ending in config under .zqk
	mockDir := "/workspace/myproject/.zqk/config"
	if root := s.ProjectRoot(mockDir); root != "/workspace/myproject" {
		t.Fatalf("expected /workspace/myproject, got %q", root)
	}

	// Dir ending in config directly
	mockDir2 := "/workspace/myproject/config"
	if root := s.ProjectRoot(mockDir2); root != "/workspace/myproject" {
		t.Fatalf("expected /workspace/myproject, got %q", root)
	}

	// Legacy root fallback
	legacyDir := "/workspace/myproject"
	if root := s.ProjectRoot(legacyDir); root != legacyDir {
		t.Fatalf("expected %q, got %q", legacyDir, root)
	}

	// Absolute path in settings
	s.Paths.ProjectRoot = "/absolute/custom/path"
	if root := s.ProjectRoot(legacyDir); root != "/absolute/custom/path" {
		t.Fatalf("expected /absolute/custom/path, got %q", root)
	}
}

func TestEnsureTestRootBrandSettingsFiles(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Mismatched test root -> no-op returns nil
	zqkenv.TestRoot().Set("/some/other/path")
	if err := EnsureTestRootBrandSettingsFiles(tempDir); err != nil {
		t.Fatalf("unexpected error on mismatched test root: %v", err)
	}

	// 2. Matched test root -> creates config/zqk-test.yaml
	zqkenv.TestRoot().Set(tempDir)
	defer zqkenv.TestRoot().Set("")

	if err := EnsureTestRootBrandSettingsFiles(tempDir); err != nil {
		t.Fatalf("EnsureTestRootBrandSettingsFiles failed: %v", err)
	}

	testConfig := filepath.Join(tempDir, "config", "zqk-test.yaml")
	if _, err := fileutil.Stat(testConfig); err != nil {
		t.Fatalf("expected generated test config at %s: %v", testConfig, err)
	}
}

func TestWritePersistedCurrentRoot_Validation(t *testing.T) {
	tempDir := t.TempDir()

	// Invalid: path escaping workspace
	if err := WritePersistedCurrentRoot(tempDir, "/outside/dir"); err == nil {
		t.Fatal("expected error for project root outside workspace")
	}

	// Invalid: not a project root (missing .zqk)
	subDir := filepath.Join(tempDir, "subproject")
	if err := fileutil.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := WritePersistedCurrentRoot(tempDir, subDir); err == nil {
		t.Fatal("expected error for directory lacking .zqk marker")
	}

	// Valid: contains .zqk
	zqkDir := filepath.Join(subDir, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(zqkDir, 0o755); err != nil {
		t.Fatalf("mkdir .zqk failed: %v", err)
	}
	if err := WritePersistedCurrentRoot(tempDir, subDir); err != nil {
		t.Fatalf("expected successful write, got %v", err)
	}
}

func TestBrandSettings_ResolveSnapshotBackupDirExtended(t *testing.T) {
	var nilSettings *BrandSettings
	if got := nilSettings.ResolveSnapshotBackupDir("/root"); got != "" {
		t.Fatalf("expected empty string for nil settings, got %q", got)
	}

	s := &BrandSettings{}
	if got := s.ResolveSnapshotBackupDir("/root"); got != "" {
		t.Fatalf("expected empty string for empty SnapshotBackupDir, got %q", got)
	}

	s.KernelState.SnapshotBackupDir = "~/custom_backups"
	home, err := fileutil.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir failed: %v", err)
	}
	if got := s.ResolveSnapshotBackupDir("/root"); got != filepath.Join(home, "custom_backups") {
		t.Fatalf("expected home-expanded dir, got %q", got)
	}

	s.KernelState.SnapshotBackupDir = "/var/backups"
	if got := s.ResolveSnapshotBackupDir("/root"); got != "/var/backups" {
		t.Fatalf("expected absolute path preserved, got %q", got)
	}

	s.KernelState.SnapshotBackupDir = "backups"
	if got := s.ResolveSnapshotBackupDir("/root"); got != "/root/backups" {
		t.Fatalf("expected joined relative path, got %q", got)
	}
}

func TestContext_GetLayersAndDerive(t *testing.T) {
	c := &Context{}
	layers := c.GetLayers()
	if layers == nil || len(layers) != 0 {
		t.Fatalf("expected empty map from uninitialized Context, got %v", layers)
	}

	derived := c.Derive(map[string]any{"format": "json"})
	if derived.Format != "json" {
		t.Fatalf("expected derived context to have format=json, got %q", derived.Format)
	}
}

func TestContextBuilder_FindNodeByName(t *testing.T) {
	cb := NewContextBuilder(ModeSequential)
	if got := cb.findNodeByName("missing"); got != nil {
		t.Fatalf("expected nil for missing node, got %v", got)
	}

	n1 := &ContextNode{Name: "first"}
	n2 := &ContextNode{Name: "second", Children: []*ContextNode{{Name: "child"}}}
	cb.nodes = []*ContextNode{n1, n2}

	if got := cb.findNodeByName("first"); got != n1 {
		t.Fatalf("expected to find node 'first'")
	}
	if got := cb.findNodeByName("child"); got == nil || got.Name != "child" {
		t.Fatalf("expected to find node 'child'")
	}
	if got := cb.findNodeByName("nonexistent"); got != nil {
		t.Fatalf("expected nil for 'nonexistent'")
	}
}

func TestBrandSettings_ErrorBranches(t *testing.T) {
	if _, err := LoadBrandSettings(""); err == nil {
		t.Fatal("expected error for empty projectRoot in LoadBrandSettings")
	}
	if _, _, err := ResolveProjectRootFromSettings("/nonexistent/start/path"); err == nil {
		t.Fatal("expected error for nonexistent path in ResolveProjectRootFromSettings")
	}
}


