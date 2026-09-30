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
		t.Fatalf("Failed to load namespaces: %v", err)
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
				t.Errorf("GetNamespaceForKind(%q) = %q, want %q (%s)", tt.kind, got, tt.want, tt.description)
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
		t.Errorf("GetNamespaceForKind(\"custom_kind\") = %q, want %q", got, "domain:custom")
	}
}

func TestNamespaceRegistry_GetAllNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf("Failed to load namespaces: %v", err)
	}

	namespaces := registry.GetAllNamespaces()

	// Should have at least kernel and organizational namespaces
	if len(namespaces) == 0 {
		t.Error("GetAllNamespaces() returned empty map")
	}

	// Check that we have some expected mappings
	if namespace, ok := namespaces["goal"]; !ok || namespace != DefaultNamespaceKernel {
		t.Errorf("Expected goal -> DefaultNamespaceKernel, got %q", namespace)
	}

	if namespace, ok := namespaces["organization"]; !ok || namespace != "domain:organizational" {
		t.Errorf("Expected organization -> domain:organizational, got %q", namespace)
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
				t.Errorf("inferNamespaceFromKind(%q) = %q, want %q (%s)", tt.kind, got, tt.want, tt.description)
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
		t.Errorf("inferNamespaceFromKind(\"custom_kind\") with registered namespace = %q, want domain:custom", got)
	}

	// Unregistered kind should still use inference
	got = registry.inferNamespaceFromKind("organization")
	if got != "domain:organizational" {
		t.Errorf("inferNamespaceFromKind(\"organization\") = %q, want domain:organizational", got)
	}
}

// TestGetNamespaceRegistry tests the singleton pattern for GetNamespaceRegistry
func TestGetNamespaceRegistry(t *testing.T) {
	t.Parallel()
	// First call should initialize the singleton
	registry1 := GetNamespaceRegistry()
	if registry1 == nil {
		t.Fatal("GetNamespaceRegistry() returned nil")
	}

	// Second call should return the same instance (singleton)
	registry2 := GetNamespaceRegistry()
	if registry1 != registry2 {
		t.Error("GetNamespaceRegistry() returned different instances (not a singleton)")
	}

	// Verify it's a valid registry by checking it can load namespaces
	if err := registry1.LoadNamespaces(); err != nil {
		t.Errorf("GetNamespaceRegistry() returned registry that failed to load namespaces: %v", err)
	}

	// Verify it has default namespaces loaded
	namespaces := registry1.GetAllNamespaces()
	if len(namespaces) == 0 {
		t.Error("GetNamespaceRegistry() returned registry with no namespaces")
	}

	// Verify it can resolve a known kind
	namespace := registry1.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf("GetNamespaceRegistry() registry returned wrong namespace for 'goal': got %q, want DefaultNamespaceKernel", namespace)
	}
}

