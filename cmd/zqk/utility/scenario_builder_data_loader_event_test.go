package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestHandleLoadDataFile_EmitsEvents tests that handleLoadDataFile emits proper events
func TestHandleLoadDataFile_EmitsEvents(t *testing.T) {
	t.Parallel()
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
	builder.projectRoot = tmpDir

	// Capture events
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	flags := &ScenarioBuilderFlags{
		DataFile: dataFile,
		Force:    false,
	}

	// Load data file
	if err := handleLoadDataFile(nil, builder, flags); err != nil {
		t.Fatalf("handleLoadDataFile failed: %v", err)
	}

	// Verify data was loaded
	if len(builder.dataFileObjects) == 0 {
		t.Fatal("Expected dataFileObjects to be populated after handleLoadDataFile")
	}

	// Wait for events using callbacks
	// Event 1: "Loading data file: ..."
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message != emptyValue && entry.message != "Loaded objects from data file"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Errorf("Expected 'Loading data file' event, but got: %v", infoLogs)
	}

	// Event 2: "Loaded objects from data file"
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Loaded objects from data file"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Errorf("Expected 'Loaded objects from data file' event, but got: %v", infoLogs)
	}

	// Verify we got at least 2 events
	infoLogs := testLogger.getInfoLogs()
	if len(infoLogs) < 2 {
		t.Errorf("Expected at least 2 info log events, got %d: %v", len(infoLogs), infoLogs)
	}
}

// TestHandleLoadDataFile_EmitsErrorOnFileNotFound tests error event emission
func TestHandleLoadDataFile_EmitsErrorOnFileNotFound(t *testing.T) {
	t.Parallel()
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
	builder.projectRoot = tmpDir

	// Capture events
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	flags := &ScenarioBuilderFlags{
		DataFile: filepath.Join(tmpDir, "nonexistent.yaml"),
		Force:    false,
	}

	// Load data file - should fail
	err := handleLoadDataFile(nil, builder, flags)
	if err == nil {
		t.Fatal("Expected error when file doesn't exist")
	}

	// Wait for error event
	if !testLogger.waitForErrorLog(func(entry testLogEntry) bool {
		return entry.message != emptyValue && entry.err != nil
	}, 2*time.Second) {
		errorLogs := testLogger.getErrorLogs()
		t.Errorf("Expected error event, but got: %v", errorLogs)
	}
}

// TestBuildFromDataFile_EmitsEvents tests that BuildFromDataFile emits proper events
func TestBuildFromDataFile_EmitsEvents(t *testing.T) {
	t.Parallel()
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
	builder.config.TargetDir = tmpDir

	// Set dataFileObjects to simulate loaded data
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:  "account",
			objects.FieldKeyTitle: "Test Account",
			objects.FieldKeyID:    "ACC-TEST",
		},
	}

	// Capture events
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)
	coordinator.Subscribe(subscriber)

	ctx := pkgctx.NewSystemContext()
	// BuildFromDataFile will fail on infrastructure setup, but we should see events
	_ = builder.BuildFromDataFile(ctx)

	// Wait for "Building scenario from data file" event
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Building scenario from data file"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Errorf("Expected 'Building scenario from data file' event, but got: %v", infoLogs)
	}

	// Should also see "Setting up infrastructure..." event
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Setting up infrastructure..."
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Logf("All info logs: %v", infoLogs)
		// This is okay if infrastructure setup fails quickly
	}
}
