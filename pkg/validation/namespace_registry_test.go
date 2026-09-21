package validation

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNamespaceRegistry_GetNamespaceForKind(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf(ConstMagicba706c3d, err)
	}

	tests := []struct {
		kind        string
		want        string
		description string
	}{
		// ZQK kernel objects
		{"goal", DefaultNamespaceKernel, "kernel goal"},
		{"backlog_item", DefaultNamespaceKernel, "kernel backlog item"},
		{"milestone", DefaultNamespaceKernel, "kernel milestone"},
		{"workstream", DefaultNamespaceKernel, "kernel workstream"},
		{"account", DefaultNamespaceKernel, "kernel account"},
		{"role", DefaultNamespaceKernel, "kernel role"},

		// Domain:organizational objects
		{"organization", "domain:organizational", "organizational organization"},
		{"division", "domain:organizational", "organizational division"},
		{"department", "domain:organizational", "organizational department"},
		{"team", "domain:organizational", "organizational team"},
		{"partnership", "domain:organizational", "organizational partnership"},

		// Unknown kind - should default to DefaultNamespaceKernel
		{"unknown_kind", DefaultNamespaceKernel, "unknown kind defaults to kernel"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := registry.GetNamespaceForKind(tt.kind)
			if got != tt.want {
				t.Errorf(ConstMagic6d2bcc56, tt.kind, got, tt.want, tt.description)
			}
		})
	}
}

func TestNamespaceRegistry_RegisterNamespace(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	// Register a custom namespace
	registry.RegisterNamespace("custom_kind", "domain:custom")

	got := registry.GetNamespaceForKind("custom_kind")
	if got != "domain:custom" {
		t.Errorf(ConstMagic52fa586b, got, "domain:custom")
	}
}

func TestNamespaceRegistry_GetAllNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf(ConstMagicba706c3d, err)
	}

	namespaces := registry.GetAllNamespaces()

	// Should have at least kernel and organizational namespaces
	if len(namespaces) == 0 {
		t.Error(ConstMagica7f9ff4d)
	}

	// Check that we have some expected mappings
	if namespace, ok := namespaces["goal"]; !ok || namespace != DefaultNamespaceKernel {
		t.Errorf(ConstMagic8a3e3dd1, namespace)
	}

	if namespace, ok := namespaces["organization"]; !ok || namespace != ConstMagic09ae1f1a {
		t.Errorf(ConstMagic8dbd9fe4, namespace)
	}
}

