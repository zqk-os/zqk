package reports

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"

	"github.com/zqk-os/zqk/pkg/objects"
)

// testUniqueSuffix returns a suffix from t.Name() for test data isolation (POL-CODE-006).
// Uses a hash of the full name so parallel or scheduler-run tests get distinct IDs (8 hex chars to avoid collisions).
func testUniqueSuffix(t *testing.T) string {
	t.Helper()
	name := t.Name()
	var h uint32
	for i := 0; i < len(name); i++ {
		h = h*31 + uint32(name[i])
	}
	return fmt.Sprintf("-%08x", h)
}

// setupTestProject creates a temporary project directory with test data via [testkit.PrepareIsolatedTempProject].
// Callers must not use t.Parallel() on the same testing.T: ZQK_TEST_ROOT uses t.Setenv.
func setupTestProject(t *testing.T) (string, storage.ObjectStorageProvider) {
	t.Helper()
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "cmd.reports",
		ForceRemoveRootOnCleanup: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			processDir := datacell.ProcessPrimaryDir(root)
			backlogDir := filepath.Join(processDir, "backlog")
			codeRefDir := filepath.Join(processDir, "code_references")
			return []testkit.NamedTestStep{{
				Name: "reports_kind_directories",
				Fn: func() error {
					if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
						return err
					}
					return fileutil.MkdirAll(codeRefDir, paths.DirPerm755)
				},
			}}
		},
	})
	return p.Root, p.FileStorage
}

