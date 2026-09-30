package validation

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

// getTestIDPrefixesConfig returns a test-specific IDPrefixesConfig (copy of default)
// This provides isolation for parallel tests by avoiding global config dependencies
func getTestIDPrefixesConfig() *IDPrefixesConfig {
	// Create a copy of the default config for test isolation
	return &IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			"agent_skill":              {"ASK-"},
			"agent_task":               {"ATK-"},
			"persona":                  {"PER-"},
			"backlog_item":             {"BLI-"},
			"milestone":                {"MIL-"},
			"goal":                     {"GOAL-"},
			"workstream":               {"WS-"},
			"priority_plan":            {"PRI-", "PRIO-"},
			"requirement":              {"REQ-", "REQU-"},
			"test_case":                {"TEST-"},
			"criteria":                 {"CRIT-"},
			"decision":                 {"DEC-"},
			"roadmap":                  {"ROAD-"},
			"mission":                  {"MIS-"},
			objects.FieldKeyVision:     {"VIS-"},
			objects.FieldKeyComponent:  {"COMP-"},
			objects.FieldKeyDisplay:    {"DSP-"},
			"account":                  {"ACC-"},
			objects.FieldKeyRole:       {"ROL-"},
			"release":                  {"REL-"},
			"corporate_initiative":     {"CI-"},
			"kind_synonym":             {"SYN-"},
			"audit_event":              {"AUD-"},
			"base_metric":              {"BAS-"},
			"scheduler_health_metric":  {"SHM-"},
			"kind_mapping_metric":      {"KMM-"},
			"command_metric":           {"CMD-"},
			"file_lock_metric":         {"FLM-"},
			"audit_aggregation_metric": {"AAM-"},
			"strategic_plan":           {"STRAT-PLAN-"},
			"workstream_transition":    {"WST-"},
			"policy":                   {"POL-"},
		},
		InferenceRules: InferenceRulesConfig{
			Patterns: []InferencePattern{},
		},
		DefaultStrategy: DefaultStrategyConfig{
			Method: "first_part_upper_3",
		},
	}
}

// getTestPathsConfig returns a test-specific PathsConfig for the actual project
// This provides isolation for parallel tests by avoiding global config dependencies
func getTestPathsConfig(t *testing.T) *PathsConfig {
	specsDir := getTestSpecsDir(t)
	// Get project root (parent of docs/)
	projectRoot := filepath.Dir(filepath.Dir(specsDir))
	return &PathsConfig{
		Paths: map[string]string{
			"object_specs":       specsDir,
			"config_dir":         filepath.Join(projectRoot, paths.ProcessInternalConfigsDir),
			"id_prefixes_config": filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "id_prefixes_config.yaml"),
		},
		SearchStrategy: SearchStrategyConfig{
			RelativePaths: []string{"", "../", "../../", "../../../"},
			Markers:       []string{paths.ProcessInternalDir, paths.ProjectDataDir, "go.mod"},
		},
	}
}

// getTestSpecsDir returns the absolute path to the object_specs directory for tests.
// This ensures tests use the correct specs directory regardless of working directory changes from other tests.
func getTestSpecsDir(t *testing.T) string {
	// Get the directory of this test file using runtime.Caller
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get test file location")
	}
	testDir := filepath.Dir(testFile)

	// Walk up from test file directory (pkg/validation/) to project root
	dir := testDir
	for {
		// Check if we're at project root (has go.mod and .zqk/specs/objects)
		specsDir := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
		if _, err := fileutil.Stat(specsDir); err == nil {
			return specsDir
		}

		// Check for go.mod as project root marker
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// At project root, use the computed specsDir path
			return specsDir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	// Fallback: use relative path from test file directory
	specsDir := filepath.Join(testDir, "..", "..", paths.ProcessInternalObjectSpecsDir)
	absPath, err := filepath.Abs(specsDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}
	return absPath
}

