package metrics

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestCalculateProjectMetrics_BaseMetrics(t *testing.T) {
	// Do not run in parallel: uses ZQK_TEST_ROOT and temp dir; write-behind can outlive the test.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	ensureObjectSpecsForAIMetricsTest(t, tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, "backlog")
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Create test backlog items with different statuses
	// Use statuses that don't require additional fields
	testItems := []map[string]any{
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: objects.ObjectStatusComplete, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyEstimatedEffort: "1h"},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: objects.ObjectStatusComplete, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-003", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: objects.ObjectStatusValidated, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-004", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 4", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range testItems {
		if err := storageProvider.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("failed to create test item %s: %v", item[objects.FieldKeyID], err)
		}
	}

	// Calculate metrics without commit data
	metrics, err := CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", false)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify PCS is calculated (should be > 0 since we have completed items)
	if metrics.PCS <= 0 {
		t.Errorf("expected PCS > 0, got %f", metrics.PCS)
	}

	// Verify PCS is within valid range
	if metrics.PCS < 0 || metrics.PCS > 100 {
		t.Errorf("expected PCS in range [0, 100], got %f", metrics.PCS)
	}

	// Verify DB structure is initialized
	if metrics.DB == nil {
		t.Error("expected DB to be initialized")
	}
}

func TestCalculateProjectMetrics_WithCommitData(t *testing.T) {
	// Do not run in parallel: uses ZQK_TEST_ROOT and temp dir; write-behind can outlive the test.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	ensureObjectSpecsForAIMetricsTest(t, tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, "backlog")
	codeRefDir := filepath.Join(processDir, "code_references")
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}
	if err := fileutil.MkdirAll(codeRefDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create code_references dir: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Create test backlog items (sync when using ForTest storage)
	testItems := []map[string]any{
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: objects.ObjectStatusComplete, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyEstimatedEffort: "1h"},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: objects.ObjectStatusValidated, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range testItems {
		if err := storageProvider.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("failed to create test item %s: %v", item[objects.FieldKeyID], err)
		}
	}

	// Create code references with commit data
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	codeRefs := []map[string]any{
		{
			objects.FieldKeyID:              "COD-001",
			objects.FieldKeyKind:            "code_reference",
			objects.FieldKeyTitle:           "Code ref 1",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:        "pkg/test1.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(60),
			objects.FieldKeyCommitHash:      "abc123def456",
			objects.FieldKeyCommitDate:      now.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "Test Author",
			objects.FieldKeyLinesAdded:      float64(50),
			objects.FieldKeyLinesRemoved:    float64(10),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-001"},
		},
		{
			objects.FieldKeyID:              "COD-002",
			objects.FieldKeyKind:            "code_reference",
			objects.FieldKeyTitle:           "Code ref 2",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:        "pkg/test2.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(35),
			objects.FieldKeyCommitHash:      "abc123def456", // Same commit
			objects.FieldKeyCommitDate:      now.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "Test Author",
			objects.FieldKeyLinesAdded:      float64(30),
			objects.FieldKeyLinesRemoved:    float64(5),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-001"},
		},
		{
			objects.FieldKeyID:              "COD-003",
			objects.FieldKeyKind:            "code_reference",
			objects.FieldKeyTitle:           "Code ref 3",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:        "pkg/test3.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(23),
			objects.FieldKeyCommitHash:      "def456ghi789",
			objects.FieldKeyCommitDate:      thirtyDaysAgo.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "Test Author",
			objects.FieldKeyLinesAdded:      float64(20),
			objects.FieldKeyLinesRemoved:    float64(3),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-002"},
		},
		{
			objects.FieldKeyID:              "COD-004",
			objects.FieldKeyKind:            "code_reference",
			objects.FieldKeyTitle:           "Code ref 4",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:        "pkg/test4.go",
			objects.FieldKeyLineStart:       float64(1),
			objects.FieldKeyLineEnd:         float64(48),
			objects.FieldKeyCommitHash:      "ghi789jkl012",
			objects.FieldKeyCommitDate:      sevenDaysAgo.Format(time.RFC3339),
			objects.FieldKeyAuthor:          "Test Author 2",
			objects.FieldKeyLinesAdded:      float64(40),
			objects.FieldKeyLinesRemoved:    float64(8),
			objects.FieldKeyBacklogItemRefs: []string{"BLI-002"},
		},
	}

	for _, ref := range codeRefs {
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, ref, objects.ObjectStatusOriginated)
	}

	// Calculate metrics with commit data
	metrics, err := CalculateProjectMetrics(ctx, storageProvider, secCtx, "test-project", true)
	if err != nil {
		t.Fatalf("failed to calculate metrics: %v", err)
	}

	// Verify PCS is calculated and enhanced
	if metrics.PCS <= 0 {
		t.Errorf("expected PCS > 0, got %f", metrics.PCS)
	}

	if metrics.PCS < 0 || metrics.PCS > 100 {
		t.Errorf("expected PCS in range [0, 100], got %f", metrics.PCS)
	}

	// Verify EDD is calculated
	// EDD can be negative (underestimation) or positive (overestimation)
	if metrics.EDD < -100 || metrics.EDD > 100 {
		t.Errorf("expected EDD in reasonable range, got %f", metrics.EDD)
	}

	// Verify DB structure is initialized
	if metrics.DB == nil {
		t.Error("expected DB to be initialized")
	}
}

