package testing

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"path/filepath"

	"gopkg.in/yaml.v3"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

const emptyValue = ""

// TestConfig provides configuration for test environments
// This allows tests to use isolated data directories separate from project data
type TestConfig struct {
	// TestProjectRoot is the root directory for test data
	// If set, all operations will use this directory instead of the actual project root
	TestProjectRoot string

	// TestDataDir is the directory containing test data (.zqk/process)
	// If not set, defaults to TestProjectRoot/.zqk/process
	TestDataDir string

	// UseTestConfig indicates whether to use test configuration
	UseTestConfig bool
}

// GetTestConfig loads test configuration from environment variables
// Environment variables:
//   - ZQK_TEST_ROOT: Root directory for test data (creates isolated test environment)
//   - ZQK_TEST_DATA_DIR: Specific directory for test data (overrides TestProjectRoot/.zqk/process)
func GetTestConfig() *TestConfig {
	testRoot := zqkenv.TestRoot().Get()
	if testRoot == emptyValue {
		return &TestConfig{
			UseTestConfig: false,
		}
	}

	// Resolve absolute path
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		// If we can't resolve, return disabled config
		return &TestConfig{
			UseTestConfig: false,
		}
	}

	config := &TestConfig{
		TestProjectRoot: absRoot,
		UseTestConfig:   true,
	}

	// Check for explicit test data directory
	testDataDir := zqkenv.TestDataDir().Get()
	if testDataDir != emptyValue {
		absDataDir, err := filepath.Abs(testDataDir)
		if err == nil {
			config.TestDataDir = absDataDir
		}
	} else {
		config.TestDataDir = datacell.ProcessPrimaryDir(absRoot)
	}

	return config
}

// GetProjectRoot returns the project root to use based on test configuration
// If test config is enabled, returns TestProjectRoot, otherwise returns the provided projectRoot
func (tc *TestConfig) GetProjectRoot(projectRoot string) string {
	if !tc.UseTestConfig {
		return projectRoot
	}
	return tc.TestProjectRoot
}

// GetDataDir returns the data directory to use based on test configuration
// If test config is enabled, returns TestDataDir, otherwise returns the computed dataDir
func (tc *TestConfig) GetDataDir(dataDir string) string {
	if !tc.UseTestConfig {
		return dataDir
	}
	return tc.TestDataDir
}

// SetupTestEnvironment creates a test environment with necessary directories
// Returns the test project root and any error
func SetupTestEnvironment(testRoot string) (string, error) {
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		return emptyValue, errfmt.Newf("failed to resolve test root path").Wrap(err)
	}

	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		return emptyValue, errfmt.Newf("failed to create test project layout").Wrap(err)
	}

	// Write test-settings.yaml so code using ZQK_TEST_ROOT loads this instead of project zqk-settings.yaml
	if err := writeTestSettingsFile(absRoot); err != nil {
		return emptyValue, errfmt.Newf("failed to write test settings").Wrap(err)
	}

	return absRoot, nil
}

// testSettingsShape is the minimal shape for test-settings.yaml (same schema as brand_settings).
type testSettingsShape struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

// writeTestSettingsFile writes test-settings.yaml at testRoot so LoadBrandSettings uses it when ZQK_TEST_ROOT is set.
func writeTestSettingsFile(testRoot string) error {
	path := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsShape{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm644) //nolint:gosec // test file
}

// WriteTestConfig writes a test configuration file to .zqk/config.yaml
// This allows the CLI to automatically use test data when running in test mode
func WriteTestConfig(testRoot string, config map[string]any) error {
	dataDir := filepath.Join(testRoot, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(dataDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create project data directory").Wrap(err)
	}

	configPath := filepath.Join(dataDir, paths.ConfigDir, paths.ProjectConfigFile)

	// For now, we'll use a simple approach - the test config will be detected
	// via environment variable. In the future, we could write YAML here.
	// For now, just ensure the directory exists
	_ = configPath

	return nil
}
