package testjobgen

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	schedcore "github.com/lanceman/zqk/pkg/scheduler"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/testscan"
)

func TestJobGenerator_BuildTestArgs_SchedulerPackageUsesParallel1(t *testing.T) {
	tmpDir := t.TempDir()
	testRoot, err := testenvroot.Setup(tmpDir)
	if err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	storage, err := storagepkg.NewFileObjectStorage(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	testkit.RegisterTempProjectTeardown(t, testRoot, storage)

	secCtx := pkgctx.NewSystemSecurityContext()
	generator := NewJobGenerator(storage, secCtx)

	bundle := &testscan.TestBundle{
		ID:                "bundle-scheduler",
		PackagePath:       "cmd/zqk/scheduler",
		IsParallel:        true,
		EstimatedDuration: 30 * time.Second,
		Tests: []*testscan.TestFunction{
			{Name: "TestShowJobActivity_Success", Package: "scheduler"},
		},
	}

	logFilePath := filepath.Join(tmpDir, "sched.log")
	args := generator.buildTestArgs(bundle, tmpDir, logFilePath)
	if len(args) < 2 || args[0] != "-c" {
		t.Fatalf("expected shell -c, got %v", args)
	}
	cmd := args[1]
	if !contains(cmd, "-parallel 1") {
		t.Errorf("expected cmd/zqk/scheduler bundle to include -parallel 1, got: %s", cmd)
	}
}

func TestJobGenerator_BuildTestArgs(t *testing.T) {
	tmpDir := t.TempDir()
	testRoot, err := testenvroot.Setup(tmpDir)
	if err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	storage, err := storagepkg.NewFileObjectStorage(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	testkit.RegisterTempProjectTeardown(t, testRoot, storage)

	secCtx := pkgctx.NewSystemSecurityContext()
	generator := NewJobGenerator(storage, secCtx)

	// Create a test bundle
	bundle := &testscan.TestBundle{
		ID:                "bundle-1",
		PackagePath:       "pkg/example",
		IsParallel:        true,
		EstimatedDuration: 30 * time.Second,
		Tests: []*testscan.TestFunction{
			{Name: "TestA", Package: "example"},
			{Name: "TestB", Package: "example"},
		},
	}

	// Build args (create a log file path for testing)
	logFilePath := filepath.Join(tmpDir, "test.log")
	args := generator.buildTestArgs(bundle, tmpDir, logFilePath)

	// Verify args
	if len(args) == 0 {
		t.Fatal("Expected non-empty args")
	}

	// The function now returns shell command args: ["-c", "go test ... > logfile.log 2>&1"]
	// Check for "-c" flag (shell command flag)
	if args[0] != "-c" {
		t.Errorf("Expected first arg to be '-c' (shell command), got %s", args[0])
	}

	// Check that the command string contains the test command
	if len(args) < 2 {
		t.Fatal("Expected command string as second arg")
	}
	commandStr := args[1]

	// Check for package path in command string
	if !contains(commandStr, "./pkg/example") {
		t.Error("Expected to find package path in command string")
	}

	// Check for -run flag in command string
	if !contains(commandStr, "-run") {
		t.Error("Expected to find -run flag in command string")
	}

	// Verify the pattern contains test names
	if !contains(commandStr, "TestA") || !contains(commandStr, "TestB") {
		t.Errorf("Expected command to contain test names, got %s", commandStr)
	}

	// Check for log file redirection
	if !contains(commandStr, logFilePath) {
		t.Errorf("Expected command to redirect output to log file %s", logFilePath)
	}
}

func isolatedJobGenProject(t *testing.T) testkit.IsolatedTempProject {
	t.Helper()
	return testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
		Kind:            "testjobgen",
	})
}