// TestGetNamespaceRegistry_ConcurrentAccess tests concurrent access to the singleton
func TestGetNamespaceRegistry_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	// Test concurrent access to ensure thread safety
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.StartTestGoroutine(fmt.Sprintf("test_registry_access_%d", i), fmt.Sprintf("accessing namespace registry %d", i), func() {
			registry := GetNamespaceRegistry()
			if registry == nil {
				t.Error("GetNamespaceRegistry() returned nil in concurrent access")
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
		t.Error("GetNamespaceRegistry() singleton pattern broken under concurrent access")
	}
}

// TestNamespaceRegistry_LoadDefaultNamespaces tests that default namespaces are loaded
// This tests loadDefaultNamespaces() indirectly through LoadNamespaces()
func TestNamespaceRegistry_LoadDefaultNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")

	// LoadNamespaces should call loadDefaultNamespaces internally
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf("LoadNamespaces() failed: %v", err)
	}

	// Verify default kernel namespaces are loaded
	kernelKinds := []string{"goal", "milestone", "workstream", "backlog_item", "account", "role"}
	for _, kind := range kernelKinds {
		namespace := registry.GetNamespaceForKind(kind)
		if namespace != DefaultNamespaceKernel {
			t.Errorf("Default namespace for %q = %q, want DefaultNamespaceKernel", kind, namespace)
		}
	}

	// Verify default organizational namespaces are loaded
	orgKinds := []string{"organization", "division", "department", "team", "partnership"}
	for _, kind := range orgKinds {
		namespace := registry.GetNamespaceForKind(kind)
		if namespace != "domain:organizational" {
			t.Errorf("Default namespace for %q = %q, want domain:organizational", kind, namespace)
		}
	}

	// Verify all default namespaces are present
	allNamespaces := registry.GetAllNamespaces()
	expectedCount := len(kernelKinds) + len(orgKinds)
	if len(allNamespaces) < expectedCount {
		t.Errorf("GetAllNamespaces() returned %d namespaces, expected at least %d", len(allNamespaces), expectedCount)
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
		t.Fatal("NewNamespaceRegistry(\"\") returned nil")
	}

	// The registry should be able to load namespaces if the directory is found
	// If the directory is not found, it should still work with defaults
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf("LoadNamespaces() failed (may be expected if specs dir not found): %v", err)
	}

	// Even if specs dir is not found, default namespaces should still work
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf("GetNamespaceForKind(\"goal\") = %q, want DefaultNamespaceKernel (should work even if specs dir not found)", namespace)
	}
}

// TestNewNamespaceRegistry_WithSpecsDir tests explicit specs directory
func TestNewNamespaceRegistry_WithSpecsDir(t *testing.T) {
	t.Parallel()
	// Test with explicit specs directory (if it exists)
	specsDir := paths.ProcessInternalObjectSpecsDir

	registry := NewNamespaceRegistry(specsDir)
	if registry == nil {
		t.Fatal("NewNamespaceRegistry(specsDir) returned nil")
	}

	// Should be able to load namespaces
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf("LoadNamespaces() failed (may be expected if specs dir doesn't exist): %v", err)
	}

	// Should still have default namespaces
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf("GetNamespaceForKind(\"goal\") = %q, want DefaultNamespaceKernel", namespace)
	}
}

