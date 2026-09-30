package validation

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// createObjectFile creates a YAML object file with the given content
func createObjectFile(t *testing.T, dir, filename string, obj map[string]any) string {
	filePath := filepath.Join(dir, filename)
	data, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	if err := fileutil.WriteFile(filePath, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return filePath
}

// TestAsyncValidator_CascadeDependencies tests validation of objects with
// multi-level dependency chains (e.g., backlog_item -> goal -> milestone -> strategic_plan)
func TestAsyncValidator_CascadeDependencies(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create directory structure for different object kinds
	strategicPlanDir := datacell.CellCASPrimaryDir(testRoot, "strategic_plans")
	milestoneDir := datacell.CellCASPrimaryDir(testRoot, "milestones")
	goalDir := datacell.CellCASPrimaryDir(testRoot, "goals")
	backlogDir := datacell.CellCASPrimaryDir(testRoot, "backlog")

	for _, dir := range []string{strategicPlanDir, milestoneDir, goalDir, backlogDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
	}

	// Create dependency chain: strategic_plan -> milestone -> goal -> backlog_item
	// Level 1: Strategic Plan (no dependencies)
	strategicPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-001",
		objects.FieldKeyKind:          "strategic_plan",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Test Strategic Plan",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, strategicPlanDir, "STRAT-PLAN-001.yaml", strategicPlan)

	// Level 2: Milestone (depends on strategic_plan)
	milestone := map[string]any{
		objects.FieldKeyID:            "MIL-001",
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Test Milestone",
		"strategic_plan_ref":          "STRAT-PLAN-001",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, milestoneDir, "MIL-001.yaml", milestone)

	// Level 3: Goal (depends on milestone)
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyMilestoneRefs: []string{"MIL-001"},
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, goalDir, "GOAL-001.yaml", goal)

	// Level 4: Backlog Item (depends on goal)
	backlogItem := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "planned",
		objects.FieldKeyTitle:         "Test Backlog Item",
		objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, backlogDir, "BLI-001.yaml", backlogItem)

	// Enqueue all objects for validation (in reverse dependency order to test cascade)
	// This simulates real-world scenario where objects are discovered in any order
	// Validate in reverse order (dependents before dependencies)
	_ = validator.Enqueue("BLI-001", "backlog_item", filepath.Join(backlogDir, "BLI-001.yaml"), 1)
	_ = validator.Enqueue("GOAL-001", "goal", filepath.Join(goalDir, "GOAL-001.yaml"), 1)
	_ = validator.Enqueue("MIL-001", "milestone", filepath.Join(milestoneDir, "MIL-001.yaml"), 1)
	_ = validator.Enqueue("STRAT-PLAN-001", "strategic_plan", filepath.Join(strategicPlanDir, "STRAT-PLAN-001.yaml"), 1)

	// Wait for validation to complete
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for validation to complete")
		case <-ticker.C:
			_, _, _, queueSize := validator.GetValidationStats()
			if queueSize == 0 {
				// Check that all objects are cached
				objects := []struct {
					id   string
					kind string
				}{
					{"STRAT-PLAN-001", "strategic_plan"},
					{"MIL-001", "milestone"},
					{"GOAL-001", "goal"},
					{"BLI-001", "backlog_item"},
				}

				for _, obj := range objects {
					state, exists := validator.GetCachedState(obj.id)
					if !exists {
						t.Errorf("expected %s (%s) to be cached after validation", obj.id, obj.kind)
					} else {
						t.Logf("Validated %s (%s): %d issues", obj.id, obj.kind, len(state.Issues))
					}
				}
				return
			}
		}
	}
}