func TestJobGenerator_GenerateJobs(t *testing.T) {
	proj := isolatedJobGenProject(t)
	tmpDir := proj.Root
	storage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()
	generator := NewJobGenerator(storage, secCtx)

	bundles := []*testscan.TestBundle{
		{
			ID:                "bundle-1",
			PackagePath:       "pkg/example",
			IsParallel:        true,
			EstimatedDuration: 30 * time.Second,
			Tests: []*testscan.TestFunction{
				{Name: "TestA", Package: "example"},
			},
		},
	}

	ctx := context.Background()
	jobIDs, err := generator.GenerateJobs(ctx, bundles, tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("GenerateJobs failed: %v", err)
	}

	if len(jobIDs) != 1 {
		t.Errorf("Expected 1 job ID, got %d", len(jobIDs))
	}

	// Verify job was created
	jobID := jobIDs[0]
	obj, err := storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read created job: %v", err)
	}

	// Verify job properties
	jobType, ok := obj[objects.FieldKeyJobType].(string)
	if !ok || jobType != "run_wrapper" {
		t.Errorf("Expected job_type to be 'run_wrapper', got %v", jobType)
	}

	category, ok := obj[objects.FieldKeyCategory].(string)
	if !ok || category != "testing" {
		t.Errorf("Expected category to be 'testing', got %v", category)
	}

	metaAny, ok := obj[objects.FieldKeyMetadata]
	if !ok {
		t.Fatalf("expected metadata map on test bundle job")
	}
	metaAny, ok = nildecode.DecodeNonNilPayload[any](metaAny)
	if !ok {
		t.Fatalf("expected metadata map on test bundle job")
	}
	meta, ok := metaAny.(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map on test bundle job")
	}
	fp, _ := meta[schedcore.KeyBundleCommandFingerprint].(string)
	if fp == "" {
		t.Errorf("expected %s in metadata, got %q", schedcore.KeyBundleCommandFingerprint, fp)
	}
	pri, _ := obj[objects.FieldKeyPriority].(string)
	if pri != schedcore.JobPriorityNormal {
		t.Errorf("bundle without process refs: priority want %q, got %q", schedcore.JobPriorityNormal, pri)
	}
	if jobID != "SCH-run-bundle-1" {
		t.Errorf("stable job id want SCH-run-bundle-1, got %q", jobID)
	}
}

func TestJobGenerator_GenerateJobs_RecreatesTerminalArchivedJob(t *testing.T) {
	proj := isolatedJobGenProject(t)
	tmpDir := proj.Root
	storage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()
	generator := NewJobGenerator(storage, secCtx)
	ctx := context.Background()

	bundles := []*testscan.TestBundle{
		{
			ID:                "bundle-arch",
			PackagePath:       "pkg/example",
			IsParallel:        true,
			EstimatedDuration: 30 * time.Second,
			Tests: []*testscan.TestFunction{
				{Name: "TestA", Package: "example"},
			},
		},
	}

	jobIDs, err := generator.GenerateJobs(ctx, bundles, tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("GenerateJobs: %v", err)
	}
	wantID := "SCH-run-bundle-arch"
	if len(jobIDs) != 1 || jobIDs[0] != wantID {
		t.Fatalf("want %q, got %v", wantID, jobIDs)
	}
	if err := storage.Update(ctx, secCtx, wantID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived}); err != nil {
		t.Fatalf("archive job: %v", err)
	}

	jobIDs2, err := generator.GenerateJobs(ctx, bundles, tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("GenerateJobs after archive: %v", err)
	}
	if len(jobIDs2) != 1 || jobIDs2[0] != wantID {
		t.Fatalf("recreate want stable %q, got %v", wantID, jobIDs2)
	}
	obj, err := storage.Read(ctx, secCtx, wantID)
	if err != nil {
		t.Fatalf("read recreated: %v", err)
	}
	if st, _ := obj[objects.FieldKeyStatus].(string); st != schedcore.StatusActive {
		t.Fatalf("recreated status want active, got %q", st)
	}
}