// TestNewNamespaceRegistry_WithInvalidSpecsDir tests behavior with invalid directory
func TestNewNamespaceRegistry_WithInvalidSpecsDir(t *testing.T) {
	t.Parallel()
	// Test with a non-existent directory
	invalidDir := "/nonexistent/path/to/specs"

	registry := NewNamespaceRegistry(invalidDir)
	if registry == nil {
		t.Fatal("NewNamespaceRegistry(invalidDir) returned nil")
	}

	// Should still be able to load namespaces (using defaults)
	err := registry.LoadNamespaces()
	if err != nil {
		t.Logf("LoadNamespaces() failed (may be expected): %v", err)
	}

	// Default namespaces should still work
	namespace := registry.GetNamespaceForKind("goal")
	if namespace != DefaultNamespaceKernel {
		t.Errorf("GetNamespaceForKind(\"goal\") = %q, want DefaultNamespaceKernel (should work with defaults)", namespace)
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
			name: "explicit namespace_id",
			specContent: `kind: test_object
namespace_id: domain:test`,
			kind:        "test_object",
			want:        "domain:test",
			description: "Spec with explicit namespace_id field",
		},
		{
			name: "namespace field",
			specContent: `kind: test_object
namespace: domain:test_namespace`,
			kind:        "test_object",
			want:        "domain:test_namespace",
			description: "Spec with namespace field (fallback)",
		},
		{
			name: "fields.namespace_id.validation.default",
			specContent: `kind: test_object
fields:
  namespace_id:
    validation:
      default: domain:test_fields`,
			kind:        "test_object",
			want:        "domain:test_fields",
			description: "Spec with default in fields.namespace_id.validation.default",
		},
		{
			name: "ontology field used when kind missing",
			specContent: `ontology: test_ontology_only
# No kind field`,
			kind:        "test_object",
			want:        DefaultNamespaceKernel, // Will infer from ontology, then from kind parameter
			description: "Spec with ontology but no kind field",
		},
		{
			name: "infer from ontology",
			specContent: `kind: test_object
ontology: organization`,
			kind:        "test_object",
			want:        "domain:organizational", // Inferred from ontology
			description: "Spec with ontology that maps to organizational domain",
		},
		{
			name:        "infer from kind parameter",
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
			description: "Spec with kind field takes precedence over parameter",
		},
		{
			name: "invalid YAML",
			specContent: `kind: test_object
invalid: yaml: [unclosed bracket`,
			kind:        "test_object",
			want:        "", // Invalid YAML should return empty string
			description: "Spec with invalid YAML should return empty string",
		},
		{
			name:        "missing file",
			specContent: "",
			kind:        "test_object",
			want:        "", // Missing file should return empty string
			description: "Non-existent spec file should return empty string",
		},
		{
			name:        "empty spec",
			specContent: `# Empty spec`,
			kind:        "test_object",
			want:        DefaultNamespaceKernel, // Empty spec should infer from kind parameter
			description: "Empty spec should infer from kind parameter",
		},
		{
			name: "precedence: namespace_id over namespace",
			specContent: `kind: test_object
namespace_id: domain:test_id
namespace: domain:test_namespace`,
			kind:        "test_object",
			want:        "domain:test_id", // namespace_id takes precedence
			description: "namespace_id should take precedence over namespace field",
		},
		{
			name: "precedence: namespace over fields default",
			specContent: `kind: test_object
namespace: domain:test_namespace
fields:
  namespace_id:
    validation:
      default: domain:test_fields`,
			kind:        "test_object",
			want:        "domain:test_namespace", // namespace takes precedence over fields default
			description: "namespace field should take precedence over fields default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create spec file
			specPath := filepath.Join(tmpDir, tt.kind+".yaml")
			if tt.specContent != emptyValue {
				if err := fileutil.WriteFile(specPath, []byte(tt.specContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write spec file: %v", err)
				}
			} else {
				// For missing file test, use a non-existent path
				specPath = filepath.Join(tmpDir, "nonexistent.yaml")
			}

			got := registry.loadNamespaceFromSpec(specPath, tt.kind)
			if got != tt.want {
				t.Errorf("loadNamespaceFromSpec(%q, %q) = %q, want %q (%s)", specPath, tt.kind, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_GetSubordinateNamespaces tests subordinate namespace retrieval
func TestNamespaceRegistry_GetSubordinateNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf("Failed to load namespaces: %v", err)
	}

	tests := []struct {
		parentNamespaceID string
		want              []string
		description       string
	}{
		{
			parentNamespaceID: DefaultNamespaceKernel,
			want:              []string{DefaultNamespaceKernelCLI, DefaultNamespaceKernelMetrics, DefaultNamespaceKernel + ":storage", DefaultNamespaceKernel + ":scheduler"},
			description:       "DefaultNamespaceKernel should have subordinate namespaces",
		},
		{
			parentNamespaceID: "domain:organizational",
			want:              []string{},
			description:       "domain:organizational should have no subordinate namespaces",
		},
		{
			parentNamespaceID: "nonexistent:namespace",
			want:              []string{},
			description:       "Non-existent namespace should return empty slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.parentNamespaceID, func(t *testing.T) {
			got := registry.GetSubordinateNamespaces(tt.parentNamespaceID)
			if len(got) != len(tt.want) {
				t.Errorf("GetSubordinateNamespaces(%q) returned %d namespaces, want %d (%s)", tt.parentNamespaceID, len(got), len(tt.want), tt.description)
				return
			}

			// Check that all expected subordinates are present
			gotMap := make(map[string]bool)
			for _, ns := range got {
				gotMap[ns] = true
			}

			for _, expected := range tt.want {
				if !gotMap[expected] {
					t.Errorf("GetSubordinateNamespaces(%q) missing expected namespace %q (%s)", tt.parentNamespaceID, expected, tt.description)
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
			description:       "DefaultNamespaceKernel:cli is subordinate to DefaultNamespaceKernel",
		},
		{
			childNamespaceID:  DefaultNamespaceKernelMetrics,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              true,
			description:       "DefaultNamespaceKernel:metrics is subordinate to DefaultNamespaceKernel",
		},
		{
			childNamespaceID:  DefaultNamespaceKernel + ":storage",
			parentNamespaceID: DefaultNamespaceKernel,
			want:              true,
			description:       "DefaultNamespaceKernel:storage is subordinate to DefaultNamespaceKernel",
		},
		{
			childNamespaceID:  DefaultNamespaceKernel,
			parentNamespaceID: DefaultNamespaceKernel,
			want:              false,
			description:       "Namespace is not subordinate to itself",
		},
		{
			childNamespaceID:  DefaultNamespaceKernelCLI,
			parentNamespaceID: DefaultNamespaceKernelMetrics,
			want:              false,
			description:       "Sibling namespaces are not subordinate to each other",
		},
		{
			childNamespaceID:  "domain:organizational",
			parentNamespaceID: DefaultNamespaceKernel,
			want:              false,
			description:       "Domain namespace is not subordinate to kernel",
		},
		{
			childNamespaceID:  DefaultNamespaceKernelCLI + ":sub",
			parentNamespaceID: DefaultNamespaceKernelCLI,
			want:              true,
			description:       "Nested subordinate namespace (DefaultNamespaceKernel:cli:sub is subordinate to DefaultNamespaceKernel:cli)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := registry.IsSubordinateOf(tt.childNamespaceID, tt.parentNamespaceID)
			if got != tt.want {
				t.Errorf("IsSubordinateOf(%q, %q) = %v, want %v (%s)", tt.childNamespaceID, tt.parentNamespaceID, got, tt.want, tt.description)
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
			description:            "DefaultNamespaceKernel:cli parent is DefaultNamespaceKernel",
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernelMetrics,
			want:                   DefaultNamespaceKernel,
			description:            "DefaultNamespaceKernel:metrics parent is DefaultNamespaceKernel",
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernel + ":storage",
			want:                   DefaultNamespaceKernel,
			description:            "DefaultNamespaceKernel:storage parent is DefaultNamespaceKernel",
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernelCLI + ":sub",
			want:                   DefaultNamespaceKernelCLI,
			description:            "Nested subordinate namespace parent extraction",
		},
		{
			subordinateNamespaceID: DefaultNamespaceKernel,
			want:                   "zqk", // Root namespace (first layer)
			description:            "DefaultNamespaceKernel parent is zqk (root namespace)",
		},
		{
			subordinateNamespaceID: "domain:organizational",
			want:                   "domain",
			description:            "domain:organizational parent is domain (root namespace)",
		},
		{
			subordinateNamespaceID: "invalid",
			want:                   "",
			description:            "Invalid format returns empty string",
		},
		{
			subordinateNamespaceID: "",
			want:                   "",
			description:            "Empty string returns empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := registry.GetParentNamespace(tt.subordinateNamespaceID)
			if got != tt.want {
				t.Errorf("GetParentNamespace(%q) = %q, want %q (%s)", tt.subordinateNamespaceID, got, tt.want, tt.description)
			}
		})
	}
}

// TestNamespaceRegistry_LoadSubordinateNamespaces tests that subordinate namespaces are loaded from config
func TestNamespaceRegistry_LoadSubordinateNamespaces(t *testing.T) {
	t.Parallel()
	registry := NewNamespaceRegistry("")
	if err := registry.LoadNamespaces(); err != nil {
		t.Fatalf("Failed to load namespaces: %v", err)
	}

	// Verify that DefaultNamespaceKernel has subordinate namespaces loaded
	subordinates := registry.GetSubordinateNamespaces(DefaultNamespaceKernel)
	if len(subordinates) == 0 {
		t.Error("DefaultNamespaceKernel should have subordinate namespaces loaded from config")
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
			t.Errorf("Unexpected subordinate namespace: %q", subordinate)
		}
		delete(expectedSubordinates, subordinate)
	}

	// Check that all expected subordinates were found
	for missing := range expectedSubordinates {
		t.Errorf("Missing expected subordinate namespace: %q", missing)
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
