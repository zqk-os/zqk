# Test Configuration Package

This package provides test configuration support to isolate test data from actual project data.

## Overview

When running tests, you can configure the system to use a separate test data directory instead of the actual project data. This ensures that:

1. Tests don't modify or interfere with real project data
2. Tests can run in parallel without conflicts
3. Test data can be easily cleaned up after tests complete

## Usage

### Environment Variables

The test configuration is controlled via environment variables:

- **`NEXOS_TEST_ROOT`**: Root directory for test data. When set, all operations will use this directory instead of the actual project root.
- **`NEXOS_TEST_DATA_DIR`**: (Optional) Specific directory for test data. If not set, defaults to `NEXOS_TEST_ROOT/docs/process`.

### Example: Setting Up Test Environment

```go
func TestMyFeature(t *testing.T) {
    // Create temporary directory for test data
    tmpDir := t.TempDir()
    
    // Set environment variable to use test data
    os.Setenv("NEXOS_TEST_ROOT", tmpDir)
    defer os.Unsetenv("NEXOS_TEST_ROOT")
    
    // Setup test environment structure
    if _, err := testconfig.SetupTestEnvironment(tmpDir); err != nil {
        t.Fatalf("failed to setup test environment: %v", err)
    }
    
    // Now all storage operations will use tmpDir instead of project root
    storage, err := storage.NewFileObjectStorage("")
    // storage will automatically use tmpDir due to NEXOS_TEST_ROOT
}
```

### Automatic Detection

The `NewFileObjectStorage()` function automatically detects the `NEXOS_TEST_ROOT` environment variable and uses it if set:

```go
// This will use NEXOS_TEST_ROOT if set, otherwise uses projectRoot
storage, err := storage.NewFileObjectStorage(projectRoot)
```

### CLI Testing

When testing CLI commands, set `NEXOS_TEST_ROOT` before building/running the CLI:

```go
func setupCLITestEnvironment(t *testing.T) (string, string) {
    tmpDir, _ := os.MkdirTemp("", "zqk-cli-test-*")
    
    // Set environment variable
    os.Setenv("NEXOS_TEST_ROOT", tmpDir)
    t.Cleanup(func() {
        os.Unsetenv("NEXOS_TEST_ROOT")
    })
    
    // Setup test environment
    testconfig.SetupTestEnvironment(tmpDir)
    
    // Build CLI - it will use test data when executed
    // ...
}
```

## API Reference

### `GetTestConfig() *TestConfig`

Loads test configuration from environment variables. Returns a `TestConfig` struct with:
- `UseTestConfig`: Whether test configuration is enabled
- `TestProjectRoot`: Root directory for test data
- `TestDataDir`: Directory containing test data

### `SetupTestEnvironment(testRoot string) (string, error)`

Creates the necessary directory structure for a test environment:
- `.zqk/` - Configuration directory
- `docs/process/` - Process data directory
- `docs/process/_internal/object_specs/` - Object specifications directory

### `TestConfig.GetProjectRoot(projectRoot string) string`

Returns the project root to use. If test config is enabled, returns `TestProjectRoot`, otherwise returns the provided `projectRoot`.

### `TestConfig.GetDataDir(dataDir string) string`

Returns the data directory to use. If test config is enabled, returns `TestDataDir`, otherwise returns the provided `dataDir`.

## Best Practices

1. **Always use `t.TempDir()`** for creating test directories - it automatically cleans up after tests
2. **Set environment variables in test setup** and clean them up with `t.Cleanup()` or `defer`
3. **Use `SetupTestEnvironment()`** to ensure all necessary directories exist
4. **Copy spec files** from the real project to test environment if needed for validation

## Example: Complete Test Setup

```go
func TestCreateObject(t *testing.T) {
    // Create temporary directory
    tmpDir := t.TempDir()
    
    // Set test environment
    os.Setenv("NEXOS_TEST_ROOT", tmpDir)
    t.Cleanup(func() {
        os.Unsetenv("NEXOS_TEST_ROOT")
    })
    
    // Setup test environment structure
    if _, err := testconfig.SetupTestEnvironment(tmpDir); err != nil {
        t.Fatalf("failed to setup test environment: %v", err)
    }
    
    // Copy spec files if needed
    // ...
    
    // Create storage - will use test data
    storage, err := storage.NewFileObjectStorage("")
    if err != nil {
        t.Fatalf("failed to create storage: %v", err)
    }
    
    // Run tests - all data goes to tmpDir
    // ...
}
```