func TestJobGenerator_GenerateJobs_BundleWithCriteriaRefs_SetsPriorityHigh(t *testing.T) {
	proj := isolatedJobGenProject(t)
	tmpDir := proj.Root
	testRoot := proj.Root
	storage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()

	// Manually write the criteria file to disk to satisfy reference validation
	critDir := filepath.Join(testRoot, "docs", "process", "criteria")
	fileutil.EnsureDir(critDir)
	_ = fileutil.WriteSecureFile(filepath.Join(critDir, "CRIT-test-1.yaml"), []byte("name: Test Criteria\n"))

	testDir := filepath.Join(tmpDir, "docs", "process", "tests")
	_ = fileutil.EnsureDir(testDir)
	_ = fileutil.WriteSecureFile(filepath.Join(testDir, "TEST-test-1.yaml"), []byte("name: Test Case\n"))

	generator := NewJobGenerator(storage, secCtx)

	bundles := []*testscan.TestBundle{
		{
			ID:                "bundle-crit",
			PackagePath:       "pkg/example",
			IsParallel:        true,
			EstimatedDuration: 30 * time.Second,
			CriteriaRefs:      []string{"CRIT-test-1"},
			TestCaseRefs:      []string{"TEST-test-1"},
			Tests: []*testscan.TestFunction{
				{Name: "TestA", Package: "example"},
			},
		},
	}

	ctx := context.Background()

	jobIDs, err := generator.GenerateJobs(ctx, bundles, tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("GenerateJobs failed: %v", err)
	}
	if len(jobIDs) != 1 {
		t.Fatalf("Expected 1 job ID, got %d", len(jobIDs))
	}
	obj, err := storage.Read(ctx, secCtx, jobIDs[0])
	if err != nil {
		t.Fatalf("read job: %v", err)
	}
	pri, _ := obj[objects.FieldKeyPriority].(string)
	if pri != schedcore.JobPriorityHigh {
		t.Errorf("priority want %q, got %q", schedcore.JobPriorityHigh, pri)
	}
}

func TestJobGenerator_GenerateJobs_updatesBundleCommandFingerprintOnReschedule(t *testing.T) {
	proj := isolatedJobGenProject(t)
	tmpDir := proj.Root
	storage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()
	generator := NewJobGenerator(storage, secCtx)
	ctx := context.Background()

	base := func(testName string) []*testscan.TestBundle {
		return []*testscan.TestBundle{
			{
				ID:                "bundle-fp-check",
				PackagePath:       "pkg/example",
				IsParallel:        true,
				EstimatedDuration: 30 * time.Second,
				Tests: []*testscan.TestFunction{
					{Name: testName, Package: "example"},
				},
			},
		}
	}

	jobIDs1, err := generator.GenerateJobs(ctx, base("TestA"), tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("first GenerateJobs: %v", err)
	}
	if len(jobIDs1) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobIDs1))
	}
	wantID := "SCH-run-bundle-fp-check"
	if jobIDs1[0] != wantID {
		t.Fatalf("stable job id want %q, got %q", wantID, jobIDs1[0])
	}
	obj1, err := storage.Read(ctx, secCtx, jobIDs1[0])
	if err != nil {
		t.Fatalf("read job: %v", err)
	}
	meta1, _ := obj1[objects.FieldKeyMetadata].(map[string]any)
	fp1, _ := meta1[schedcore.KeyBundleCommandFingerprint].(string)
	if fp1 == "" {
		t.Fatalf("expected fingerprint after first schedule")
	}

	jobIDs2, err := generator.GenerateJobs(ctx, base("TestB"), tmpDir, tmpDir, 4)
	if err != nil {
		t.Fatalf("second GenerateJobs: %v", err)
	}
	if len(jobIDs2) != 1 || jobIDs2[0] != wantID {
		t.Fatalf("reschedule must reuse stable id %q, got %v", wantID, jobIDs2)
	}
	obj2, err := storage.Read(ctx, secCtx, jobIDs2[0])
	if err != nil {
		t.Fatalf("read job 2: %v", err)
	}
	meta2, _ := obj2[objects.FieldKeyMetadata].(map[string]any)
	fp2, _ := meta2[schedcore.KeyBundleCommandFingerprint].(string)
	if fp2 == "" {
		t.Fatalf("expected fingerprint after reschedule")
	}
	if fp1 == fp2 {
		t.Fatalf("fingerprint should change when -run test set changes, got %q twice", fp1)
	}
}

func TestScanAndSchedule(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	proj := isolatedJobGenProject(t)
	projectRoot := proj.Root
	storage := proj.FileStorage

	// Create a test package structure
	testPkgDir := filepath.Join(projectRoot, "pkg", "example")
	if err := fileutil.MkdirAll(testPkgDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test package directory: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(testPkgDir, "example_test.go")
	testContent := `package example

import "testing"

func TestExample(t *testing.T) {
	t.Parallel()
}
`
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Run ScanAndSchedule
	jobIDs, err := ScanAndSchedule(ctx, projectRoot, storage, secCtx, 10, 4)
	if err != nil {
		t.Fatalf("ScanAndSchedule failed: %v", err)
	}

	if len(jobIDs) == 0 {
		t.Error("Expected at least one job ID")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
