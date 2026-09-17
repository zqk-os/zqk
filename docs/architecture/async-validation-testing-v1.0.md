# Async Validation Testing Guide v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Guide for testing async validation system with isolated test data

## Overview

The async validation system includes comprehensive tests that use isolated test data to prevent corruption of project data. All tests use the `ZQK_TEST_ROOT` environment variable to create temporary test environments.

## Test Isolation

### Environment Variables

Tests use the following environment variables for isolation:

- **`ZQK_TEST_ROOT`**: Root directory for test data (creates isolated test environment)
- **`ZQK_TEST_DATA_DIR`**: (Optional) Specific directory for test data

### Test Configuration

The `pkg/testing` package provides test configuration:

```go
import testconfig "github.com/lanceman/zqk/pkg/testing"

// Setup test environment
tmpDir := t.TempDir()
os.Setenv("ZQK_TEST_ROOT", tmpDir)
defer os.Unsetenv("ZQK_TEST_ROOT")

testRoot, err := testconfig.SetupTestEnvironment(tmpDir)
```

## Running Tests

### Run All Validation Tests

```bash
go test ./pkg/validation/... -v
```

### Run Specific Test Suites

```bash
# State cache tests
go test ./pkg/validation/... -v -run TestValidationStateCache

# Priority queue tests
go test ./pkg/validation/... -v -run TestPriorityQueue

# Async validator tests
go test ./pkg/validation/... -v -run TestAsyncValidator
```

### Run Individual Tests

```bash
go test ./pkg/validation/... -v -run TestValidationStateCache_BasicOperations
```

## Test Coverage

### State Cache Tests (`state_cache_test.go`)

- ✅ Basic operations (Set, Get, Invalidate)
- ✅ Persistence (Save/Load)
- ✅ Stale detection
- ✅ Invalidate by kind
- ✅ Get by tier
- ✅ Count statistics

### Priority Queue Tests (`priority_queue_test.go`)

- ✅ Basic operations (Enqueue, Dequeue)
- ✅ Priority ordering
- ✅ Same priority ordering (FIFO)
- ✅ Peek operation
- ✅ Get by priority
- ✅ Clear operation

### Async Validator Tests (`async_validator_test.go`)

- ✅ Basic operations (Start, Stop, Enqueue)
- ✅ Cache integration
- ✅ Progress reporting
- ✅ ValidateNow (immediate validation)

## Test Data Structure

Tests create the following structure in temporary directories:

```
/tmp/test-XXXXXX/
├── .zqk/
│   └── validation_cache.json
└── docs/
    └── process/
        ├── _internal/
        │   └── object_specs/
        └── test/
            └── TEST-*.yaml
```

## Example Test

```go
func TestMyFeature(t *testing.T) {
    // Create temporary directory
    tmpDir := t.TempDir()
    
    // Set environment variable
    os.Setenv("ZQK_TEST_ROOT", tmpDir)
    defer os.Unsetenv("ZQK_TEST_ROOT")
    
    // Setup test environment
    testRoot, err := testconfig.SetupTestEnvironment(tmpDir)
    if err != nil {
        t.Fatalf("failed to setup test environment: %v", err)
    }
    
    // Create validator with test root
    validator := validation.NewAsyncValidator(testRoot, 2, time.Hour)
    
    // Run tests...
}
```

## Safety Guarantees

1. **No Project Data Pollution**: All tests use `t.TempDir()` which is automatically cleaned up
2. **Isolated Environments**: `ZQK_TEST_ROOT` ensures tests never touch real project data
3. **Automatic Cleanup**: Test directories are removed after tests complete
4. **No Side Effects**: Tests don't modify global state or project files

## Troubleshooting

### Tests Failing with "permission denied"

Ensure test directory is writable:
```bash
chmod -R 755 /tmp/test-*
```

### Tests Hanging

Check for deadlocks in priority queue operations. Use timeout:
```bash
go test ./pkg/validation/... -timeout 10s
```

### Cache File Issues

If validation cache tests fail, check that `.zqk/validation_cache.json` is writable in test directory.

## Related Documentation

- [Async Validation System](./async-validation-system-v1.0.md)
- [Test Data Isolation Policy](../policies/POL-CODE-006.yaml)
- [Testing Package](../../pkg/testing/README.md)