func TestIDValidator_ValidateID(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id      string
		kind    string
		want    bool
		wantErr bool
	}{
		{"BLI-001", "backlog_item", true, false},
		{"BLI-123", "backlog_item", true, false},
		{"MIL-001", "milestone", true, false},
		{"GOAL-123", "goal", true, false},
		{"PRI-208", "priority_plan", true, false},
		{"PRIO-002", "priority_plan", true, false},
		{"REQ-016", "requirement", true, false},
		{"REQU-001", "requirement", true, false},
		{"TEST-011", "test_case", true, false},
		{"CRIT-8193", "criteria", true, false},
		{"INVALID-001", "backlog_item", false, false},
		{"BLI-001", "milestone", false, false},
		{"UNKNOWN-001", "unknown_kind", true, false},                            // Unknown kind is permissive
		{strings.Repeat("x", MaxObjectIDLength+1), "backlog_item", false, true}, // ID length exceeds MaxObjectIDLength
	}

	for _, tt := range tests {
		t.Run(tt.id+"_"+tt.kind, func(t *testing.T) {
			got, err := validator.ValidateID(tt.id, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIDValidator_InferKindFromID(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id   string
		want string
	}{
		{"BLI-001", "backlog_item"},
		{"ASK-1785886324283087000-34320add", "agent_skill"},
		{"ATK-1785886324283087000-12345678", "agent_task"},
		{"MIL-001", "milestone"},
		{"GOAL-123", "goal"},
		{"PRI-208", "priority_plan"},
		{"PRIO-002", "priority_plan"},
		{"REQ-016", "requirement"},
		{"REQU-001", "requirement"},
		{"TEST-011", "test_case"},
		{"CRIT-8193", "criteria"},
		{"UNKNOWN-001", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := validator.InferKindFromID(tt.id)
			if got != tt.want {
				t.Errorf("InferKindFromID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIDValidator_InferKindFromID_LongestPrefix ensures NAMESPACE-REGISTRY-001
// maps to namespace_registry, not namespace (longest matching prefix wins).
func TestIDValidator_InferKindFromID_LongestPrefix(t *testing.T) {
	t.Parallel()
	config := &IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			"namespace":          {"NAM-", "NAMESPACE-"},
			"namespace_registry": {"NSR-", "NAMESPACE-REGISTRY-"},
		},
		InferenceRules:  InferenceRulesConfig{Patterns: []InferencePattern{}},
		DefaultStrategy: DefaultStrategyConfig{Method: "first_part_upper_3"},
	}
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		config,
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id   string
		want string
	}{
		{"NAMESPACE-REGISTRY-001", "namespace_registry"},
		{"NAMESPACE-001", "namespace"},
		{"NAM-001", "namespace"},
		{"NSR-001", "namespace_registry"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := validator.InferKindFromID(tt.id)
			if got != tt.want {
				t.Errorf("InferKindFromID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestIDValidator_InferKindFromID_DefaultPolicy(t *testing.T) {
	t.Parallel()
	config := &IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			"policy": {"POL-CODE-", "POL-DEFAULT-"},
		},
		InferenceRules:  InferenceRulesConfig{Patterns: []InferencePattern{}},
		DefaultStrategy: DefaultStrategyConfig{Method: "first_part_upper_3"},
	}
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		config,
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()
	got := validator.InferKindFromID("POL-DEFAULT-37173b5595dc3bad")
	if got != "policy" {
		t.Fatalf("InferKindFromID(POL-DEFAULT-…) = %q, want policy", got)
	}
}

func TestIDPrefixesConfigYAML_policyIncludesDefault(t *testing.T) {
	t.Parallel()
	root := findProjectRoot()
	if root == emptyValue {
		t.Skip("project root not found")
	}
	data, err := fileutil.ReadFile(filepath.Join(root, paths.ProcessInternalConfigsDir, "id_prefixes_config.yaml"))
	if err != nil {
		t.Fatalf("read id_prefixes_config.yaml: %v", err)
	}
	if !strings.Contains(string(data), "- POL-DEFAULT-") {
		t.Fatal("id_prefixes_config.yaml must list POL-DEFAULT- so object get can infer policy for init-seeded drafts")
	}
}

func TestIDValidator_GetValidPrefixes(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		kind string
		want []string
	}{
		{"backlog_item", []string{"BLI-"}},
		{"priority_plan", []string{"PRI-", "PRIO-"}},
		{"requirement", []string{"REQ-", "REQU-"}},
		{"audit_aggregation_metric", []string{"AAM-"}},
		{"unknown_kind", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := validator.GetValidPrefixes(tt.kind)
			if len(got) != len(tt.want) {
				t.Errorf("GetValidPrefixes() = %v, want %v", got, tt.want)
				return
			}
			for i, prefix := range tt.want {
				if got[i] != prefix {
					t.Errorf("GetValidPrefixes()[%d] = %v, want %v", i, got[i], prefix)
				}
			}
		})
	}
}

func TestIDValidator_AuditAggregationMetric(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	// Test that audit_aggregation_metric pattern is loaded
	prefixes := validator.GetValidPrefixes("audit_aggregation_metric")
	if len(prefixes) == 0 {
		t.Fatal("audit_aggregation_metric pattern not loaded")
	}
	if prefixes[0] != "AAM-" {
		t.Errorf("Expected prefix 'AAM-', got %v", prefixes)
	}

	// Test ID validation
	valid, err := validator.ValidateID("AAM-001", "audit_aggregation_metric")
	if err != nil {
		t.Fatalf("ValidateID returned error: %v", err)
	}
	if !valid {
		t.Error("AAM-001 should be valid for audit_aggregation_metric")
	}

	// Test kind inference
	kind := validator.InferKindFromID("AAM-001")
	if kind != "audit_aggregation_metric" {
		t.Errorf("Expected kind 'audit_aggregation_metric', got %s", kind)
	}
}

func TestParseNamespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		want    *ParsedNamespace
		wantNil bool
	}{
		{
			name:    "legacy format - no namespace",
			id:      "GOAL-123",
			want:    nil,
			wantNil: true,
		},
		{
			name: "short format - goal",
			id:   "goal:GOAL-123",
			want: &ParsedNamespace{
				NamespaceID: "zqk:kernel",
				Layer:       "zqk",
				Domain:      "kernel",
				ObjectType:  "goal",
				ObjectID:    "GOAL-123",
				FullID:      "goal:GOAL-123",
			},
			wantNil: false,
		},
		{
			name: "full format - zqk kernel goal",
			id:   "zqk:kernel:goal:GOAL-123",
			want: &ParsedNamespace{
				NamespaceID: "zqk:kernel",
				Layer:       "zqk",
				Domain:      "kernel",
				ObjectType:  "goal",
				ObjectID:    "GOAL-123",
				FullID:      "zqk:kernel:goal:GOAL-123",
			},
			wantNil: false,
		},
		{
			name: "full format - domain organizational",
			id:   "domain:organizational:organization:ORG-001",
			want: &ParsedNamespace{
				NamespaceID: "domain:organizational",
				Layer:       "domain",
				Domain:      "organizational",
				ObjectType:  "organization",
				ObjectID:    "ORG-001",
				FullID:      "domain:organizational:organization:ORG-001",
			},
			wantNil: false,
		},
		{
			name: "full format - domain with subdomain",
			id:   "domain:financial:accounting:account:ACC-001",
			want: &ParsedNamespace{
				NamespaceID: "domain:financial:accounting",
				Layer:       "domain",
				Domain:      "financial",
				Subdomain:   "accounting",
				ObjectType:  "account",
				ObjectID:    "ACC-001",
				FullID:      "domain:financial:accounting:account:ACC-001",
			},
			wantNil: false,
		},
		{
			name: "full format - integration jira",
			id:   "integration:jira:issue:PROJ-123",
			want: &ParsedNamespace{
				NamespaceID: "integration:jira",
				Layer:       "integration",
				Domain:      "jira",
				ObjectType:  "issue",
				ObjectID:    "PROJ-123",
				FullID:      "integration:jira:issue:PROJ-123",
			},
			wantNil: false,
		},
		{
			name: "full format - zqk kernel without object type",
			id:   "zqk:kernel:GOAL-123",
			want: &ParsedNamespace{
				NamespaceID: "zqk:kernel",
				Layer:       "zqk",
				Domain:      "kernel",
				ObjectID:    "GOAL-123",
				FullID:      "zqk:kernel:GOAL-123",
			},
			wantNil: false,
		},
		{
			name: "full format - domain without object type",
			id:   "domain:organizational:ORG-001",
			want: &ParsedNamespace{
				NamespaceID: "domain:organizational",
				Layer:       "domain",
				Domain:      "organizational",
				ObjectID:    "ORG-001",
				FullID:      "domain:organizational:ORG-001",
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNamespace(tt.id)
			if tt.wantNil {
				if got != nil {
					t.Errorf("ParseNamespace() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ParseNamespace() returned nil, expected non-nil")
			}
			if got.NamespaceID != tt.want.NamespaceID {
				t.Errorf("ParseNamespace().NamespaceID = %v, want %v", got.NamespaceID, tt.want.NamespaceID)
			}
			if got.Layer != tt.want.Layer {
				t.Errorf("ParseNamespace().Layer = %v, want %v", got.Layer, tt.want.Layer)
			}
			if got.Domain != tt.want.Domain {
				t.Errorf("ParseNamespace().Domain = %v, want %v", got.Domain, tt.want.Domain)
			}
			if got.Subdomain != tt.want.Subdomain {
				t.Errorf("ParseNamespace().Subdomain = %v, want %v", got.Subdomain, tt.want.Subdomain)
			}
			if got.ObjectType != tt.want.ObjectType {
				t.Errorf("ParseNamespace().ObjectType = %v, want %v", got.ObjectType, tt.want.ObjectType)
			}
			if got.ObjectID != tt.want.ObjectID {
				t.Errorf("ParseNamespace().ObjectID = %v, want %v", got.ObjectID, tt.want.ObjectID)
			}
			if got.FullID != tt.want.FullID {
				t.Errorf("ParseNamespace().FullID = %v, want %v", got.FullID, tt.want.FullID)
			}
		})
	}
}

func TestIDValidator_ValidateID_WithNamespace(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id          string
		kind        string
		want        bool
		wantErr     bool
		description string
	}{
		// Legacy format (no namespace)
		{"GOAL-123", "goal", true, false, "Legacy format goal ID"},
		{"BLI-001", "backlog_item", true, false, "Legacy format backlog item ID"},
		// Short format (assumes zqk:kernel)
		{"goal:GOAL-123", "goal", true, false, "Short format goal ID"},
		{"backlog_item:BLI-001", "backlog_item", true, false, "Short format backlog item ID"},
		// Full format
		{"zqk:kernel:goal:GOAL-123", "goal", true, false, "Full format zqk kernel goal"},
		{"zqk:kernel:backlog_item:BLI-001", "backlog_item", true, false, "Full format zqk kernel backlog item"},
		{"domain:organizational:organization:ORG-001", "organization", true, false, "Full format domain organizational"},
		// Invalid namespace format
		{"invalid:namespace:goal:GOAL-123", "goal", false, true, "Invalid namespace layer"},
		// Valid ID but wrong kind
		{"zqk:kernel:goal:GOAL-123", "backlog_item", false, false, "Valid ID but wrong kind"},
		// Invalid namespace format - malformed
		{"unknown:layer:goal:GOAL-123", "goal", false, true, "Unknown namespace layer"},
		// Domain with subdomain
		{"domain:financial:accounting:account:ACC-001", "account", true, false, "Domain with subdomain"},
		// Integration namespace
		{"integration:jira:issue:PROJ-123", "issue", true, false, "Integration namespace"},
		// Invalid namespace format - too many colons
		// ParseNamespace joins parts[3:] with ":", so "GOAL-123:extra" becomes the object ID
		// This is actually valid, just has extra in the ID
		{"zqk:kernel:goal:GOAL-123:extra", "goal", true, false, "Extra colons become part of object ID"},
		// Invalid namespace ID format
		{"zqk:INVALID_CHARS:goal:GOAL-123", "goal", false, true, "Invalid namespace ID format (uppercase)"},
		// Short format with unknown prefix - treated as short format, not error
		{"unknown:GOAL-123", "goal", true, false, "Short format with unknown prefix (treated as kind:id)"},
	}

	for _, tt := range tests {
		t.Run(tt.id+"_"+tt.kind, func(t *testing.T) {
			got, err := validator.ValidateID(tt.id, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v (%s)", err, tt.wantErr, tt.description)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateID() = %v, want %v (%s)", got, tt.want, tt.description)
			}
		})
	}
}

// TestIDValidator_ValidateID_AccountSpecialHandling tests special account ID handling
func TestIDValidator_ValidateID_AccountSpecialHandling(t *testing.T) {
	t.Parallel()
	validator := NewIDValidator(getTestSpecsDir(t))
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id          string
		kind        string
		want        bool
		wantErr     bool
		description string
	}{
		{"account:username", "account", false, false, "Account format account:username no longer valid"},
		{"account:testuser", "account", false, false, "Account format with username no longer valid"},
		{"ACC-001", "account", true, false, "Account with ACC- prefix"},
		// account:invalid-format - actual behavior shows it's valid (pattern might be permissive)
		{"account:invalid-format", "account", false, false, "Account format is no longer permissive"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := validator.ValidateID(tt.id, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v (%s)", err, tt.wantErr, tt.description)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateID() = %v, want %v (%s)", got, tt.want, tt.description)
			}
		})
	}
}