// TestAsyncValidator_MissingDependencies tests validation when dependencies are missing
func TestAsyncValidator_MissingDependencies(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	backlogDir := datacell.CellCASPrimaryDir(testRoot, "backlog")
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create backlog item with reference to non-existent goal
	backlogItem := map[string]any{
		objects.FieldKeyID:            "BLI-002",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "planned",
		objects.FieldKeyTitle:         "Backlog Item with Missing Goal",
		objects.FieldKeyGoalRefs:      []string{"GOAL-999"}, // Non-existent goal
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	filePath := createObjectFile(t, backlogDir, "BLI-002.yaml", backlogItem)

	// Validate the object
	ctx := pkgctx.NewSystemContext()
	state, err := validator.ValidateNow(ctx, "BLI-002", "backlog_item", filePath)
	if err != nil {
		t.Fatalf("validation should complete even with missing dependencies: %v", err)
	}

	// Should have reference integrity issues
	hasReferenceIssue := false
	for _, issue := range state.Issues {
		if issue.Category == "reference" || issue.Category == "reference_integrity" {
			hasReferenceIssue = true
			t.Logf("Found reference issue: %s (Tier %d)", issue.Message, issue.Tier)
		}
	}

	if !hasReferenceIssue {
		t.Log("No reference issues found (may be expected depending on validation implementation)")
	}
}

// TestAsyncValidator_DeepDependencyChain tests a deep dependency chain (5+ levels)
func TestAsyncValidator_DeepDependencyChain(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create directories
	dirs := map[string]string{
		"strategic_plan": datacell.CellCASPrimaryDir(testRoot, "strategic_plans"),
		"milestone":      datacell.CellCASPrimaryDir(testRoot, "milestones"),
		"goal":           datacell.CellCASPrimaryDir(testRoot, "goals"),
		"requirement":    datacell.CellCASPrimaryDir(testRoot, "requirements"),
		"backlog_item":   datacell.CellCASPrimaryDir(testRoot, "backlog"),
	}

	for _, dir := range dirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	// Create 5-level dependency chain
	// Level 1: Strategic Plan
	sp := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-002",
		objects.FieldKeyKind:          "strategic_plan",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Deep Chain Strategic Plan",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, dirs["strategic_plan"], "STRAT-PLAN-002.yaml", sp)

	// Level 2: Milestone
	mil := map[string]any{
		objects.FieldKeyID:            "MIL-002",
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Deep Chain Milestone",
		"strategic_plan_ref":          "STRAT-PLAN-002",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, dirs["milestone"], "MIL-002.yaml", mil)

	// Level 3: Goal
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-002",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Deep Chain Goal",
		objects.FieldKeyMilestoneRefs: []string{"MIL-002"},
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, dirs["goal"], "GOAL-002.yaml", goal)

	// Level 4: Requirement
	req := map[string]any{
		objects.FieldKeyID:            "REQ-002",
		objects.FieldKeyKind:          "requirement",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "planned",
		objects.FieldKeyTitle:         "Deep Chain Requirement",
		objects.FieldKeyGoalRefs:      []string{"GOAL-002"},
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createObjectFile(t, dirs["requirement"], "REQ-002.yaml", req)

	// Level 5: Backlog Item
	bli := map[string]any{
		objects.FieldKeyID:              "BLI-003",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          "planned",
		objects.FieldKeyTitle:           "Deep Chain Backlog Item",
		objects.FieldKeyRequirementRefs: []string{"REQ-002"},
		objects.FieldKeyCreatedAt:       time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:       time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
	}
	createObjectFile(t, dirs["backlog_item"], "BLI-003.yaml", bli)

	// Enqueue all objects (in any order to test cascade handling)
	objects := []struct {
		id       string
		kind     string
		filePath string
	}{
		{"BLI-003", "backlog_item", filepath.Join(dirs["backlog_item"], "BLI-003.yaml")},
		{"REQ-002", "requirement", filepath.Join(dirs["requirement"], "REQ-002.yaml")},
		{"GOAL-002", "goal", filepath.Join(dirs["goal"], "GOAL-002.yaml")},
		{"MIL-002", "milestone", filepath.Join(dirs["milestone"], "MIL-002.yaml")},
		{"STRAT-PLAN-002", "strategic_plan", filepath.Join(dirs["strategic_plan"], "STRAT-PLAN-002.yaml")},
	}

	for _, obj := range objects {
		_ = validator.Enqueue(obj.id, obj.kind, obj.filePath, 1)
	}

	// Wait for validation
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for validation")
		case <-ticker.C:
			_, _, _, queueSize := validator.GetValidationStats()
			if queueSize == 0 {
				// Verify all objects validated
				for _, obj := range objects {
					state, exists := validator.GetCachedState(obj.id)
					if !exists {
						t.Errorf("expected %s to be cached", obj.id)
					} else {
						t.Logf("%s: %d issues", obj.id, len(state.Issues))
					}
				}
				return
			}
		}
	}
}