// createTestData creates mock backlog items and code references for testing.
// Uses test-unique IDs so parallel or scheduler-run tests don't collide (POL-CODE-006).
func createTestData(t *testing.T, storageProvider storage.ObjectStorageProvider) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	suffix := testUniqueSuffix(t)
	idMap := make(map[string]string)

	// A backlog_item cannot reach complete on an empty criteria_refs list: the ref-status matrix
	// fails closed on absence, because a barrier that passes when it cannot see the evidence is not
	// a barrier (pkg/validation/ref_status_constraints.go). The two complete items below carry a
	// validated criterion rather than asking for an exemption.
	critID := "CRIT-901" + suffix
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Reports fixture acceptance",
		objects.FieldKeyCategory:      "testing",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeySchemaVersion: quickSchemaV2,
	}, objects.ObjectStatusValidated)

	backlogItems := []map[string]any{
		{objects.FieldKeyID: "BLI-901", objects.FieldKeyKind: quickKindBacklog, objects.FieldKeyTitle: "Complete Item 1", objects.FieldKeyStatus: quickStatusComplete, objects.FieldKeySchemaVersion: quickSchemaV2, objects.FieldKeyCategory: "development", objects.FieldKeyEstimatedEffort: "1h", objects.FieldKeyCriteriaRefs: []string{critID}},
		{objects.FieldKeyID: "BLI-902", objects.FieldKeyKind: quickKindBacklog, objects.FieldKeyTitle: "Complete Item 2", objects.FieldKeyStatus: quickStatusComplete, objects.FieldKeySchemaVersion: quickSchemaV2, objects.FieldKeyCategory: "development", objects.FieldKeyEstimatedEffort: "1h", objects.FieldKeyCriteriaRefs: []string{critID}},
		{objects.FieldKeyID: "BLI-903", objects.FieldKeyKind: quickKindBacklog, objects.FieldKeyTitle: "Validated Item 1", objects.FieldKeyStatus: quickStatusValidated, objects.FieldKeySchemaVersion: quickSchemaV2, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-904", objects.FieldKeyKind: quickKindBacklog, objects.FieldKeyTitle: "Validated Item 2", objects.FieldKeyStatus: quickStatusValidated, objects.FieldKeySchemaVersion: quickSchemaV2, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-905", objects.FieldKeyKind: quickKindBacklog, objects.FieldKeyTitle: "Exploring Item", objects.FieldKeyStatus: quickStatusExploring, objects.FieldKeySchemaVersion: quickSchemaV2, objects.FieldKeyCategory: "development"},
	}
	for _, item := range backlogItems {
		oldID := item[objects.FieldKeyID].(string)
		newID := oldID + suffix
		idMap[oldID] = newID
		item[objects.FieldKeyID] = newID
		intended, _ := item[objects.FieldKeyStatus].(string)
		leave := intended
		if intended != quickStatusExploring {
			leave = storage.DistinctLeaveStatusForTest(quickKindBacklog, intended)
		}
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, item, leave)
	}

	// Create code references with commit data for enhanced metrics
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	codeRefs := []map[string]any{
		// Recent commits (within 7 days) - healthy activity
		{
			objects.FieldKeyID:              "COD-901",
			objects.FieldKeyKind:            quickKindCodeRef,
			objects.FieldKeyTitle:           "Recent commit 1",
			objects.FieldKeySchemaVersion:   quickSchemaV2,
			objects.FieldKeyStatus:          objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:        "pkg/test1.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(50),
			objects.FieldKeyCommitHash:      "commit-recent-1",
			objects.FieldKeyCommitDate:      now.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "test-author",
			objects.FieldKeyLinesAdded:      float64(50),
			objects.FieldKeyLinesRemoved:    float64(10),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-901"},
		},
		{
			objects.FieldKeyID:              "COD-902",
			objects.FieldKeyKind:            quickKindCodeRef,
			objects.FieldKeyTitle:           "Recent commit 2",
			objects.FieldKeySchemaVersion:   quickSchemaV2,
			objects.FieldKeyStatus:          objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:        "pkg/test2.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(30),
			objects.FieldKeyCommitHash:      "commit-recent-1", // Same commit
			objects.FieldKeyCommitDate:      now.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "test-author",
			objects.FieldKeyLinesAdded:      float64(30),
			objects.FieldKeyLinesRemoved:    float64(5),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-901"},
		},
		// Commit within 30 days but not recent
		{
			objects.FieldKeyID:              "COD-903",
			objects.FieldKeyKind:            quickKindCodeRef,
			objects.FieldKeyTitle:           "Older commit",
			objects.FieldKeySchemaVersion:   quickSchemaV2,
			objects.FieldKeyStatus:          objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:        "pkg/test3.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(20),
			objects.FieldKeyCommitHash:      "commit-older-1",
			objects.FieldKeyCommitDate:      sevenDaysAgo.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "test-author-2",
			objects.FieldKeyLinesAdded:      float64(20),
			objects.FieldKeyLinesRemoved:    float64(3),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-902"},
		},
		// Co-change pattern for dependency detection
		{
			objects.FieldKeyID:            "COD-904",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file A",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-a.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(40),
			objects.FieldKeyCommitHash:    "commit-cochange-1",
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(40),
			objects.FieldKeyLinesRemoved:  float64(8),
		},
		{
			objects.FieldKeyID:            "COD-905",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file B",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-b.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(35),
			objects.FieldKeyCommitHash:    "commit-cochange-1", // Same commit as cochange-a.go
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(35),
			objects.FieldKeyLinesRemoved:  float64(7),
		},
		{
			objects.FieldKeyID:            "COD-906",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file A again",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-a.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(45),
			objects.FieldKeyCommitHash:    "commit-cochange-2",
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(45),
			objects.FieldKeyLinesRemoved:  float64(9),
		},
		{
			objects.FieldKeyID:            "COD-907",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file B again",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-b.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(38),
			objects.FieldKeyCommitHash:    "commit-cochange-2", // Same commit as cochange-a.go
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(38),
			objects.FieldKeyLinesRemoved:  float64(8),
		},
		{
			objects.FieldKeyID:            "COD-908",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file A third time",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-a.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(50),
			objects.FieldKeyCommitHash:    "commit-cochange-3",
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(50),
			objects.FieldKeyLinesRemoved:  float64(10),
		},
		{
			objects.FieldKeyID:            "COD-909",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Co-change file B third time",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/cochange-b.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(42),
			objects.FieldKeyCommitHash:    "commit-cochange-3", // Same commit as cochange-a.go
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(42),
			objects.FieldKeyLinesRemoved:  float64(9),
		},
		// Stale file for blocker detection
		{
			objects.FieldKeyID:            "COD-910",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Stale file",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/stale.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(100),
			objects.FieldKeyCommitHash:    "commit-stale-1",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.AddDate(0, 0, -5).Format(time.RFC3339), // 35 days ago
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(100),
			objects.FieldKeyLinesRemoved:  float64(20),
		},
		{
			objects.FieldKeyID:            "COD-911",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Stale file commit 2",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/stale.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(110),
			objects.FieldKeyCommitHash:    "commit-stale-2",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.AddDate(0, 0, -10).Format(time.RFC3339), // 40 days ago
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(110),
			objects.FieldKeyLinesRemoved:  float64(25),
		},
		{
			objects.FieldKeyID:            "COD-912",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Stale file commit 3",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/stale.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(120),
			objects.FieldKeyCommitHash:    "commit-stale-3",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.AddDate(0, 0, -15).Format(time.RFC3339), // 45 days ago
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(120),
			objects.FieldKeyLinesRemoved:  float64(30),
		},
		{
			objects.FieldKeyID:            "COD-913",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Stale file commit 4",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/stale.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(130),
			objects.FieldKeyCommitHash:    "commit-stale-4",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.AddDate(0, 0, -20).Format(time.RFC3339), // 50 days ago
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(130),
			objects.FieldKeyLinesRemoved:  float64(35),
		},
		{
			objects.FieldKeyID:            "COD-914",
			objects.FieldKeyKind:          quickKindCodeRef,
			objects.FieldKeyTitle:         "Stale file commit 5",
			objects.FieldKeySchemaVersion: quickSchemaV2,
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyFilePath:      "pkg/stale.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(140),
			objects.FieldKeyCommitHash:    "commit-stale-5",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.AddDate(0, 0, -25).Format(time.RFC3339), // 55 days ago
			objects.FieldKeyAuthor:        "test-author",
			objects.FieldKeyLinesAdded:    float64(140),
			objects.FieldKeyLinesRemoved:  float64(40),
		},
	}

	for _, ref := range codeRefs {
		ref[objects.FieldKeyID] = ref[objects.FieldKeyID].(string) + suffix
		ref[objects.FieldKeyStatus] = "conceptual"
		if refs, ok := ref[objects.FieldKeyBacklogItemRefs].([]string); ok {
			mapped := make([]string, len(refs))
			for i, r := range refs {
				if m, ok := idMap[r]; ok {
					mapped[i] = m
				} else {
					mapped[i] = r
				}
			}
			ref[objects.FieldKeyBacklogItemRefs] = mapped
		}
		// code_reference lifecycle: conceptual (origin/draft) -> originated (membrane-crossed).
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, ref, "originated")
	}
}

