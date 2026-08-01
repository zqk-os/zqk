# Goroutine Naming Implementation

## Summary

Added meaningful names to goroutines for better profiling visibility. Goroutines now appear with descriptive names in profiler output (pprof, sample, etc.) making it easier to identify what work each goroutine is doing.

## Implementation

### New Package: `pkg/goroutinelabels`

Created a lightweight package that provides goroutine labeling functionality using Go's `runtime/pprof` API:

- `SetGoroutineLabel(name, purpose)` - Sets labels for the current goroutine
- `StartNamedGoroutine(name, purpose, fn)` - Convenience wrapper to start a named goroutine
- `DoWithLabels(name, purpose, fn)` - Executes function with labels (recommended approach)

### Updated: `pkg/validation/async_validator.go`

Updated `AsyncValidator` to use goroutine labels:

1. **Worker goroutines**: Named as `validation_worker_0`, `validation_worker_1`, etc.
   - Purpose: "processing validation queue (worker X of Y)"

2. **Validation task goroutines**: Named as `validation_task_<object_id>`
   - Purpose: "validating <object_id> (<object_kind>)"

## Usage Example

```go
import "github.com/lanceman/zqk/pkg/goroutinelabels"

// Option 1: Set label at start of goroutine
go func() {
    goroutinelabels.SetGoroutineLabel("my_worker", "processing tasks")
    // ... work ...
}()

// Option 2: Use convenience wrapper
goroutinelabels.StartNamedGoroutine("my_worker", "processing tasks", func() {
    // ... work ...
})
```

## Benefits

1. **Better Profiling**: Goroutines appear with meaningful names in:
   - CPU profiles (`go tool pprof`)
   - Goroutine profiles (`go tool pprof`)
   - macOS `sample` profiler output
   - Stack traces

2. **Easier Debugging**: Can quickly identify which goroutines are:
   - Blocked/waiting
   - Doing actual work
   - Causing performance issues

3. **Performance Analysis**: Makes it easier to correlate profiler output with actual code paths

## Next Steps

Consider adding labels to other goroutine creation points:
- `cmd/zqk/system/*` - System check workers
- `pkg/scheduler/*` - Scheduler workers
- `pkg/storage/*` - Storage workers
- `pkg/mcp/*` - MCP handlers

## Notes

- Labels only appear in CPU and goroutine profiles (not heap, block, etc.)
- Some stack trace dumps may not include labels (known Go issue #63712)
- Minimal performance overhead
