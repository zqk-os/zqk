# Test Separation: Unit vs Integration

## Unit Tests (Fast - Run by Default)

**Command**: `go test ./pkg/storage -short`

- **Time**: ~1-2 seconds
- **Purpose**: Quick validation of individual storage components
- **Characteristics**:
  - Isolated components
  - No real file system operations (uses t.TempDir())
  - No background workers
  - Marked with `t.Parallel()` for concurrent execution

## Integration Tests (Slow - Run Separately)

**Command**: `go test ./pkg/storage -run "Integration"` or `go test ./pkg/storage` (without `-short`)

- **Time**: Varies
- **Purpose**: Test storage components working together with real file systems
- **Characteristics**:
  - Real file system operations
  - Background workers and goroutines
  - CLI command execution
  - Marked with `if testing.Short() { t.Skip(...) }`

## Integration Test Files

- `cli_integration_test.go` - CLI command execution
- `bucketing_strategy_integration_test.go` - Bucketing strategy validation
- `content_addressable_storage_integration_test.go` - CAS integration scenarios

## Best Practices

1. **Development**: Run `go test ./pkg/storage -short` for fast feedback
2. **CI/CD**: Run unit tests in PR checks, integration tests in nightly builds
3. **Local Testing**: Run integration tests separately when needed
4. **Never mix**: Unit and integration tests should never run together in the same command without `-short`
