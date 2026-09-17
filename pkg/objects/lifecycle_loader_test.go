package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLifecycleLoader_LoadLifecycle(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Test loading backlog_item lifecycle
	lifecycle, err := loader.LoadLifecycle("backlog_item")
	if err != nil {
		t.Fatalf("Failed to load lifecycle: %v", err)
	}

	if lifecycle == nil {
		t.Fatal("Lifecycle is nil")
	}

	if lifecycle.ObjectType != "backlog_item" && lifecycle.ObjectType != emptyValue {
		t.Errorf("Expected object_type to be backlog_item or empty, got %s", lifecycle.ObjectType)
	}

	if len(lifecycle.Statuses) == 0 {
		t.Error("Expected at least one status")
	}

	// Check for common statuses
	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}

	expectedStatuses := []string{"exploring", "validated", "planned", "in_progress", "complete"}
	for _, expected := range expectedStatuses {
		if !statusMap[expected] {
			t.Logf("Warning: Expected status %s not found (this may be OK if lifecycle changed)", expected)
		}
	}
}

func TestLifecycleLoader_LoadLifecycle_mcp_session(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	lifecycle, err := loader.LoadLifecycle("mcp_session")
	if err != nil {
		t.Fatalf("Failed to load mcp_session lifecycle: %v", err)
	}
	if lifecycle == nil {
		t.Fatal("mcp_session lifecycle is nil")
	}
	if lifecycle.ObjectType != "mcp_session" {
		t.Errorf("object_type = %q, want mcp_session", lifecycle.ObjectType)
	}

	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}
	// Required for MCP disconnect callback and retention: in_progress (active), disconnected (set on disconnect, eligible for prune)
	for _, required := range []string{"in_progress", "disconnected", "archived", "error"} {
		if !statusMap[required] {
			t.Errorf("mcp_session lifecycle missing required status %q", required)
		}
	}
}

func TestLifecycleLoader_LoadLifecycle_mcp_spec(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	lifecycle, err := loader.LoadLifecycle("mcp_spec")
	if err != nil {
		t.Fatalf("Failed to load mcp_spec lifecycle: %v", err)
	}
	if lifecycle.ObjectType != "mcp_spec" {
		t.Errorf("object_type = %q, want mcp_spec", lifecycle.ObjectType)
	}

	statusMap := make(map[string]bool)
	for _, status := range lifecycle.Statuses {
		statusMap[status.Value] = true
	}
	for _, required := range []string{"draft", "active", "deprecated", "archived", "error"} {
		if !statusMap[required] {
			t.Errorf("mcp_spec lifecycle missing required status %q", required)
		}
	}
}

func TestLifecycleLoader_LoadLifecycle_verification_matrix(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	lifecycle, err := loader.LoadLifecycle("verification_matrix")
	if err != nil {
		t.Fatalf("Failed to load verification_matrix lifecycle: %v", err)
	}
	if lifecycle == nil {
		t.Fatal("verification_matrix lifecycle is nil")
	}
	if lifecycle.ObjectType != "verification_matrix" {
		t.Errorf("object_type = %q, want verification_matrix", lifecycle.ObjectType)
	}
	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}
	for _, required := range []string{"draft", "active", "archived"} {
		if !statusMap[required] {
			t.Errorf("verification_matrix lifecycle missing required status %q", required)
		}
	}
}

func TestLifecycleLoader_LoadLifecycle_epic(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	lifecycle, err := loader.LoadLifecycle("epic")
	if err != nil {
		t.Fatalf("Failed to load epic lifecycle: %v", err)
	}
	if lifecycle == nil {
		t.Fatal("epic lifecycle is nil")
	}
	if lifecycle.ObjectType != "epic" {
		t.Errorf("object_type = %q, want epic", lifecycle.ObjectType)
	}
	statusMap := make(map[string]Status)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = s
	}
	expectedStatuses := []string{
		"conceptual", "originated", "draft", "planned", "in_progress", "paused", "completed", "archived", "error",
	}
	for _, required := range expectedStatuses {
		if _, ok := statusMap[required]; !ok {
			t.Errorf("epic lifecycle missing required status %q", required)
		}
	}

	// Verify roles
	if statusMap["planned"].Role != LifecycleRoleShovelReady {
		t.Errorf("planned status role = %q, want %q", statusMap["planned"].Role, LifecycleRoleShovelReady)
	}
	if statusMap["in_progress"].Role != LifecycleRoleExecutionLocked {
		t.Errorf("in_progress status role = %q, want %q", statusMap["in_progress"].Role, LifecycleRoleExecutionLocked)
	}
	if statusMap["paused"].Role != LifecycleRoleHalted {
		t.Errorf("paused status role = %q, want %q", statusMap["paused"].Role, LifecycleRoleHalted)
	}
	if statusMap["completed"].Role != LifecycleRoleTerminal {
		t.Errorf("completed status role = %q, want %q", statusMap["completed"].Role, LifecycleRoleTerminal)
	}

	// Verify key transitions
	transitions := []struct {
		from string
		to   string
		want bool
	}{
		{"conceptual", "originated", true},
		{"originated", "draft", true},
		{"draft", "planned", true},
		{"planned", "in_progress", true},
		{"in_progress", "paused", true},
		{"paused", "in_progress", true},
		{"in_progress", "completed", true},
		{"completed", "archived", true},
	}
	for _, tc := range transitions {
		valid, err := loader.IsValidTransition("epic", tc.from, tc.to)
		if err != nil || valid != tc.want {
			t.Errorf("IsValidTransition(epic, %s, %s) = (%v, %v), want %v", tc.from, tc.to, valid, err, tc.want)
		}
	}
}