func TestEnhancePCSWithCommits(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tests := []struct {
		name          string
		basePCS       float64
		commitMetrics *CommitMetrics
		expectedRange [2]float64 // [min, max] expected range
	}{
		{
			name:    "healthy commit frequency",
			basePCS: 70.0,
			commitMetrics: &CommitMetrics{
				CommitFrequency: 1.0, // 1 commit per day (optimal)
				CommitPattern:   "stable",
			},
			expectedRange: [2]float64{75.0, 78.0}, // +5 for frequency, +0-3 for pattern
		},
		{
			name:    "very low commit frequency",
			basePCS: 70.0,
			commitMetrics: &CommitMetrics{
				CommitFrequency: 0.05, // Very low
				CommitPattern:   "stable",
			},
			expectedRange: [2]float64{60.0, 65.0}, // -10 for very low frequency
		},
		{
			name:    "increasing commit pattern",
			basePCS: 70.0,
			commitMetrics: &CommitMetrics{
				CommitFrequency: 1.0,
				CommitPattern:   "increasing",
			},
			expectedRange: [2]float64{78.0, 78.0}, // +5 for frequency, +3 for increasing
		},
		{
			name:    "decreasing commit pattern",
			basePCS: 70.0,
			commitMetrics: &CommitMetrics{
				CommitFrequency: 1.0,
				CommitPattern:   "decreasing",
			},
			expectedRange: [2]float64{70.0, 70.0}, // +5 for frequency, -5 for decreasing = 70
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhancePCSWithCommits(tt.basePCS, tt.commitMetrics)
			if result < tt.expectedRange[0] || result > tt.expectedRange[1] {
				t.Errorf("expected PCS in range [%f, %f], got %f", tt.expectedRange[0], tt.expectedRange[1], result)
			}
			// Verify it's clamped to 0-100
			if result < 0 || result > 100 {
				t.Errorf("expected PCS in range [0, 100], got %f", result)
			}
		})
	}
}

func TestEnhanceEDDWithCommits(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tests := []struct {
		name          string
		baseEDD       float64
		commitMetrics *CommitMetrics
		expectedRange [2]float64
	}{
		{
			name:    "optimal commit-to-work-item ratio",
			baseEDD: 10.0,
			commitMetrics: &CommitMetrics{
				WorkItemRatio: 3.0, // Optimal: 2-5 commits per work item
			},
			expectedRange: [2]float64{8.0, 8.0}, // -2 for optimal ratio
		},
		{
			name:    "low commit-to-work-item ratio",
			baseEDD: 10.0,
			commitMetrics: &CommitMetrics{
				WorkItemRatio: 0.5, // Low ratio (under-scoped)
			},
			expectedRange: [2]float64{20.0, 20.0}, // +10 for low ratio (indicates underestimation)
		},
		{
			name:    "very high commit-to-work-item ratio",
			baseEDD: 10.0,
			commitMetrics: &CommitMetrics{
				WorkItemRatio: 15.0, // Very high ratio (over-scoped)
			},
			expectedRange: [2]float64{-5.0, -5.0}, // -15 for very high ratio (indicates overestimation)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhanceEDDWithCommits(tt.baseEDD, tt.commitMetrics)
			if result < tt.expectedRange[0] || result > tt.expectedRange[1] {
				t.Errorf("expected EDD in range [%f, %f], got %f", tt.expectedRange[0], tt.expectedRange[1], result)
			}
		})
	}
}

