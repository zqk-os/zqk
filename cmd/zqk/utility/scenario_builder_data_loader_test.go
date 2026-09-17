package utility

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestHandleLoadDataFile_ParsesYAMLArray tests that handleLoadDataFile correctly parses YAML array format
func TestHandleLoadDataFile_ParsesYAMLArray(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test-data.yaml")

	// Create test YAML file with array format (starting with -)
	yamlContent := `- kind: account
  title: System Account
  username: system
  status: active
- kind: workstream
  id: WS-001
  title: Test Workstream
  status: active
- kind: goal
  id: GOAL-001
  title: Test Goal
  status: active
`

	if err := fileutil.WriteFile(dataFile, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test data file: %v", err)
	}

	// Create required directory structure for storage factory
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create coordinator and scenario builder
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create isolated storage provider for testing (ensures storage shutdown before tmp dir cleanup).
	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	flags := &ScenarioBuilderFlags{
		DataFile: dataFile,
		Force:    false,
	}

	// Load data file
	if err := handleLoadDataFile(nil, builder, flags); err != nil {
		t.Fatalf("handleLoadDataFile failed: %v", err)
	}

	// Verify objects were loaded
	if len(builder.dataFileObjects) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(builder.dataFileObjects))
	}

	// Verify all objects are present (order may vary due to dependency sorting)
	foundAccount := false
	foundWorkstream := false
	foundGoal := false
	for _, obj := range builder.dataFileObjects {
		if kind, ok := obj[objects.FieldKeyKind].(string); ok {
			switch kind {
			case "account":
				if username, ok := obj[objects.FieldKeyUsername].(string); ok && username == "system" {
					foundAccount = true
				}
			case "workstream":
				if id, ok := obj[objects.FieldKeyID].(string); ok && id == "WS-001" {
					foundWorkstream = true
				}
			case "goal":
				if id, ok := obj[objects.FieldKeyID].(string); ok && id == "GOAL-001" {
					foundGoal = true
				}
			}
		}
	}

	if !foundAccount {
		t.Error("Expected to find account object with username 'system'")
	}
	if !foundWorkstream {
		t.Error("Expected to find workstream object with id 'WS-001'")
	}
	if !foundGoal {
		t.Error("Expected to find goal object with id 'GOAL-001'")
	}
}

// TestHandleLoadDataFile_YAMLListFormat tests that YAML list format (starting with -) is parsed correctly
// This matches the actual scenario-data.yaml format
func TestHandleLoadDataFile_YAMLListFormat(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test-data.yaml")

	// Create YAML file in list format (starting with -) like scenario-data.yaml
	yamlContent := `- kind: account
  title: System Account
  username: system
  status: active
- kind: account
  title: Project Owner
  username: project-owner
  status: active
- kind: workstream
  id: WS-001
  title: Main Development Stream
  status: active
`

	if err := fileutil.WriteFile(dataFile, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test data file: %v", err)
	}

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	flags := &ScenarioBuilderFlags{
		DataFile: dataFile,
		Force:    false,
	}

	// Load data file
	if err := handleLoadDataFile(nil, builder, flags); err != nil {
		t.Fatalf("handleLoadDataFile failed: %v", err)
	}

	// Verify objects were loaded
	if len(builder.dataFileObjects) != 3 {
		t.Errorf("Expected 3 objects from YAML list format, got %d", len(builder.dataFileObjects))
	}

	// Verify all objects have correct structure
	for i, obj := range builder.dataFileObjects {
		if obj[objects.FieldKeyKind] == nil {
			t.Errorf("Object %d missing 'kind' field", i)
		}
		if obj[objects.FieldKeyTitle] == nil {
			t.Errorf("Object %d missing 'title' field", i)
		}
	}
}

// TestBuildFromDataFile_IsCalledWhenDataFileObjectsSet tests that BuildFromDataFile is called when dataFileObjects is set
func TestBuildFromDataFile_IsCalledWhenDataFileObjectsSet(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create required directory structure for storage factory
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create coordinator and scenario builder
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.config.TargetDir = tmpDir

	// Set dataFileObjects to simulate loaded data
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyTitle:    "Test Account",
			objects.FieldKeyID:       "ACC-TEST",
			objects.FieldKeyUsername: "test",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
	}

	// Create test logger to capture events
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	// Call Build - should call BuildFromDataFile
	ctx := pkgctx.NewSystemContext()
	buildErr := builder.Build(ctx)

	// Check if BuildFromDataFile was called (should emit "Building scenario from data file")
	infoLogs := testLogger.getInfoLogs()
	found := false
	for _, log := range infoLogs {
		if log.message == "Building scenario from data file" {
			found = true
			break
		}
	}

	if !found {
		t.Error("BuildFromDataFile was not called - 'Building scenario from data file' event not found")
		t.Logf("Info logs received: %v", infoLogs)
	}

	// Build should not return error for this simple case (even if object creation fails)
	// We're just checking that BuildFromDataFile was called
	if buildErr != nil {
		t.Logf("Build returned error (may be expected): %v", buildErr)
	}
}

// TestBuildFromDataFile_EmptyDataFileObjects tests that BuildFromDataFile returns error when no objects
func TestBuildFromDataFile_EmptyDataFileObjects(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create required directory structure for storage factory
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.config.TargetDir = tmpDir

	// Don't set dataFileObjects (should be empty)

	ctx := pkgctx.NewSystemContext()
	buildErr := builder.BuildFromDataFile(ctx)

	if buildErr == nil {
		t.Error("Expected error when dataFileObjects is empty, but got nil")
	} else if buildErr.Error() != "no objects loaded from data file" {
		t.Errorf("Expected error 'no objects loaded from data file', got: %v", buildErr)
	}
}

