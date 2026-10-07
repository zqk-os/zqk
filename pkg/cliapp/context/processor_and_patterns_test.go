package context

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestContextNodeAdapter_Methods(t *testing.T) {
	node := &ContextNode{
		Name:    "test-node",
		Context: &Context{Format: "json"},
	}
	adapter := NewContextNodeAdapter(node, 10, 2)
	if adapter.GetNode() != node {
		t.Errorf("expected node match")
	}
	if adapter.GetPrecedence() != 10 {
		t.Errorf("expected precedence 10, got %d", adapter.GetPrecedence())
	}
	adapter.SetPrecedence(20)
	if adapter.GetPrecedence() != 20 {
		t.Errorf("expected precedence 20, got %d", adapter.GetPrecedence())
	}
	if adapter.GetDepth() != 2 {
		t.Errorf("expected depth 2, got %d", adapter.GetDepth())
	}
	adapter.SetDepth(3)
	if adapter.GetDepth() != 3 {
		t.Errorf("expected depth 3, got %d", adapter.GetDepth())
	}

	errs := adapter.Validate()
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors, got %d", len(errs))
	}

	// Nil node validation
	nilAdapter := NewContextNodeAdapter(nil, 0, 0)
	if len(nilAdapter.Validate()) == 0 {
		t.Errorf("expected validation errors for nil node")
	}

	// Nil context in node validation
	nilCtxAdapter := NewContextNodeAdapter(&ContextNode{}, 0, 0)
	if len(nilCtxAdapter.Validate()) == 0 {
		t.Errorf("expected validation errors for nil context")
	}
}

func TestContextNodeAdapter_MergeAndChainable(t *testing.T) {
	node1 := &ContextNode{
		Name:    "n1",
		Context: &Context{Format: "table", Verbose: true},
	}
	node2 := &ContextNode{
		Name:    "n2",
		Context: &Context{Format: "json", Quiet: true},
	}
	a1 := NewContextNodeAdapter(node1, 1, 0)
	a2 := NewContextNodeAdapter(node2, 2, 0)

	merged := a1.Merge(a2)
	if merged == nil {
		t.Fatal("expected non-nil merged adapter")
	}

	// Merge with nil returns self
	if a1.Merge(nil) != a1 {
		t.Errorf("expected merge with nil to return receiver")
	}

	// ToChainableContextNode
	chainable := ToChainableContextNode(node1, 1)
	if chainable == nil {
		t.Errorf("expected non-nil chainable")
	}
	if ToChainableContextNode(nil, 1) != nil {
		t.Errorf("expected nil for nil node")
	}
}

