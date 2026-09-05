# Test Separation: Unit vs Integration

**Last Verified:** 2026-08-31


## Unit Tests (Fast - Run by Default)

**Command**: `go test ./pkg/scheduler -short`

- **Time**: ~1-2 seconds
- **Purpose**: Quick validation of individual components
- **Characteristics**:
  - Isolated components
  - No real scheduler instances
  - No file locks or PID files
  - Marked with `t.Parallel()` for concurrent execution

## Integration Tests (Slow - Run Separately)

**Command**: `go test ./pkg/scheduler -run "Integration"` or `go test ./pkg/scheduler` (without `-short`)

- **Time**: ~155 seconds
- **Purpose**: Test components working together in realistic scenarios
- **Characteristics**:
  - Real scheduler instances (`scheduler.Start(ctx)`)
  - File locks, PID files, keep-alive files
  - Background workers and goroutines
  - Marked with `if testing.Short() { t.Skip(...) }`

## Integration Test Files

- `keepalive_integration_test.go` - Keep-alive heartbeat (~40s)
- `job_lock_integration_test.go` - File-based locking (~30-40s each)
- `scheduler_submit_integration_test.go` - Full scheduler submit flow (~30s each)
- `handlers_run_wrapper_test_command_test.go` - Test command execution (some tests)

## Best Practices

1. **Development**: Run `go test ./pkg/scheduler -short` for fast feedback
2. **CI/CD**: Run unit tests in PR checks, integration tests in nightly builds
3. **Local Testing**: Run integration tests separately when needed
4. **Never mix**: Unit and integration tests should never run together in the same command without `-short`