func TestNamespaceRegistry_InferNamespaceFromKind(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	tests := []struct {
		kind        string
		want        string
		description string
	}{
		// Organizational domain objects
		{"organization", "domain:organizational", "Organization kind"},
		{"division", "domain:organizational", "Division kind"},
		{"department", "domain:organizational", "Department kind"},
		{"team", "domain:organizational", "Team kind"},
		{"partnership", "domain:organizational", "Partnership kind"},

		// Integration objects (pattern matching)
		{"integration_jira", "integration:jira", "Integration prefix pattern"},
		{"integration_github", "integration:github", "Integration prefix pattern"},
		{"jira_integration", "integration:jira", "Integration suffix pattern"},
		{"github_integration", "integration:github", "Integration suffix pattern"},
		{"integration_test_integration", "integration:test", "Integration both prefix and suffix"},

		// Unknown kinds default to kernel
		{"unknown_kind", DefaultNamespaceKernel, "Unknown kind defaults to kernel"},
		{"random_object", DefaultNamespaceKernel, "Random object defaults to kernel"},
		{"", DefaultNamespaceKernel, "Empty kind defaults to kernel"},

		// Kernel objects
		{"goal", DefaultNamespaceKernel, "Kernel goal object"},
		{"milestone", DefaultNamespaceKernel, "Kernel milestone object"},
		{"backlog_item", DefaultNamespaceKernel, "Kernel backlog item"},
		{"account", DefaultNamespaceKernel, "Kernel account object"},

		// Edge cases
		{"integration_", "integration:", "Integration prefix with empty domain"},
		{"_integration", "integration:", "Integration suffix only - strings.HasSuffix matches, returns integration:"},
		{"integration", DefaultNamespaceKernel, "Just 'integration' (not a pattern) defaults to kernel"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := registry.inferNamespaceFromKind(tt.kind)
			if got != tt.want {
				t.Errorf(ConstMagic9eac380c, tt.kind, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_InferNamespaceFromKind_WithRegistered tests inference with registered namespaces
func TestNamespaceRegistry_InferNamespaceFromKind_WithRegistered(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	// Register a custom namespace
	registry.RegisterNamespace("custom_kind", "domain:custom")

	// Registered namespace should take precedence over inference
	got := registry.inferNamespaceFromKind("custom_kind")
	if got != "domain:custom" {
		t.Errorf(ConstMagic5fca5cf2, got)
	}

	// Unregistered kind should still use inference
	got = registry.inferNamespaceFromKind("organization")
	if got != ConstMagic09ae1f1a {
		t.Errorf(ConstMagic9640f267, got)
	}
}

// TestGetNamespaceRegistry tests the singleton pattern for GetNamespaceRegistry
func TestGetNamespaceRegistry(t *testing.T) {
	t.Parallel()
	// First call should initialize the singleton
	registry1 := GetNamespaceRegistry()
	if registry1 == nil {
		t.Fatal(ConstMagicff6a164f)
	}

	// Second call should return the same instance (singleton)
	registry2 := GetNamespaceRegistry()
	if registry1 != registry2 {
		t.Error(ConstMagic9a92f7f0)
	}

	// Verify it's a valid registry by checking it can load namespaces
	if err := registry1.LoadNamespaces(); err != nil {
		t.Errorf(ConstMagic5e0026fd, err)
	}

	// Verify it has default namespaces loaded
	namespaces := registry1.GetAllNamespaces()
	if len(namespaces) == 0 {
		t.Error(ConstMagicef3a7cc7)
	}

	// Verify it can resolve a known kind
	namespace := registry1.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf(ConstMagicf6310275, namespace)
	}
}

// TestGetNamespaceRegistry_ConcurrentAccess tests concurrent access to the singleton
func TestGetNamespaceRegistry_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	// Test concurrent access to ensure thread safety
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.StartTestGoroutine(fmt.Sprintf(ConstMagic24f3efc0, i), fmt.Sprintf(ConstMagic6f51c6d1, i), func() {
			registry := GetNamespaceRegistry()
			if registry == nil {
				t.Error(ConstMagic7527c988)
			}
			done <- true
		})
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all calls returned the same instance
	registry1 := GetNamespaceRegistry()
	registry2 := GetNamespaceRegistry()
	if registry1 != registry2 {
		t.Error(ConstMagicc789f10f)
	}
}

// TestNamespaceRegistry_LoadDefaultNamespaces tests that default namespaces are loaded
// This tests loadDefaultNamespaces() indirectly through LoadNamespaces()
func TestNamespaceRegistry_LoadDefaultNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	// LoadNamespaces should call loadDefaultNamespaces internally
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf(ConstMagicc7c50a94, err)
	}

	// Verify default kernel namespaces are loaded
	kernelKinds := []string{"goal", "milestone", "workstream", "backlog_item", "account", "role"}
	for _, kind := range kernelKinds {
		namespace := registry.GetNamespaceForKind(kind)
		if namespace != DefaultNamespaceKernel {
			t.Errorf(ConstMagic370f61d1, kind, namespace)
		}
	}

	// Verify default organizational namespaces are loaded
	orgKinds := []string{"organization", "division", "department", "team", "partnership"}
	for _, kind := range orgKinds {
		namespace := registry.GetNamespaceForKind(kind)
		if namespace != ConstMagic09ae1f1a {
			t.Errorf(ConstMagic69498b33, kind, namespace)
		}
	}

	// Verify all default namespaces are present
	allNamespaces := registry.GetAllNamespaces()
	expectedCount := len(kernelKinds) + len(orgKinds)
	if len(allNamespaces) < expectedCount {
		t.Errorf(ConstMagic71a89150, len(allNamespaces), expectedCount)
	}
}

// TestDiscoverSpecsDirForRegistry tests the directory discovery logic
// This tests discoverSpecsDirForRegistry() indirectly through NewNamespaceRegistry("")
func TestDiscoverSpecsDirForRegistry(t *testing.T) {
	t.Parallel()
	// Test that NewNamespaceRegistry("") successfully discovers the specs directory
	// This exercises discoverSpecsDirForRegistry() which tries multiple paths
	registry := NewNamespaceRegistry("")
	if registry == nil {
		t.Fatal(ConstMagic62c3b1b9)
	}

	// The registry should be able to load namespaces if the directory is found
	// If the directory is not found, it should still work with defaults
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf(ConstMagic5262aa8b, err)
	}

	// Even if specs dir is not found, default namespaces should still work
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf(ConstMagic34b93550, namespace)
	}
}