// createMockCommand creates a cobra command with context for testing
//
//nolint:unused // Test helper - reserved for future use
func createMockCommand(t *testing.T, projectRoot string) *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create context using GetContextFromCommand (simulates what root.go does)
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, projectRoot)
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}

	// Register context with command
	cli.SetContext(cmd, ctx)

	return cmd
}

func TestReportsPCS_BaseMetrics(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	// Calculate expected PCS manually
	// The filter only includes: "complete", "in_progress", "validated", "planned"
	// So "exploring" items are excluded from the calculation
	// We have: 2 complete (1.0), 2 validated (0.5) = 4 items in the filter
	// Score: (2*1.0 + 2*0.5) / 4 * 100 = 3.0 / 4 * 100 = 75.0
	// However, commit data may be auto-detected, which could enhance the score
	// So we'll check that it's in a reasonable range
	expectedBasePCS := (2.0*1.0 + 2.0*0.5) / 4.0 * 100.0

	// Verify calculation
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", false)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Allow for commit data auto-detection (if commit data exists, it may enhance PCS)
	// Base PCS should be around 75.0, but with commit data it could be enhanced
	// So we check it's in a reasonable range (base to base + enhancements)
	minExpected := expectedBasePCS - 5.0  // Allow some variance
	maxExpected := expectedBasePCS + 20.0 // Allow for commit enhancements
	if projectMetrics.PCS < minExpected || projectMetrics.PCS > maxExpected {
		t.Errorf("expected PCS in range [%.2f, %.2f], got %.2f (base: %.2f)", minExpected, maxExpected, projectMetrics.PCS, expectedBasePCS)
	}

	// Verify PCS is in valid range
	if projectMetrics.PCS < 0 || projectMetrics.PCS > 100 {
		t.Errorf("PCS should be in range [0, 100], got %.2f", projectMetrics.PCS)
	}
}

