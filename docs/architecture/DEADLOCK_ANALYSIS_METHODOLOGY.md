# Deadlock Analysis Methodology

## Overview
This document outlines the systematic methodology for analyzing potential deadlocks and hangs in concurrent code.

## Why Initial Analysis Missed Issues

### What We Missed
1. **Nested Lock Pattern**: `HashRegistry.Save()` holding RLock while trying to acquire Lock
2. **Blocking I/O While Holding Locks**: `file.Sync()` called while holding mutex
3. **Lock Contention During File I/O**: Multiple goroutines blocking on same registry file operations
4. **Real Validation Function Complexity**: Tests used simplified validation, not the real `checkObjectWithCacheAndContent`

### Root Causes of Missed Issues
1. **Incomplete Call Chain Analysis**: Didn't trace through the entire validation path
2. **Assumed Thread Safety**: Assumed `HashRegistry` was fully thread-safe without checking implementation
3. **Test Coverage Gap**: Tests didn't use real validation function with real blocking operations
4. **Lock Pattern Assumptions**: Didn't systematically check for nested locks or lock+blocking I/O patterns

## Systematic Analysis Methodology

### Phase 1: Call Chain Tracing
1. **Start from Entry Point**: Trace from validation goroutine → validation function → all called functions
2. **Map All Lock Acquisitions**: For each function, identify:
   - Which mutexes are acquired
   - Lock type (Lock vs RLock)
   - When locks are acquired and released
   - What operations happen while holding locks
3. **Identify Blocking Operations**: For each lock hold, check for:
   - File I/O (`os.ReadFile`, `os.WriteFile`, `file.Sync()`)
   - Network I/O
   - Channel operations (blocking sends/receives)
   - System calls that could block

### Phase 2: Lock Pattern Analysis
1. **Nested Lock Detection**: Check for patterns like:
   ```go
   mu1.Lock()
   // ... work ...
   mu2.Lock()  // Nested lock - potential deadlock
   ```
2. **Lock Upgrade Detection**: Check for RLock → Lock upgrades:
   ```go
   mu.RLock()
   // ... work ...
   mu.Lock()  // Upgrade - can deadlock if other RLock holders exist
   ```
3. **Lock + Blocking I/O**: Check for:
   ```go
   mu.Lock()
   file.Sync()  // Blocking I/O while holding lock - causes contention
   mu.Unlock()
   ```

### Phase 3: Concurrency Scenario Analysis
1. **Multiple Goroutines, Same Resource**: 
   - Identify shared resources (files, caches, registries)
   - Check if multiple goroutines access them concurrently
   - Verify locks protect all access paths
2. **Lock Contention Points**:
   - Identify hot paths where many goroutines compete for same lock
   - Check if blocking operations increase contention
   - Verify lock hold times are minimized

### Phase 4: Test Coverage Validation
1. **Real Function Testing**: Tests must use the actual validation function, not simplified mocks
2. **Concurrency Testing**: Tests must simulate realistic concurrency (multiple goroutines, shared resources)
3. **Blocking Operation Testing**: Tests must include actual file I/O, not just in-memory operations
4. **Stress Testing**: Tests must use realistic data volumes (15k+ objects) to trigger contention

## Checklist for Code Review

### Before Merging Concurrent Code
- [ ] **Call Chain Traced**: All functions in the call chain have been analyzed
- [ ] **Locks Documented**: Every mutex acquisition is documented with:
  - Lock type (Lock/RLock)
  - What operations happen while holding lock
  - Lock hold duration (should be minimal)
- [ ] **Blocking Operations Identified**: All blocking operations (I/O, system calls) are identified
- [ ] **No Lock + Blocking I/O**: No blocking operations happen while holding locks
- [ ] **No Nested Locks**: No function acquires a second lock while holding a first (unless explicitly safe)
- [ ] **No Lock Upgrades**: No RLock → Lock upgrades in the same function
- [ ] **Shared Resources Protected**: All shared resources have proper locking
- [ ] **Tests Use Real Functions**: Tests use actual implementation, not simplified mocks
- [ ] **Tests Include Concurrency**: Tests simulate realistic concurrent access patterns
- [ ] **Tests Include Blocking I/O**: Tests include actual file I/O operations

## Specific Patterns to Check

### Pattern 1: Lock + File I/O (BAD)
```go
mu.Lock()
defer mu.Unlock()
data, _ := os.ReadFile(path)  // Blocking I/O while holding lock
file.Sync()  // Blocking I/O while holding lock
```

**Fix**: Release lock before I/O
```go
mu.Lock()
data := copyData()  // Copy data quickly
mu.Unlock()
os.ReadFile(path)  // I/O without lock
```

### Pattern 2: Nested Locks (BAD)
```go
mu1.Lock()
// ... work ...
mu2.Lock()  // Nested - potential deadlock
mu2.Unlock()
mu1.Unlock()
```

**Fix**: Acquire all locks at once, or restructure to avoid nesting

### Pattern 3: Lock Upgrade (BAD)
```go
mu.RLock()
// ... read work ...
mu.Lock()  // Upgrade - can deadlock
mu.RUnlock()
mu.Unlock()
```

**Fix**: Release RLock, then acquire Lock separately

### Pattern 4: Lock Through Long Operation (BAD)
```go
mu.Lock()
defer mu.Unlock()
expensiveOperation()  // Takes a long time
```

**Fix**: Do work without lock, or minimize lock hold time

## Tools and Techniques

### Static Analysis
- Use `go vet` to detect some lock issues
- Use `golangci-lint` with `gocritic` for lock pattern detection
- Custom analysis: grep for lock patterns and verify no blocking I/O

### Dynamic Analysis
- Use `go test -race` to detect data races
- Use goroutine dumps (`kill -QUIT`) to identify blocked goroutines
- Use `pprof` to identify lock contention hotspots

### Testing
- Create tests that use real validation functions
- Use realistic data volumes (15k+ objects)
- Simulate concurrent access patterns
- Include actual file I/O operations

## Example: HashRegistry.Save() Analysis

### What We Should Have Found
1. **Call Chain**: `validation goroutine` → `checkObjectWithCacheAndContent` → `registry.Save()`
2. **Lock Pattern**: `Save()` acquires `hr.mu.RLock()`, does blocking `file.Sync()`, then tries `hr.mu.Lock()`
3. **Blocking I/O**: `file.Sync()` is a blocking system call
4. **Nested Lock**: RLock → Lock upgrade while RLock still held (via defer)

### How to Find It
1. Grep for `registry.Save()` calls
2. Read `HashRegistry.Save()` implementation
3. Check for lock acquisitions
4. Check for blocking operations while holding locks
5. Check for nested lock patterns

## Action Items

1. **Immediate**: Apply this methodology to all concurrent code paths
2. **Short-term**: Create automated checks for lock+blocking I/O patterns
3. **Long-term**: Integrate deadlock analysis into CI/CD pipeline