func TestContextProcessor_ProcessSequential(t *testing.T) {
	cp := NewContextProcessor(ModeSequential)

	// Empty contexts
	_, err := cp.ProcessSequential(nil)
	if err == nil {
		t.Errorf("expected error for empty contexts")
	}

	ctx1 := &Context{Format: "table", Verbose: false}
	ctx2 := &Context{Format: "json", Verbose: true}

	res, err := cp.ProcessSequential([]*Context{ctx1, ctx2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestContextProcessor_HierarchicalAndHybrid(t *testing.T) {
	cp := NewContextProcessor(ModeHierarchical)

	// Nil root checks
	if _, err := cp.ProcessHierarchical(nil); err == nil {
		t.Errorf("expected error for nil root")
	}
	if _, err := cp.ProcessHybrid(nil); err == nil {
		t.Errorf("expected error for nil root")
	}
	if _, err := cp.ProcessNode(nil); err == nil {
		t.Errorf("expected error for nil node")
	}

	root := &ContextNode{
		Name:    "root",
		Context: &Context{Format: "yaml"},
		Children: []*ContextNode{
			{
				Name:    "child",
				Context: &Context{Verbose: true},
			},
		},
	}

	resH, err := cp.ProcessHierarchical(root)
	if err != nil {
		t.Fatalf("ProcessHierarchical error: %v", err)
	}
	if resH == nil {
		t.Fatal("expected non-nil result from hierarchical")
	}

	resHy, err := cp.ProcessHybrid(root)
	if err != nil {
		t.Fatalf("ProcessHybrid error: %v", err)
	}
	if resHy == nil {
		t.Fatal("expected non-nil result from hybrid")
	}

	resNode, err := cp.ProcessNode(root)
	if err != nil {
		t.Fatalf("ProcessNode error: %v", err)
	}
	if resNode == nil {
		t.Fatal("expected non-nil result from ProcessNode")
	}
}

func TestPatterns_BuilderAndOverrides(t *testing.T) {
	pattern := NewContextBuilderPattern()
	pattern.WithSystemDefaults(map[string]any{"format": "table"})
	pattern.WithUserConfig(map[string]any{"verbose": true})
	pattern.WithOverride("quiet", false)
	pattern.WithOverrides(map[string]any{"profile": "human"})

	ctx := pattern.MustBuild()
	if ctx == nil {
		t.Fatal("MustBuild returned nil")
	}
	if ctx.Format != "table" {
		t.Errorf("expected table, got %s", ctx.Format)
	}

	// BuildContextWithOverrides
	derived, err := BuildContextWithOverrides(ctx, map[string]any{"format": "json"})
	if err != nil || derived == nil {
		t.Fatalf("BuildContextWithOverrides failed: %v", err)
	}
	if derived.Format != "json" {
		t.Errorf("expected json, got %s", derived.Format)
	}

	// Error on nil base
	if _, err := BuildContextWithOverrides(nil, nil); err == nil {
		t.Errorf("expected error on nil base context")
	}

	// BuildContextForCommand
	cmdCtx, err := BuildContextForCommand(
		map[string]any{"format": "table"},
		map[string]any{"verbose": true},
		map[string]any{},
		map[string]any{"quiet": true},
		"",
	)
	if err != nil || cmdCtx == nil {
		t.Fatalf("BuildContextForCommand failed: %v", err)
	}
}

func TestWorkspaceRoot_PersistedAndNotify(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	projDir := filepath.Join(wsDir, "my-proj")

	if err := fileutil.MkdirAll(filepath.Join(projDir, ".zqk"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(wsDir, ".zqk"), 0755); err != nil {
		t.Fatal(err)
	}

	if !IsValidProjectRoot(projDir) {
		t.Errorf("expected IsValidProjectRoot=true for projDir")
	}

	if err := WritePersistedCurrentRoot(wsDir, projDir); err != nil {
		t.Fatalf("WritePersistedCurrentRoot failed: %v", err)
	}

	read := ReadPersistedCurrentRoot(wsDir)
	if read != projDir {
		t.Errorf("expected %s, got %s", projDir, read)
	}

	notified := false
	OnCurrentRootSet = func(ws, newP, prevP string) {
		notified = true
	}
	defer func() { OnCurrentRootSet = nil }()

	NotifyCurrentRootSet(wsDir, projDir, "")
	if !notified {
		t.Errorf("expected OnCurrentRootSet to be invoked")
	}

	// Test cloneYAMLSlice via createContextFromMap with nested structures
	cm := NewContextManager()
	m := map[string]any{
		"items": []any{
			"a",
			map[string]any{"nestedKey": "nestedVal"},
			[]any{1, 2},
		},
	}
	c, err := cm.createContextFromMap(m, LayerProject)
	if err != nil || c == nil {
		t.Errorf("expected non-nil context from map with slice")
	}

	// Test cloneYAMLSlice directly
	if cloneYAMLSlice(nil) != nil {
		t.Errorf("expected nil for nil slice")
	}

	// Test mergeProfileIntoTarget with unknown profile fallback to copyProfileFieldsFromContext
	targetCtx := &Context{}
	sourceCtx := &Context{
		Profile:                "unknown-profile-fallback",
		Format:                 "yaml",
		Verbose:                true,
		Quiet:                  false,
		StorageMaxPageSize:     50,
		StorageDefaultPageSize: 10,
		StorageEnableGrouping:  true,
		StorageMaxGroupSize:    20,
	}
	mergeProfileIntoTarget(targetCtx, sourceCtx)
	if targetCtx.Format != "yaml" || !targetCtx.Verbose || targetCtx.StorageMaxPageSize != 50 {
		t.Errorf("unexpected target after mergeProfile: %+v", targetCtx)
	}
	copyProfileFieldsFromContext(nil, nil) // guard check

	wsFound := FindWorkspaceRoot(projDir)
	if wsFound != wsDir {
		t.Logf("FindWorkspaceRoot: %s", wsFound)
	}
}

func TestBrandSettings_HelpersAndWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	path := BrandSettingsPath(tmpDir)
	if path == "" {
		t.Errorf("expected non-empty BrandSettingsPath")
	}

	// s.ProjectRoot variations
	s := &BrandSettings{}
	if root := s.ProjectRoot("/test/my-proj/config"); root != "/test/my-proj" {
		t.Errorf("expected /test/my-proj, got %s", root)
	}
	if root := s.ProjectRoot("/test/my-proj/.zqk/config"); root != "/test/my-proj" {
		t.Errorf("expected /test/my-proj, got %s", root)
	}
	if root := s.ProjectRoot("/test/my-proj"); root != "/test/my-proj" {
		t.Errorf("expected /test/my-proj, got %s", root)
	}

	// EnsureTestRootBrandSettingsFiles when testRoot does not match
	if err := EnsureTestRootBrandSettingsFiles(tmpDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// getDefaultSystemDefaults
	defs := getDefaultSystemDefaults()
	if defs == nil || defs["format"] != "table" {
		t.Errorf("unexpected default system defaults: %v", defs)
	}

	// readPersistedCurrentRoot helper
	_ = readPersistedCurrentRoot(tmpDir)

	// ResolveProjectRootFromSettings error paths
	_, _, err := ResolveProjectRootFromSettings(filepath.Join(tmpDir, "does-not-exist"))
	if err == nil {
		t.Errorf("expected error for non-existent path")
	}
	_, _, err = ResolveProjectRootFromSettings("/worktree/agent-seat-1")
	if err == nil {
		t.Errorf("expected error for agent worktree path")
	}

	// ContextChainAdapter validation
	nilCtxAdapter := &ContextChainAdapter{}
	if errs := nilCtxAdapter.Validate(); len(errs) == 0 {
		t.Errorf("expected error for nil context in chain adapter")
	}
	invalidFmtAdapter := &ContextChainAdapter{
		ctx: &Context{Format: "bogus-fmt"},
	}
	if errs := invalidFmtAdapter.Validate(); len(errs) == 0 {
		t.Errorf("expected error for invalid format in chain adapter")
	}
}

func TestContextNode_ChildrenAndProfileLoader(t *testing.T) {
	root := &ContextNode{Name: "root"}
	c1 := &ContextNode{Name: "c1", Priority: 2}
	c2 := &ContextNode{Name: "c2", Priority: 1}

	root.AddChildren(c1, c2)
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}

	root.SortChildren()
	if root.Children[0].Name != "c2" {
		t.Errorf("expected c2 first after SortChildren, got %s", root.Children[0].Name)
	}

	// ProfileLoader ClearCache and findProfilesDir
	pl := NewProfileLoader(t.TempDir())
	pl.ClearCache()
	if findProfilesDir() != "" {
		t.Errorf("expected empty string from findProfilesDir")
	}

	// Test getPositiveInt
	intMap := map[string]any{
		"int_val":   42,
		"float_val": 12.34,
		"str_val":   "not-an-int",
	}
	if getPositiveInt(intMap, "missing") != 0 {
		t.Errorf("expected 0 for missing key")
	}
	if getPositiveInt(intMap, "int_val") != 42 {
		t.Errorf("expected 42, got %d", getPositiveInt(intMap, "int_val"))
	}
	if getPositiveInt(intMap, "float_val") != 12 {
		t.Errorf("expected 12, got %d", getPositiveInt(intMap, "float_val"))
	}
	if getPositiveInt(intMap, "str_val") != 0 {
		t.Errorf("expected 0 for string value")
	}
}

func TestContextBuilder_HierarchicalAndModes(t *testing.T) {
	// ModeHierarchical with AddChild and AddNestedChild
	b := NewContextBuilder(ModeHierarchical)
	b.AddChild(&Context{Format: "table"}, "parent")

	// Missing parent
	if _, err := b.AddNestedChild("nonexistent", &Context{Verbose: true}, "child"); err == nil {
		t.Errorf("expected error adding to nonexistent parent")
	}

	// Valid nested child
	if _, err := b.AddNestedChild("parent", &Context{Verbose: true}, "child"); err != nil {
		t.Fatalf("AddNestedChild failed: %v", err)
	}

	res, err := b.Build()
	if err != nil || res == nil {
		t.Fatalf("Build failed: %v", err)
	}

	// ModeHierarchical with nil root error
	bEmpty := NewContextBuilder(ModeHierarchical)
	if _, err := bEmpty.Build(); err == nil {
		t.Errorf("expected error for empty hierarchical build")
	}

	// ModeHybrid with nil root error
	bHybridEmpty := NewContextBuilder(ModeHybrid)
	if _, err := bHybridEmpty.Build(); err == nil {
		t.Errorf("expected error for empty hybrid build")
	}

	// ModeHybrid with root
	bHybrid := NewContextBuilder(ModeHybrid)
	bHybrid.AddChild(&Context{Quiet: true}, "child1")
	if _, err := bHybrid.Build(); err != nil {
		t.Errorf("hybrid build failed: %v", err)
	}

	// Unknown mode
	bUnknown := NewContextBuilder(ProcessingMode(999))
	if _, err := bUnknown.Build(); err == nil {
		t.Errorf("expected error for unknown mode")
	}
}