// TestNewNamespaceRegistry_WithSpecsDir tests explicit specs directory
func TestNewNamespaceRegistry_WithSpecsDir(t *testing.T) {
	t.Parallel()
	// Test with explicit specs directory (if it exists)
	specsDir := paths.ProcessInternalObjectSpecsDir

	registry := NewNamespaceRegistry(specsDir)
	if registry == nil {
		t.Fatal(ConstMagic045a4789)
	}

	// Should be able to load namespaces
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf(ConstMagic454eabeb, err)
	}

	// Should still have default namespaces
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf(ConstMagic49b63b5f, namespace)
	}
}

// TestNewNamespaceRegistry_WithInvalidSpecsDir tests behavior with invalid directory
func TestNewNamespaceRegistry_WithInvalidSpecsDir(t *testing.T) {
	t.Parallel()
	// Test with a non-existent directory
	invalidDir := "/nonexistent/path/to/specs"

	registry := NewNamespaceRegistry(invalidDir)
	if registry == nil {
		t.Fatal(ConstMagic553fdfca)
	}

	// Should still be able to load namespaces (using defaults)
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf(ConstMagica3c11c59, err)
	}

	// Default namespaces should still work
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf(ConstMagicb511e1d8, namespace)
	}
}

// TestNamespaceRegistry_LoadNamespaceFromSpec tests loading namespace from spec files
// This tests loadNamespaceFromSpec() with various spec structures
func TestNamespaceRegistry_LoadNamespaceFromSpec(t *testing.T) {
	t.Parallel()
	// Create a temporary directory for test specs
	tmpDir := t.TempDir()
	registry := NewNamespaceRegistry(tmpDir)

	tests := []struct {
		name        string
		specContent string
		kind        string
		want        string
		description string
	}{
		{
			name: ConstMagice1b9b182,
			specContent: `kind: test_object
namespace_id: domain:test`,
			kind:        "test_object",
			want:        "domain:test",
			description: ConstMagic2b1fa03d,
		},
		{
			name: "namespace field",
			specContent: `kind: test_object
namespace: domain:test_namespace`,
			kind:        "test_object",
			want:        ConstMagic7651f84a,
			description: ConstMagicdf4ccdcd,
		},
		{
			name: ConstMagicb26a4ec2,
			specContent: `kind: test_object
fields:
  namespace_id:
    validation:
      default: domain:test_fields`,
			kind:        "test_object",
			want:        ConstMagic49dd5602,
			description: ConstMagic2d07774f,
		},
		{
			name: ConstMagicb184a1a6,
			specContent: `ontology: test_ontology_only
# No kind field`,
			kind:        "test_object",
			want:        DefaultNamespaceKernel, // Will infer from ontology, then from kind parameter
			description: ConstMagic294c6380,
		},
		{
			name: ConstMagic541e1108,
			specContent: `kind: test_object
ontology: organization`,
			kind:        "test_object",
			want:        ConstMagic09ae1f1a, // Inferred from ontology
			description: ConstMagic7dc70cf6,
		},
		{
			name:        ConstMagic0b738db0,
			specContent: `kind: test_object`,
			kind:        "organization",
			// When spec has kind="test_object" but parameter is "organization",
			// the code uses spec.Kind ("test_object") first, then falls back to parameter
			// Since "test_object" is not in the spec, it uses the parameter "organization"
			// But wait - the spec has kind: test_object, so objectKind = "test_object"
			// Then it infers from "test_object" which defaults to DefaultNamespaceKernel
			// Actually, let me check the logic: if spec.Kind is "test_object", objectKind = "test_object"
			// Then it infers from "test_object" which is unknown, so returns DefaultNamespaceKernel
			want:        DefaultNamespaceKernel, // Spec has kind="test_object", not "organization"
			description: ConstMagica72603c2,
		},
		{
			name: "invalid YAML",
			specContent: `kind: test_object
invalid: yaml: [unclosed bracket`,
			kind:        "test_object",
			want:        "", // Invalid YAML should return empty string
			description: ConstMagic5943e1e4,
		},
		{
			name:        "missing file",
			specContent: "",
			kind:        "test_object",
			want:        "", // Missing file should return empty string
			description: ConstMagicc0184485,
		},
		{
			name:        "empty spec",
			specContent: `# Empty spec`,
			kind:        "test_object",
			want:        DefaultNamespaceKernel, // Empty spec should infer from kind parameter
			description: ConstMagicfc0ae52b,
		},
		{
			name: ConstMagice13212b7,
			specContent: `kind: test_object
namespace_id: domain:test_id
namespace: domain:test_namespace`,
			kind:        "test_object",
			want:        "domain:test_id", // namespace_id takes precedence
			description: ConstMagic80021653,
		},
		{
			name: ConstMagicc37550c9,
			specContent: `kind: test_object
namespace: domain:test_namespace
fields:
  namespace_id:
    validation:
      default: domain:test_fields`,
			kind:        "test_object",
			want:        ConstMagic7651f84a, // namespace takes precedence over fields default
			description: ConstMagica3b2fbb0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create spec file
			specPath := filepath.Join(tmpDir, tt.kind+".yaml")
			if tt.specContent != emptyValue {
				if err := fileutil.WriteFile(specPath, []byte(tt.specContent), paths.FilePerm644); err != nil {
					t.Fatalf(ConstMagic186b95e3, err)
				}
			} else {
				// For missing file test, use a non-existent path
				specPath = filepath.Join(tmpDir, ConstMagic8293f515)
			}

			got := registry.loadNamespaceFromSpec(specPath, tt.kind)
			if got != tt.want {
				t.Errorf(ConstMagicda037864, specPath, tt.kind, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_GetSubordinateNamespaces tests subordinate namespace retrieval
func TestNamespaceRegistry_GetSubordinateNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf(ConstMagicba706c3d, err)
	}

	tests := []struct {
		parentNamespaceID string
		want              []string
		description       string
	}{
		{
			parentNamespaceID: DefaultNamespaceKernel,
			want:              []string{DefaultNamespaceKernelCLI, DefaultNamespaceKernelMetrics, DefaultNamespaceKernel + ":storage", DefaultNamespaceKernel + ":scheduler"},
			description:       ConstMagic09f8262c,
		},
		{
			parentNamespaceID: ConstMagic09ae1f1a,
			want:              []string{},
			description:       ConstMagice4cd4a74,
		},
		{
			parentNamespaceID: ConstMagic635d07d5,
			want:              []string{},
			description:       ConstMagic35d83ba2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.parentNamespaceID, func(t *testing.T) {
			got := registry.GetSubordinateNamespaces(tt.parentNamespaceID)
			if len(got) != len(tt.want) {
				t.Errorf(ConstMagic82bad832, tt.parentNamespaceID, len(got), len(tt.want), tt.description)
				return
			}

			// Check that all expected subordinates are present
			gotMap := make(map[string]bool)
			for _, ns := range got {
				gotMap[ns] = true
			}

			for _, expected := range tt.want {
				if !gotMap[expected] {
					t.Errorf(ConstMagic3c937e62, tt.parentNamespaceID, expected, tt.description)
				}
			}
		})
	}
}

// TestNamespaceRegistry_IsSubordinateOf tests hierarchy checks
func TestNamespaceRegistry_IsSubordinateOf(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	tests := []struct {
		childNamespaceID  string
		parentNamespaceID string
		want              bool
		description       string
	}{
		{
			childNamespaceID:  DefaultNamespaceKernelCLI,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              true,
			description:       ConstMagicd46bdde5,
		},
		{
			childNamespaceID:  DefaultNamespaceKernelMetrics,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              true,
			description:       ConstMagic18ea8d3f,
		},
		{
			childNamespaceID:  DefaultNamespaceKernel + ":storage",
			parentNamespaceID: DefaultNamespaceKernel,
			want:              true,
			description:       ConstMagic88afd274,
		},
		{
			childNamespaceID:  DefaultNamespaceKernel,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              false,
			description:       ConstMagicd51a63ad,
		},
		{
			childNamespaceID:  DefaultNamespaceKernelCLI,
			parentNamespaceID: DefaultNamespaceKernelMetrics,
			want:              false,
			description:       ConstMagic9b179651,
		},
		{
			childNamespaceID:  ConstMagic09ae1f1a,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              false,
			description:       ConstMagice6ac7562,
		},
		{
			childNamespaceID:  DefaultNamespaceKernelCLI + ":sub",
			parentNamespaceID: DefaultNamespaceKernelCLI,
			want:              true,
			description:       ConstMagic4bc8ebb1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := registry.IsSubordinateOf(tt.childNamespaceID, tt.parentNamespaceID)
			if got != tt.want {
				t.Errorf(ConstMagiccc51d34f, tt.childNamespaceID, tt.parentNamespaceID, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_GetParentNamespace tests parent namespace extraction
func TestNamespaceRegistry_GetParentNamespace(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	tests := []struct {
		subordinateNamespaceID string
		want                   string
		description            string
	}{
		{
			subordinateNamespaceID: DefaultNamespaceKernelCLI,
			want:                   DefaultNamespaceKernel,
			description:            ConstMagic2e6449e2,
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernelMetrics,
			want:                   DefaultNamespaceKernel,
			description:            ConstMagic3786bb1c,
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernel + ":storage",
			want:                   DefaultNamespaceKernel,
			description:            ConstMagic9e3812e9,
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernelCLI + ":sub",
			want:                   DefaultNamespaceKernelCLI,
			description:            ConstMagic2bf523f1,
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernel,
			want:                   "zqk", // Root namespace (first layer)
			description:            ConstMagic316c736f,
		},
		{
			subordinateNamespaceID: ConstMagic09ae1f1a,
			want:                   "domain",
			description:            ConstMagiccfbc97a6,
		},
		{
			subordinateNamespaceID: "invalid",
			want:                   "",
			description:            ConstMagic1813c675,
		},
		{
			subordinateNamespaceID: "",
			want:                   "",
			description:            ConstMagic41c9861e,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := registry.GetParentNamespace(tt.subordinateNamespaceID)
			if got != tt.want {
				t.Errorf(ConstMagic416be8fb, tt.subordinateNamespaceID, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_LoadSubordinateNamespaces tests that subordinate namespaces are loaded from config
func TestNamespaceRegistry_LoadSubordinateNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf(ConstMagicba706c3d, err)
	}

	// Verify that DefaultNamespaceKernel has subordinate namespaces loaded
	subordinates := registry.GetSubordinateNamespaces(DefaultNamespaceKernel)
	if len(subordinates) == 0 {
		t.Error(ConstMagica65554e2)
	}

	// Verify expected subordinates are present
	expectedSubordinates := map[string]bool{
		DefaultNamespaceKernelCLI:             true,
		DefaultNamespaceKernelMetrics:         true,
		DefaultNamespaceKernel + ":storage":   true,
		DefaultNamespaceKernel + ":scheduler": true,
	}

	for _, subordinate := range subordinates {
		if !expectedSubordinates[subordinate] {
			t.Errorf(ConstMagic6817012c, subordinate)
		}
		delete(expectedSubordinates, subordinate)
	}

	// Check that all expected subordinates were found
	for missing := range expectedSubordinates {
		t.Errorf(ConstMagic7088b332, missing)
	}
}

func TestLoadNamespaces_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := fileutil.WriteFile(filepath.Join(dir, name), []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	write("stampmemo_ns_a.yaml", "kind: stampmemo_ns_a\nnamespace_id: domain:alpha\n")
	first := NewNamespaceRegistry(dir)
	if err := first.LoadNamespaces(); err != nil {
		t.Fatal(err)
	}
	if got := first.GetNamespaceForKind("stampmemo_ns_a"); got != "domain:alpha" {
		t.Fatalf("got %q", got)
	}
	write("stampmemo_ns_b.yaml", "kind: stampmemo_ns_b\nnamespace_id: domain:beta\n")
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(dir, later, later); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.Chtimes(filepath.Join(dir, "stampmemo_ns_b.yaml"), later, later); err != nil {
		t.Fatal(err)
	}
	second := NewNamespaceRegistry(dir)
	if err := second.LoadNamespaces(); err != nil {
		t.Fatal(err)
	}
	if got := second.GetNamespaceForKind("stampmemo_ns_b"); got != "domain:beta" {
		t.Fatalf("expected overlay reload, got %q", got)
	}
}