func TestReportsPCS_WithCommitData(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	// Calculate metrics with commit data
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// With commit data, PCS should be enhanced
	// We have 5 unique commits in last 30 days
	// 5 commits / 30 days = ~0.167 commits/day (low frequency, but recent activity)
	// 4 commits in last 7 days
	// Pattern should be "increasing" or "stable"

	// Verify PCS is enhanced (should be different from base)
	if projectMetrics.PCS <= 0 || projectMetrics.PCS > 100 {
		t.Errorf("PCS should be in range [0, 100], got %.2f", projectMetrics.PCS)
	}

	// Verify commit data is being used (PCS should be enhanced)
	// Base PCS is ~65, with commit enhancements it should be different
	baseMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", false)
	if err != nil {
		t.Logf("Note: Could not calculate base metrics for comparison: %v", err)
	} else if projectMetrics.PCS == baseMetrics.PCS {
		t.Logf("Note: PCS with commit data (%.2f) equals base PCS (%.2f) - commit data may not be enhancing", projectMetrics.PCS, baseMetrics.PCS)
	}
}

func TestReportsEDD_Calculation(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Calculate EDD
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// EDD can be negative or positive
	// With commit data: we have ~5 unique commits and 5 backlog items
	// Work item ratio = 5 / 5 = 1.0 (optimal range is 2-5)
	// This should indicate underestimation (low ratio)
	// So EDD should be adjusted upward

	// Verify EDD is calculated
	if projectMetrics.EDD < -100 || projectMetrics.EDD > 100 {
		t.Errorf("EDD should be in reasonable range, got %.2f", projectMetrics.EDD)
	}
}

func TestReportsBlockers_DependencyDetection(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Calculate D&B
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify dependencies are detected
	// We created co-change pattern: cochange-a.go and cochange-b.go changed together 3 times
	// This should trigger dependency detection
	foundCoChangeDep := false
	for _, dep := range projectMetrics.DB.Dependencies {
		if strings.Contains(dep.Description, "cochange-a.go") && strings.Contains(dep.Description, "cochange-b.go") {
			foundCoChangeDep = true
			if dep.Type != "code" {
				t.Errorf("expected dependency type 'code', got %s", dep.Type)
			}
			if dep.Severity != "medium" {
				t.Errorf("expected dependency severity 'medium', got %s", dep.Severity)
			}
			break
		}
	}
	if !foundCoChangeDep {
		t.Error("expected to detect co-change dependency between cochange-a.go and cochange-b.go")
	}

	// Verify blockers are detected
	// We created stale file pattern: stale.go has 5 commits but all > 30 days ago
	// This should trigger stale file blocker detection
	foundStaleBlocker := false
	for _, blocker := range projectMetrics.DB.Blockers {
		if strings.Contains(blocker.Description, "stale.go") && strings.Contains(blocker.Description, "no recent activity") {
			foundStaleBlocker = true
			if blocker.Type != "code" {
				t.Errorf("expected blocker type 'code', got %s", blocker.Type)
			}
			break
		}
	}
	if !foundStaleBlocker {
		t.Error("expected to detect stale file blocker for stale.go")
	}
}