// TestIDValidator_ValidateID_ErrorPaths tests error handling paths
func TestIDValidator_ValidateID_ErrorPaths(t *testing.T) {
	t.Parallel()
	validator := NewIDValidator(getTestSpecsDir(t))
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id          string
		kind        string
		wantErr     bool
		errContains string
		description string
	}{
		{
			id:   "invalid:layer:goal:GOAL-123",
			kind: "goal",
			// ParseNamespace doesn't recognize "invalid" as a valid layer, so it returns nil
			// Then the code checks if id contains ":" and has >2 parts, which it does
			// So it returns an error with "unknown layer: invalid"
			wantErr:     true,
			errContains: "unknown layer",
			description: "Invalid namespace layer should return error",
		},
		{
			id:          "zqk:INVALID_FORMAT:goal:GOAL-123",
			kind:        "goal",
			wantErr:     true,
			errContains: "invalid namespace format",
			description: "Invalid namespace format (uppercase) should return error",
		},
		{
			id:   "unknown:GOAL-123",
			kind: "goal",
			// "unknown:GOAL-123" has 2 parts, so it's treated as short format
			// ParseNamespace treats it as short format (unknown is not a valid layer)
			// So it doesn't error, just validates GOAL-123
			wantErr:     false,
			errContains: "",
			description: "Unknown layer in 2-part format treated as short format (no error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			_, err := validator.ValidateID(tt.id, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v (%s)", err, tt.wantErr, tt.description)
				return
			}
			if tt.wantErr && err != nil {
				if tt.errContains != emptyValue && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateID() error = %v, want error containing %q (%s)", err, tt.errContains, tt.description)
				}
			}
		})
	}
}