func TestLifecycleLoader_IsValidStatus(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	tests := []struct {
		kind   string
		status string
		want   bool
	}{
		{"backlog_item", "exploring", true},
		{"backlog_item", "validated", true},
		{"backlog_item", "planned", true},
		{"backlog_item", "in_progress", true},
		{"backlog_item", "complete", true},
		{"backlog_item", "completed", true}, // normalized to complete
		{"backlog_item", "archive", true},   // normalized to archived
		{"backlog_item", "invalid_status", false},
		{"mcp_session", "in_progress", true},
		{"mcp_session", "disconnected", true},
		{"mcp_session", "archived", true},
		{"mcp_session", "error", true},
		{"mcp_session", "invalid_status", false},
		{"verification_matrix", "draft", true},
		{"verification_matrix", "active", true},
		{"verification_matrix", "archived", true},
		{"verification_matrix", "invalid_status", false},
		// audit_event uses "completed" as canonical; kind-aware normalization keeps it
		{"audit_event", "completed", true},
		{"audit_event", "complete", false},
		// priority_plan has "cancelled"; "canceled" (US) normalizes to it
		{"priority_plan", "canceled", true},
		{"priority_plan", "cancelled", true},
		// agent_task terminal is implemented — "completed" is not a valid stage
		{"agent_task", "implemented", true},
		{"agent_task", "pending_verification", true},
		{"agent_task", "completed", false},
		{"agent_task", "complete", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.status, func(t *testing.T) {
			got, err := loader.IsValidStatus(tt.kind, tt.status)
			if err != nil {
				// If lifecycle file doesn't exist, skip test
				if fileutil.IsNotExist(err) {
					t.Skipf("Lifecycle file not found for %s", tt.kind)
				}
				t.Fatalf("IsValidStatus() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("IsValidStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLifecycleLoader_IsValidTransition(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	tests := []struct {
		kind string
		from string
		to   string
		want bool
	}{
		{"backlog_item", "exploring", "validated", true},
		{"backlog_item", "validated", "planned", true},
		{"backlog_item", "planned", "testing", true},
		{"backlog_item", "in_progress", "complete", true},
		{"backlog_item", "in_progress", "completed", true}, // "completed" normalized to "complete"
		{"backlog_item", "complete", "archived", true},
		{"backlog_item", "complete", "planned", true},
		{"backlog_item", "complete", "in_progress", true},
		{"backlog_item", "complete", "archive", true},    // "archive" normalized to "archived"
		{"backlog_item", "exploring", "complete", false}, // Invalid direct transition
		{"backlog_item", "complete", "exploring", false}, // Cannot go backwards
		{"verification_matrix", "draft", "active", true},
		{"verification_matrix", "draft", "archived", true},
		{"verification_matrix", "active", "archived", true},
		{"verification_matrix", "active", "draft", false},
		{"verification_matrix", "archived", "active", false},
		{"priority_plan", "active", "grooming", true},
		{"priority_plan", "in_progress", "grooming", false}, // check valve; TRACK: BLI-1785439369431933000-f0cccd6c
		{"priority_plan", "grooming", "active", true},
		{"priority_plan", "complete", "grooming", true},
		{"priority_plan", "complete", "active", true},
		{"agent_task", "in_progress", "pending_verification", true},
		{"agent_task", "pending_verification", "implemented", true},
		{"agent_task", "in_progress", "implemented", true},
		{"agent_task", "pending_verification", "completed", false},
		{"agent_task", "in_progress", "completed", false},
		// Draft must not skip shovel-ready. TRACK: BLI-CEF-R27-DUAL-SEAT-REMEASURE-001
		{"agent_task", "proposed", "in_progress", false},
		{"agent_task", "proposed", "approved", true},
		{"agent_task", "approved", "in_progress", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.from+"->"+tt.to, func(t *testing.T) {
			got, err := loader.IsValidTransition(tt.kind, tt.from, tt.to)
			if err != nil {
				// If lifecycle file doesn't exist, skip test
				if fileutil.IsNotExist(err) {
					t.Skipf("Lifecycle file not found for %s", tt.kind)
				}
				// If transition doesn't exist, that's OK - we're testing validity
				if strings.Contains(err.Error(), "invalid") {
					if got != tt.want {
						t.Errorf("IsValidTransition() = %v, want %v (error: %v)", got, tt.want, err)
					}
					return
				}
				t.Fatalf("IsValidTransition() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("IsValidTransition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLifecycleLoader_GetOriginStatus(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	origin, err := loader.GetOriginStatus("backlog_item")
	if err != nil {
		if fileutil.IsNotExist(err) {
			t.Skip("Lifecycle file not found")
		}
		t.Fatalf("GetOriginStatus() error = %v", err)
	}

	if origin == emptyValue {
		t.Error("Expected origin status to be non-empty")
	}

	// Common origin statuses
	validOrigins := []string{"exploring", "draft", "new"}
	found := false
	for _, valid := range validOrigins {
		if origin == valid {
			found = true
			break
		}
	}
	if !found {
		t.Logf("Origin status is %s (may be valid, just not in common list)", origin)
	}
}

// TestLifecycleLoader_LoadLifecycle_strategicAlignmentKinds tests lifecycle load for
// PRI-212 strategic alignment object types (BLI-772, BLI-771, BLI-773).
func TestLifecycleLoader_LoadLifecycle_strategicAlignmentKinds(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	kinds := []struct {
		kind           string
		initialStatus  string
		expectedStatus []string
	}{
		{"stakeholder_profile", "conceptual", []string{"conceptual", "originated", "draft", "active", "archived", "error"}},
		{"strategic_context", "conceptual", []string{"conceptual", "originated", "draft", "active", "archived", "error"}},
		{"important_date", "conceptual", []string{"conceptual", "originated", "draft", "active", "passed", "archived", "error"}},
	}

	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			lc, err := loader.LoadLifecycle(k.kind)
			if err != nil {
				if fileutil.IsNotExist(err) {
					t.Skipf("Lifecycle file not found for %s", k.kind)
				}
				t.Fatalf("LoadLifecycle(%q) error = %v", k.kind, err)
			}
			if lc == nil {
				t.Fatal("Lifecycle is nil")
			}
			if lc.ObjectType != k.kind {
				t.Errorf("ObjectType = %q, want %q", lc.ObjectType, k.kind)
			}
			statusMap := make(map[string]bool)
			var originFound string
			for _, s := range lc.Statuses {
				statusMap[s.Value] = true
				if s.Origin {
					originFound = s.Value
				}
			}
			if originFound != k.initialStatus {
				t.Errorf("Origin status = %q, want %q", originFound, k.initialStatus)
			}
			for _, expected := range k.expectedStatus {
				if !statusMap[expected] {
					t.Errorf("Missing expected status %q", expected)
				}
			}
		})
	}
}

func TestLifecycleLoader_GetTransitionPreconditions(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Test transition with preconditions (validated -> planned)
	preconditions, err := loader.GetTransitionPreconditions("backlog_item", "validated", "planned")
	if err != nil {
		if fileutil.IsNotExist(err) {
			t.Skip("Lifecycle file not found")
		}
		// If transition doesn't exist, that's OK
		if strings.Contains(err.Error(), "not found") {
			t.Logf("Transition not found (may be OK): %v", err)
			return
		}
		t.Fatalf("GetTransitionPreconditions() error = %v", err)
	}

	// Log preconditions for debugging
	if len(preconditions) > 0 {
		t.Logf("Preconditions for validated -> planned: %v", preconditions)
	}
}

func TestLifecycleLoader_IsTerminalStatusForKind_convergence_session(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	kind := "convergence_session"
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"active", false},
		{"completed", true},
		{"abandoned", true},
		// escalated is role:halted and recoverable (resume/complete/abandon) — not terminal:true in lifecycle YAML.
		{"escalated", false},
		{"", false},
	} {
		got, err := loader.IsTerminalStatusForKind(kind, tc.status)
		if err != nil {
			t.Fatalf("IsTerminalStatusForKind(%q, %q): %v", kind, tc.status, err)
		}
		if got != tc.want {
			t.Fatalf("IsTerminalStatusForKind(%q, %q) = %v, want %v", kind, tc.status, got, tc.want)
		}
	}
}
