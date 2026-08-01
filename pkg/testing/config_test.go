package testing

// ITEM-177483 inventory: env-only GetTestConfig tests; t.TempDir + t.Setenv; no RunProjectTestTeardown (no storage).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestGetTestConfig_NoEnv(t *testing.T) {
	// Not t.Parallel(): t.Setenv(ZQK_TEST_*) is invalid after Parallel; empty env matches "not configured".
	t.Setenv(zqkenv.TestRoot(), "")
	t.Setenv(zqkenv.TestDataDir(), "")

	config := GetTestConfig()
	if config.UseTestConfig {
		t.Error("expected UseTestConfig to be false when ZQK_TEST_ROOT is not set")
	}
}

func TestGetTestConfig_WithTestRoot(t *testing.T) {
	// Not t.Parallel(): t.Setenv(ZQK_TEST_ROOT).
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)

	config := GetTestConfig()
	if !config.UseTestConfig {
		t.Error("expected UseTestConfig to be true when ZQK_TEST_ROOT is set")
	}
	if config.TestProjectRoot != tmpDir {
		t.Errorf("expected TestProjectRoot to be %s, got %s", tmpDir, config.TestProjectRoot)
	}
	expectedDataDir := datacell.ProcessPrimaryDir(tmpDir)
	if config.TestDataDir != expectedDataDir {
		t.Errorf("expected TestDataDir to be %s, got %s", expectedDataDir, config.TestDataDir)
	}
}

func TestGetTestConfig_WithTestDataDir(t *testing.T) {
	// Not t.Parallel(): t.Setenv(ZQK_TEST_*).
	tmpDir := t.TempDir()
	customDataDir := filepath.Join(tmpDir, "custom", "data")
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	t.Setenv(zqkenv.TestDataDir(), customDataDir)

	config := GetTestConfig()
	if config.TestDataDir != customDataDir {
		t.Errorf("expected TestDataDir to be %s, got %s", customDataDir, config.TestDataDir)
	}
}

func TestGetProjectRoot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := &TestConfig{
		TestProjectRoot: tmpDir,
		UseTestConfig:   true,
	}

	actualRoot := config.GetProjectRoot("/real/project/root")
	if actualRoot != tmpDir {
		t.Errorf("expected test root %s, got %s", tmpDir, actualRoot)
	}

	config.UseTestConfig = false
	actualRoot = config.GetProjectRoot("/real/project/root")
	if actualRoot != "/real/project/root" {
		t.Errorf("expected real root /real/project/root, got %s", actualRoot)
	}
}

func TestGetDataDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDataDir := filepath.Join(tmpDir, "test", "data")
	config := &TestConfig{
		TestDataDir:   testDataDir,
		UseTestConfig: true,
	}

	realProcessDir := datacell.ProcessPrimaryDir("/real/project")
	actualDataDir := config.GetDataDir(realProcessDir)
	if actualDataDir != testDataDir {
		t.Errorf("expected test data dir %s, got %s", testDataDir, actualDataDir)
	}

	config.UseTestConfig = false
	actualDataDir = config.GetDataDir(realProcessDir)
	if actualDataDir != realProcessDir {
		t.Errorf("expected real data dir %s, got %s", realProcessDir, actualDataDir)
	}
}

func TestSetupTestEnvironment(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testRoot, err := SetupTestEnvironment(tmpDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment failed: %v", err)
	}

	if testRoot != tmpDir {
		t.Errorf("expected test root %s, got %s", tmpDir, testRoot)
	}

	// Verify directories were created
	dataDir := filepath.Join(testRoot, paths.ProjectDataDir)
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Errorf("project data directory (%s) was not created", paths.ProjectDataDir)
	}

	processDir := datacell.ProcessPrimaryDir(testRoot)
	if _, err := os.Stat(processDir); os.IsNotExist(err) {
		t.Errorf("process directory (%s) was not created", paths.ProcessDir)
	}

	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		t.Error("specs directory was not created")
	}
}