func TestCalculateCommitMetrics(t *testing.T) {
	// Do not run in parallel: uses ZQK_TEST_ROOT and temp dir; write-behind can outlive the test.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	ensureObjectSpecsForAIMetricsTest(t, tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	codeRefDir := filepath.Join(processDir, "code_references")
	backlogDir := filepath.Join(processDir, "backlog")
	if err := fileutil.MkdirAll(codeRefDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create code_references dir: %v", err)
	}
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// A backlog_item cannot reach complete on an empty criteria_refs list: the ref-status matrix
	// fails closed on absence, on the grounds that a barrier which passes when it cannot see the
	// evidence is not a barrier (pkg/validation/ref_status_constraints.go). So BLI-001 carries a
	// validated criterion rather than asking for an exemption.
	crit := map[string]any{
		objects.FieldKeyID:            "CRIT-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Item 1 accepted",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, crit, objects.ObjectStatusValidated)

	// Create backlog items for ratio calculation (sync create when using ForTest storage)
	backlogItems := []map[string]any{
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: objects.ObjectStatusComplete, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyEstimatedEffort: "1h", objects.FieldKeyCriteriaRefs: []string{"CRIT-001"}},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: objects.ObjectStatusValidated, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	for _, item := range backlogItems {
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, item, objects.GetString(item, objects.FieldKeyStatus))
	}

	// Create code references with different commit hashes and dates
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	codeRefs := []map[string]any{
		{
			objects.FieldKeyID:            "COD-001",
			objects.FieldKeyKind:          "code_reference",
			objects.FieldKeyTitle:         "Code ref 1",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:      "pkg/test1.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(60),
			objects.FieldKeyCommitHash:    "commit1",
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "Author 1",
			objects.FieldKeyLinesAdded:    float64(50),
			objects.FieldKeyLinesRemoved:  float64(10),
		},
		{
			objects.FieldKeyID:            "COD-002",
			objects.FieldKeyKind:          "code_reference",
			objects.FieldKeyTitle:         "Code ref 2",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:      "pkg/test2.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(35),
			objects.FieldKeyCommitHash:    "commit1", // Same commit
			objects.FieldKeyCommitDate:    now.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "Author 1",
			objects.FieldKeyLinesAdded:    float64(30),
			objects.FieldKeyLinesRemoved:  float64(5),
		},
		{
			objects.FieldKeyID:            "COD-003",
			objects.FieldKeyKind:          "code_reference",
			objects.FieldKeyTitle:         "Code ref 3",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:      "pkg/test3.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(23),
			objects.FieldKeyCommitHash:    "commit2",
			objects.FieldKeyCommitDate:    thirtyDaysAgo.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "Author 2",
			objects.FieldKeyLinesAdded:    float64(20),
			objects.FieldKeyLinesRemoved:  float64(3),
		},
		{
			objects.FieldKeyID:            "COD-004",
			objects.FieldKeyKind:          "code_reference",
			objects.FieldKeyTitle:         "Code ref 4",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeyFilePath:      "pkg/test4.go",
			objects.FieldKeyLineStart:     float64(1),
			objects.FieldKeyLineEnd:       float64(48),
			objects.FieldKeyCommitHash:    "commit3",
			objects.FieldKeyCommitDate:    sevenDaysAgo.Format(time.RFC3339),
			objects.FieldKeyAuthor:        "Author 1",
			objects.FieldKeyLinesAdded:    float64(40),
			objects.FieldKeyLinesRemoved:  float64(8),
		},
	}

	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	for _, ref := range codeRefs {
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, ref, objects.ObjectStatusOriginated)
	}

	// Calculate commit metrics
	commitMetrics, err := calculateCommitMetrics(ctx, storageProvider, secCtx, "test-project")
	if err != nil {
		t.Fatalf("failed to calculate commit metrics: %v", err)
	}

	// Verify unique commits are counted (3 unique commits: commit1, commit2, commit3)
	if commitMetrics.TotalCommits != 3 {
		t.Errorf("expected 3 unique commits, got %d", commitMetrics.TotalCommits)
	}

	// Verify commits in last 30 days (commit1 and commit3 are within 30 days)
	if commitMetrics.CommitsLast30Days != 2 {
		t.Errorf("expected 2 commits in last 30 days, got %d", commitMetrics.CommitsLast30Days)
	}

	// Verify commits in last 7 days (only commit1 is within 7 days)
	if commitMetrics.CommitsLast7Days != 1 {
		t.Errorf("expected 1 commit in last 7 days, got %d", commitMetrics.CommitsLast7Days)
	}

	// Verify commit frequency calculation
	expectedFrequency := float64(commitMetrics.CommitsLast30Days) / 30.0
	if commitMetrics.CommitFrequency != expectedFrequency {
		t.Errorf("expected commit frequency %f, got %f", expectedFrequency, commitMetrics.CommitFrequency)
	}

	// Verify authors are tracked
	if len(commitMetrics.Authors) != 2 {
		t.Errorf("expected 2 unique authors, got %d", len(commitMetrics.Authors))
	}

	// Verify lines changed are aggregated
	expectedLines := 50 + 10 + 30 + 5 + 20 + 3 + 40 + 8 // All lines_added + lines_removed
	if commitMetrics.LinesChanged != expectedLines {
		t.Errorf("expected %d lines changed, got %d", expectedLines, commitMetrics.LinesChanged)
	}

	// Verify work item ratio (3 commits / 2 work items = 1.5)
	expectedRatio := 3.0 / 2.0
	if commitMetrics.WorkItemRatio != expectedRatio {
		t.Errorf("expected work item ratio %f, got %f", expectedRatio, commitMetrics.WorkItemRatio)
	}
}