// TestBuild_DataFileObjectsTakesPrecedence tests that dataFileObjects takes precedence over config.Kinds
func TestBuild_DataFileObjectsTakesPrecedence(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create required directory structure for storage factory
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.config.TargetDir = tmpDir
	builder.config.Kinds = []string{scenarioBuilderKindBacklogItem} // Set kinds (should be ignored if dataFileObjects is set)

	// Set dataFileObjects - should take precedence
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyTitle:    "Test Account",
			objects.FieldKeyID:       "ACC-TEST",
			objects.FieldKeyUsername: "test",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
	}

	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	ctx := pkgctx.NewSystemContext()
	_ = builder.Build(ctx) // May return error, but we're checking which path was taken

	// Verify BuildFromDataFile was called (not the regular Build path)
	infoLogs := testLogger.getInfoLogs()
	foundDataFile := false
	foundRegular := false
	for _, log := range infoLogs {
		if log.message == "Building scenario from data file" {
			foundDataFile = true
		}
		if log.message == "Building test scenario" {
			foundRegular = true
		}
	}

	if !foundDataFile {
		t.Error("Expected 'Building scenario from data file' event (BuildFromDataFile path)")
	}
	if foundRegular {
		t.Error("Did not expect 'Building test scenario' event (regular Build path) when dataFileObjects is set")
	}
}

// TestFullCommandFlow_DataFileLoadAndBuild tests the full command flow: handleLoadDataFile -> Build -> BuildFromDataFile
// Runs without t.Parallel() so temp dir cleanup can succeed: async hash registry save worker may still
// be draining (saveBatchTimeout 100ms); a short wait allows it to finish before RemoveAll.
func TestFullCommandFlow_DataFileLoadAndBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test-data.yaml")

	// Create test YAML file
	yamlContent := `- kind: account
  title: System Account
  username: system
  status: active
- kind: workstream
  id: WS-001
  title: Test Workstream
  status: active
`

	if err := fileutil.WriteFile(dataFile, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test data file: %v", err)
	}

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.config.TargetDir = tmpDir

	// Capture events - subscribe BEFORE loading data file
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	// Step 1: Load data file (simulating handleLoadDataFile)
	flags := &ScenarioBuilderFlags{
		DataFile: dataFile,
		Force:    false,
	}
	if err := handleLoadDataFile(nil, builder, flags); err != nil {
		t.Fatalf("handleLoadDataFile failed: %v", err)
	}

	// Verify data was loaded
	if len(builder.dataFileObjects) == 0 {
		t.Fatal("Expected dataFileObjects to be populated after handleLoadDataFile")
	}

	// Wait for "Loaded objects from data file" event using callback
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Loaded objects from data file"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Errorf("Expected 'Loaded objects from data file' event after handleLoadDataFile, but got: %v", infoLogs)
	}

	// Step 2: Call Build (simulating runScenarioBuilder -> builder.Build())
	ctx := pkgctx.NewSystemContext()
	buildErr := builder.Build(ctx)

	// Wait for "Building scenario from data file" event using callback
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Building scenario from data file"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Error("Expected 'Building scenario from data file' event after Build()")
		t.Logf("Info logs received: %v", infoLogs)
	}

	// Build may return error for incomplete setup, but BuildFromDataFile should have been called
	if buildErr != nil {
		t.Logf("Build returned error (may be expected for test setup): %v", buildErr)
	}

	// Allow async hash registry save worker to process pending batch (saveBatchTimeout 100ms)
	// so temp dir cleanup does not hit "directory not empty".
	time.Sleep(250 * time.Millisecond)
}

// TestHandleLoadDataFile_ActualYAMLFile tests with the actual scenario-data.yaml format
func TestHandleLoadDataFile_ActualYAMLFile(t *testing.T) {
	t.Parallel()
	// Use the actual scenario-data.yaml file if it exists (relative to project root)
	// Try multiple possible locations
	var dataFile string
	possiblePaths := []string{
		"test-scenarios/ai-job-search-tool/scenario-data.yaml",
		"../test-scenarios/ai-job-search-tool/scenario-data.yaml",
		filepath.Join("..", "..", "test-scenarios", "ai-job-search-tool", "scenario-data.yaml"),
	}

	found := false
	for _, path := range possiblePaths {
		if _, err := fileutil.Stat(path); err == nil {
			dataFile = path
			found = true
			break
		}
	}

	if !found {
		t.Skip("scenario-data.yaml not found in any expected location, skipping test")
	}

	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	flags := &ScenarioBuilderFlags{
		DataFile: dataFile,
		Force:    false,
	}

	// Load data file
	if err := handleLoadDataFile(nil, builder, flags); err != nil {
		t.Fatalf("handleLoadDataFile failed: %v", err)
	}

	// Verify objects were loaded
	if len(builder.dataFileObjects) == 0 {
		t.Error("Expected objects to be loaded from scenario-data.yaml, but got 0")
	} else {
		t.Logf("Successfully loaded %d objects from scenario-data.yaml", len(builder.dataFileObjects))
	}

	// Verify we have expected kinds
	kinds := make(map[string]int)
	for _, obj := range builder.dataFileObjects {
		if kind, ok := obj[objects.FieldKeyKind].(string); ok {
			kinds[kind]++
		}
	}

	// Check for expected kinds from the scenario
	expectedKinds := []string{"account", "workstream", "goal", scenarioBuilderKindBacklogItem}
	for _, expectedKind := range expectedKinds {
		if count, ok := kinds[expectedKind]; !ok || count == 0 {
			t.Errorf("Expected to find at least one %s object, but found %d", expectedKind, count)
		}
	}
}