// TestIDValidator_ValidateID_PrefixValidation tests prefix-based validation
func TestIDValidator_ValidateID_PrefixValidation(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id          string
		kind        string
		want        bool
		description string
	}{
		{"PRI-208", "priority_plan", true, "Priority plan with PRI- prefix"},
		{"PRIO-002", "priority_plan", true, "Priority plan with PRIO- prefix"},
		{"REQ-016", "requirement", true, "Requirement with REQ- prefix"},
		{"REQU-001", "requirement", true, "Requirement with REQU- prefix"},
		{"INVALID-001", "priority_plan", false, "Invalid prefix for priority plan"},
		{"PRI-001", "requirement", false, "Wrong prefix for requirement"},
	}

	for _, tt := range tests {
		t.Run(tt.id+"_"+tt.kind, func(t *testing.T) {
			got, err := validator.ValidateID(tt.id, tt.kind)
			if err != nil {
				t.Errorf("ValidateID() error = %v (%s)", err, tt.description)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateID() = %v, want %v (%s)", got, tt.want, tt.description)
			}
		})
	}
}

func TestIDValidator_InferKindFromID_WithNamespace(t *testing.T) {
	t.Parallel()
	// Use test-specific configs for complete isolation (no global state dependencies)
	validator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = validator.LoadPatterns()

	tests := []struct {
		id   string
		want string
	}{
		// Legacy format (no namespace)
		{"GOAL-123", "goal"},
		{"BLI-001", "backlog_item"},
		// Short format (assumes zqk:kernel)
		{"goal:GOAL-123", "goal"},
		{"backlog_item:BLI-001", "backlog_item"},
		// Full format
		{"zqk:kernel:goal:GOAL-123", "goal"},
		{"zqk:kernel:backlog_item:BLI-001", "backlog_item"},
		{"domain:organizational:organization:ORG-001", "organization"},
		// Unknown ID
		{"UNKNOWN-001", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := validator.InferKindFromID(tt.id)
			if got != tt.want {
				t.Errorf("InferKindFromID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractPrefixesFromPattern tests the extractPrefixesFromPattern function
func TestExtractPrefixesFromPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		pattern     string
		want        []string
		description string
	}{
		{
			name:        "simple prefix pattern",
			pattern:     "^PRI-\\d{3,}$",
			want:        []string{"PRI-"},
			description: "Pattern with simple prefix like ^PRI-",
		},
		{
			name:        "alternation pattern",
			pattern:     "^(PRI|PRIO)-\\d{3,}$",
			want:        []string{"PRI-", "PRIO-"},
			description: "Pattern with alternation like (PRI|PRIO)-",
		},
		{
			name:        "alternation with multiple options",
			pattern:     "^(REQ|REQU|REQUI)-\\d{3,}$",
			want:        []string{"REQ-", "REQU-", "REQUI-"},
			description: "Pattern with multiple alternation options",
		},
		{
			name:        "generic pattern",
			pattern:     "^[A-Z]+-\\d{3,}$",
			want:        []string{},
			description: "Generic pattern without literal prefix returns empty",
		},
		{
			name:        "pattern without prefix",
			pattern:     "^\\d{3,}$",
			want:        []string{},
			description: "Pattern without prefix returns empty",
		},
		{
			name:        "empty pattern",
			pattern:     "",
			want:        []string{},
			description: "Empty pattern returns empty",
		},
		{
			name:    "pattern with spaces in alternation",
			pattern: "^(PRI |PRIO )-\\d{3,}$",
			// The regex `\(([A-Z\|]+)\)-` only matches [A-Z|], so spaces are not captured
			// Actual behavior: pattern doesn't match because of space
			want:        []string{},
			description: "Pattern with spaces in alternation doesn't match [A-Z]+ pattern",
		},
		{
			name:    "long prefix with underscore",
			pattern: "^BACKLOG_ITEM-\\d{3,}$",
			// The regex `\^([A-Z]+)-` only matches [A-Z]+, underscore breaks the match
			want:        []string{},
			description: "Pattern with underscore doesn't match [A-Z]+ pattern",
		},
		{
			name:        "lowercase prefix (not matched)",
			pattern:     "^pri-\\d{3,}$",
			want:        []string{},
			description: "Lowercase prefix not matched by [A-Z]+ pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPrefixesFromPattern(tt.pattern)
			if len(got) != len(tt.want) {
				t.Errorf("extractPrefixesFromPattern(%q) = %v, want %v (%s)", tt.pattern, got, tt.want, tt.description)
				return
			}
			for i, prefix := range tt.want {
				if i >= len(got) || got[i] != prefix {
					t.Errorf("extractPrefixesFromPattern(%q)[%d] = %v, want %v (%s)", tt.pattern, i, got[i], prefix, tt.description)
				}
			}
		})
	}
}

// TestDiscoverSpecsDir tests the discoverSpecsDir function
func TestDiscoverSpecsDir(t *testing.T) {
	// This function tries to find the specs directory
	// We can test that it returns a valid path or empty string
	result := discoverSpecsDir()

	// It should either return a valid directory path or empty string
	if result != emptyValue {
		// If it found a directory, verify it exists and is a directory
		info, err := fileutil.Stat(result)
		if err != nil {
			t.Errorf("discoverSpecsDir() returned path that doesn't exist: %q, error: %v", result, err)
		} else if !info.IsDir() {
			t.Errorf("discoverSpecsDir() returned path that is not a directory: %q", result)
		}

		// The path should be absolute
		if !filepath.IsAbs(result) {
			t.Errorf("discoverSpecsDir() should return absolute path, got: %q", result)
		}
	}

	// Test that the function is deterministic (calling it multiple times gives same result)
	result2 := discoverSpecsDir()
	if result != result2 {
		t.Errorf("discoverSpecsDir() returned different results: %q vs %q", result, result2)
	}
}