func TestGenerateDBSummary(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tests := []struct {
		name     string
		db       *DependenciesBlockers
		expected string
	}{
		{
			name: "empty dependencies and blockers",
			db: &DependenciesBlockers{
				Dependencies: []DependencyItem{},
				Blockers:     []BlockerItem{},
			},
			expected: "No dependencies or blockers identified.",
		},
		{
			name: "critical blockers",
			db: &DependenciesBlockers{
				Dependencies: []DependencyItem{},
				Blockers: []BlockerItem{
					{ID: "B1", Severity: "critical", Description: "Critical blocker"},
					{ID: "B2", Severity: "high", Description: "High blocker"},
				},
			},
			expected: "1 critical blocker(s) identified. 1 high-priority blocker(s) identified. ",
		},
		{
			name: "dependencies and blockers",
			db: &DependenciesBlockers{
				Dependencies: []DependencyItem{
					{ID: "D1", Severity: "critical", Description: "Critical dependency"},
					{ID: "D2", Severity: "medium", Description: "Medium dependency"},
				},
				Blockers: []BlockerItem{
					{ID: "B1", Severity: "high", Description: "High blocker"},
				},
			},
			expected: "1 high-priority blocker(s) identified. 1 critical dependency(ies) identified. 1 additional dependency(ies) present. ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDBSummary(tt.db)
			if result != tt.expected {
				t.Errorf("expected summary %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectCommitBasedDependencies(t *testing.T) {
	// Do not run in parallel: uses ZQK_TEST_ROOT and temp dir; write-behind can outlive the test.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	ensureObjectSpecsForAIMetricsTest(t, tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	codeRefDir := filepath.Join(processDir, "code_references")
	if err := fileutil.MkdirAll(codeRefDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create code_references dir: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	now := time.Now()

	// Create code references that simulate files changed together (co-change pattern)
	// Files pkg/a.go and pkg/b.go are changed together in 3 commits (dependency)
	// File pkg/c.go is changed alone in 5 commits but no recent activity (blocker)
	codeRefs := []map[string]any{
		// Co-change pattern: a.go and b.go together
		{objects.FieldKeyID: "COD-001", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 1", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/a.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(10), objects.FieldKeyCommitHash: "commit1", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(10)},
		{objects.FieldKeyID: "COD-002", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 2", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/b.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(20), objects.FieldKeyCommitHash: "commit1", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(20)},
		{objects.FieldKeyID: "COD-003", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 3", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/a.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(15), objects.FieldKeyCommitHash: "commit2", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(15)},
		{objects.FieldKeyID: "COD-004", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 4", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/b.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(25), objects.FieldKeyCommitHash: "commit2", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(25)},
		{objects.FieldKeyID: "COD-005", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 5", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/a.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(12), objects.FieldKeyCommitHash: "commit3", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(12)},
		{objects.FieldKeyID: "COD-006", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 6", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/b.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(18), objects.FieldKeyCommitHash: "commit3", objects.FieldKeyCommitDate: now.Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(18)},
		// Stale file pattern: c.go has many commits but old
		{objects.FieldKeyID: "COD-007", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 7", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/c.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(30), objects.FieldKeyCommitHash: "commit4", objects.FieldKeyCommitDate: now.AddDate(0, 0, -60).Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(30)},
		{objects.FieldKeyID: "COD-008", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 8", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/c.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(25), objects.FieldKeyCommitHash: "commit5", objects.FieldKeyCommitDate: now.AddDate(0, 0, -50).Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(25)},
		{objects.FieldKeyID: "COD-009", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 9", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/c.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(20), objects.FieldKeyCommitHash: "commit6", objects.FieldKeyCommitDate: now.AddDate(0, 0, -45).Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(20)},
		{objects.FieldKeyID: "COD-010", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 10", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/c.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(15), objects.FieldKeyCommitHash: "commit7", objects.FieldKeyCommitDate: now.AddDate(0, 0, -40).Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(15)},
		{objects.FieldKeyID: "COD-011", objects.FieldKeyKind: objects.KindCodeReference, objects.FieldKeyTitle: "Ref 11", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusConceptual, objects.FieldKeyFilePath: "pkg/c.go", objects.FieldKeyLineStart: float64(1), objects.FieldKeyLineEnd: float64(10), objects.FieldKeyCommitHash: "commit8", objects.FieldKeyCommitDate: now.AddDate(0, 0, -35).Format(time.RFC3339), objects.FieldKeyLinesAdded: float64(10)},
	}

	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	for _, ref := range codeRefs {
		storage.CreateCASVisible(t, storageProvider, ctx, secCtx, ref, objects.ObjectStatusOriginated)
	}

	// Calculate commit metrics
	commitMetrics, err := calculateCommitMetrics(ctx, storageProvider, secCtx, "test-project")
	if err != nil {
		t.Fatalf("failed to calculate commit metrics: %v", err)
	}

	// Test dependency detection
	deps, blockers := detectCommitBasedDependencies(ctx, storageProvider, secCtx, commitMetrics)

	// Should detect co-change dependency between a.go and b.go
	foundCoChangeDep := false
	for _, dep := range deps {
		if strings.Contains(dep.Description, "pkg/a.go") && strings.Contains(dep.Description, "pkg/b.go") {
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
		t.Error("expected to detect co-change dependency between pkg/a.go and pkg/b.go")
	}

	// Should detect stale file blocker for c.go
	foundStaleBlocker := false
	for _, blocker := range blockers {
		if strings.Contains(blocker.Description, "pkg/c.go") && strings.Contains(blocker.Description, "no recent activity") {
			foundStaleBlocker = true
			if blocker.Type != "code" {
				t.Errorf("expected blocker type 'code', got %s", blocker.Type)
			}
			break
		}
	}
	if !foundStaleBlocker {
		t.Error("expected to detect stale file blocker for pkg/c.go")
	}
}

// tdd refresh