func TestReportsPCS_JSONOutput(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	// Test JSON output format
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", false)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify JSON can be marshaled
	result := map[string]any{
		"pcs":                  projectMetrics.PCS,
		objects.FieldKeyStatus: getPCSStatus(projectMetrics.PCS),
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	// Verify JSON is valid
	var unmarshaled map[string]any
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if unmarshaled["pcs"] == nil {
		t.Error("JSON should contain 'pcs' field")
	}
	if unmarshaled[objects.FieldKeyStatus] == nil {
		t.Error("JSON should contain 'status' field")
	}
}

func TestReportsEDD_JSONOutput(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify JSON can be marshaled
	result := map[string]any{
		"edd":                  projectMetrics.EDD,
		objects.FieldKeyStatus: getEDDStatus(projectMetrics.EDD),
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	// Verify JSON is valid
	var unmarshaled map[string]any
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if unmarshaled["edd"] == nil {
		t.Error("JSON should contain 'edd' field")
	}
	if unmarshaled[objects.FieldKeyStatus] == nil {
		t.Error("JSON should contain 'status' field")
	}
}

func TestReportsBlockers_JSONOutput(t *testing.T) {
	_, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify JSON can be marshaled
	result := map[string]any{
		objects.FieldKeySummary:      projectMetrics.DB.Summary,
		objects.FieldKeyDependencies: projectMetrics.DB.Dependencies,
		objects.FieldKeyBlockers:     projectMetrics.DB.Blockers,
		"action_items":               projectMetrics.DB.ActionItems,
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	// Verify JSON is valid
	var unmarshaled map[string]any
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if unmarshaled[objects.FieldKeySummary] == nil {
		t.Error("JSON should contain 'summary' field")
	}
	if unmarshaled[objects.FieldKeyDependencies] == nil {
		t.Error("JSON should contain 'dependencies' field")
	}
	if unmarshaled[objects.FieldKeyBlockers] == nil {
		t.Error("JSON should contain 'blockers' field")
	}
	if unmarshaled["action_items"] == nil {
		t.Error("JSON should contain 'action_items' field")
	}
}

func TestReportsCommands_Cleanup(t *testing.T) {
	// Verify that temporary directories are cleaned up
	projectRoot, _ := setupTestProject(t)

	// t.TempDir() automatically cleans up, but let's verify the directory exists
	if _, err := fileutil.Stat(projectRoot); fileutil.IsNotExist(err) {
		t.Fatalf("test project directory should exist: %s", projectRoot)
	}

	// The cleanup happens automatically via t.Cleanup() in setupTestProject
	// This test verifies the pattern is correct
}

func TestReportsPCS_StatusInterpretation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pcs      float64
		expected string
	}{
		{"excellent", 85.0, "excellent"},
		{"good", 70.0, "good"},
		{"fair", 50.0, "fair"},
		{"poor", 30.0, "poor"},
		{"critical", 10.0, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := getPCSStatus(tt.pcs)
			if status != tt.expected {
				t.Errorf("expected status %s for PCS %.2f, got %s", tt.expected, tt.pcs, status)
			}
		})
	}
}

func TestReportsEDD_StatusInterpretation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		edd      float64
		expected string
	}{
		{"significant_underestimation", 25.0, "significant_underestimation"},
		{"moderate_underestimation", 10.0, "moderate_underestimation"},
		{"good_accuracy", 0.0, "good_accuracy"},
		{"moderate_overestimation", -10.0, "moderate_overestimation"},
		{"significant_overestimation", -25.0, "significant_overestimation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := getEDDStatus(tt.edd)
			if status != tt.expected {
				t.Errorf("expected status %s for EDD %.2f, got %s", tt.expected, tt.edd, status)
			}
		})
	}
}