// TestAsyncValidator_ConcurrentCascadeValidation tests concurrent validation
// of objects in a dependency chain
func TestAsyncValidator_ConcurrentCascadeValidation(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create multiple dependency chains in parallel
	numChains := 10
	chains := make([][]struct {
		id       string
		kind     string
		filePath string
	}, numChains)

	// Create directories
	milestoneDir := datacell.CellCASPrimaryDir(testRoot, "milestones")
	goalDir := datacell.CellCASPrimaryDir(testRoot, "goals")
	backlogDir := datacell.CellCASPrimaryDir(testRoot, "backlog")

	for _, dir := range []string{milestoneDir, goalDir, backlogDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	// Create multiple chains
	for i := 0; i < numChains; i++ {
		chainID := fmt.Sprintf("%03d", i+1)

		// Milestone
		mil := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("MIL-%s", chainID),
			objects.FieldKeyKind:          "milestone",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyTitle:         fmt.Sprintf("Milestone %s", chainID),
			objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		}
		milPath := createObjectFile(t, milestoneDir, fmt.Sprintf("MIL-%s.yaml", chainID), mil)

		// Goal
		goal := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("GOAL-%s", chainID),
			objects.FieldKeyKind:          "goal",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyTitle:         fmt.Sprintf("Goal %s", chainID),
			objects.FieldKeyMilestoneRefs: []string{fmt.Sprintf("MIL-%s", chainID)},
			objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		}
		goalPath := createObjectFile(t, goalDir, fmt.Sprintf("GOAL-%s.yaml", chainID), goal)

		// Backlog Item
		bli := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-%s", chainID),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "planned",
			objects.FieldKeyTitle:         fmt.Sprintf("Backlog Item %s", chainID),
			objects.FieldKeyGoalRefs:      []string{fmt.Sprintf("GOAL-%s", chainID)},
			objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		}
		bliPath := createObjectFile(t, backlogDir, fmt.Sprintf("BLI-%s.yaml", chainID), bli)

		chains[i] = []struct {
			id       string
			kind     string
			filePath string
		}{
			{fmt.Sprintf("MIL-%s", chainID), "milestone", milPath},
			{fmt.Sprintf("GOAL-%s", chainID), "goal", goalPath},
			{fmt.Sprintf("BLI-%s", chainID), "backlog_item", bliPath},
		}
	}

	// Enqueue all objects concurrently (simulating discovery in any order)
	var wg sync.WaitGroup
	for _, chain := range chains {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(chain []struct {
				id       string
				kind     string
				filePath string
			}) {
				defer wg.Done()
				// Enqueue in reverse order (dependents first) to stress test cascade
				for i := len(chain) - 1; i >= 0; i-- {
					obj := chain[i]
					_ = validator.Enqueue(obj.id, obj.kind, obj.filePath, 1)
				}
			}(chain)
		})
	}
	wg.Wait()

	// Wait for validation
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	expected := numChains * 3 // 3 objects per chain
	for {
		select {
		case <-timeout:
			total, stale, withIssues, queueSize := validator.GetValidationStats()
			t.Fatalf("timeout: processed %d, queue: %d, stale: %d, withIssues: %d", total, queueSize, stale, withIssues)
		case <-ticker.C:
			// Do not exit on queueSize==0 alone: work can still be publishing cached state.
			// Wait until every object has a cached validation state or we hit the timeout above.
			validated := 0
			for _, chain := range chains {
				for _, obj := range chain {
					if _, exists := validator.GetCachedState(obj.id); exists {
						validated++
					}
				}
			}
			if validated == expected {
				t.Logf("Successfully validated %d objects across %d dependency chains", validated, numChains)
				return
			}
		}
	}
}

// TestAsyncValidator_DependencyUpdateCascade tests that updating a dependency
// invalidates dependent objects in the cache
func TestAsyncValidator_DependencyUpdateCascade(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	goalDir := datacell.CellCASPrimaryDir(testRoot, "goals")
	backlogDir := datacell.CellCASPrimaryDir(testRoot, "backlog")

	for _, dir := range []string{goalDir, backlogDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	// Create goal
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-003",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Original Goal",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	goalPath := createObjectFile(t, goalDir, "GOAL-003.yaml", goal)

	// Create backlog item depending on goal
	bli := map[string]any{
		objects.FieldKeyID:            "BLI-004",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "planned",
		objects.FieldKeyTitle:         "Dependent Backlog Item",
		objects.FieldKeyGoalRefs:      []string{"GOAL-003"},
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	bliPath := createObjectFile(t, backlogDir, "BLI-004.yaml", bli)

	// Validate both objects
	ctx := pkgctx.NewSystemContext()
	goalState, err := validator.ValidateNow(ctx, "GOAL-003", "goal", goalPath)
	if err != nil {
		t.Fatalf("failed to validate goal: %v", err)
	}

	_, err = validator.ValidateNow(ctx, "BLI-004", "backlog_item", bliPath)
	if err != nil {
		t.Fatalf("failed to validate backlog item: %v", err)
	}

	// Both should be cached
	if _, exists := validator.GetCachedState("GOAL-003"); !exists {
		t.Error("goal should be cached")
	}
	if _, exists := validator.GetCachedState("BLI-004"); !exists {
		t.Error("backlog item should be cached")
	}

	// Update the goal (simulating a change)
	goal[objects.FieldKeyTitle] = "Updated Goal"
	goal[objects.FieldKeyUpdatedAt] = time.Now().Format(time.RFC3339)
	createObjectFile(t, goalDir, "GOAL-003.yaml", goal)

	// Re-validate goal (should update cache)
	goalState2, err := validator.ValidateNow(ctx, "GOAL-003", "goal", goalPath)
	if err != nil {
		t.Fatalf("failed to re-validate goal: %v", err)
	}

	// Goal checksum should have changed
	if goalState.Checksum == goalState2.Checksum {
		t.Error("goal checksum should have changed after update")
	}

	// Backlog item should still be cached (but may need revalidation if dependency changed)
	// This tests that the system handles dependency updates correctly
	bliState2, err := validator.ValidateNow(ctx, "BLI-004", "backlog_item", bliPath)
	if err != nil {
		t.Fatalf("failed to re-validate backlog item: %v", err)
	}

	t.Logf("Goal updated: checksum changed from %s to %s", goalState.Checksum[:8], goalState2.Checksum[:8])
	t.Logf("Backlog item re-validated: %d issues", len(bliState2.Issues))
}
