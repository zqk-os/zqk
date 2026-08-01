# Deadlock Analysis Checklist

## Pre-Implementation Analysis

Before implementing or modifying concurrent code, complete this checklist:

### 1. Call Chain Analysis
- [ ] **Trace complete call chain**: From entry point (goroutine) → all called functions → all sub-functions
- [ ] **Map all mutexes**: Identify every `sync.Mutex`, `sync.RWMutex`, `sync.Map` in the call chain
- [ ] **Document lock acquisitions**: For each function, list:
  - Which mutexes are acquired
  - Lock type (Lock vs RLock)
  - When locks are acquired and released
  - What operations happen while holding locks

### 2. Blocking Operation Detection
For each lock hold, check for:
- [ ] **File I/O**: `os.ReadFile`, `os.WriteFile`, `file.Sync()`, `os.Stat`, `os.MkdirAll`
- [ ] **Network I/O**: HTTP requests, database queries, socket operations
- [ ] **Channel operations**: Blocking sends/receives (non-buffered or full channels)
- [ ] **System calls**: Any syscall that could block
- [ ] **JSON/YAML operations**: `json.Marshal`, `yaml.Marshal` (can be slow for large data)

**Rule**: If any blocking operation is found while holding a lock, it's a potential deadlock/hang.

### 3. Lock Pattern Analysis
Check for these anti-patterns:

- [ ] **Nested Locks**: Function acquires lock A, then lock B
  ```go
  mu1.Lock()
  mu2.Lock()  // NESTED - potential deadlock
  ```
  
- [ ] **Lock Upgrade**: Function acquires RLock, then tries to acquire Lock
  ```go
  mu.RLock()
  mu.Lock()  // UPGRADE - can deadlock if other RLock holders exist
  ```

- [ ] **Lock + Blocking I/O**: Lock held during file I/O
  ```go
  mu.Lock()
  file.Sync()  // BLOCKING I/O - causes contention
  mu.Unlock()
  ```

- [ ] **Long Lock Hold**: Lock held during expensive operations
  ```go
  mu.Lock()
  expensiveComputation()  // Takes a long time
  mu.Unlock()
  ```

### 4. Shared Resource Analysis
- [ ] **Identify shared resources**: Files, caches, registries, maps accessed by multiple goroutines
- [ ] **Verify all access paths are protected**: Every read/write to shared resource has proper locking
- [ ] **Check for lock-free alternatives**: Can we use `sync.Map`, channels, or atomic operations instead?

### 5. Test Coverage Validation
- [ ] **Tests use real functions**: Not simplified mocks that skip blocking operations
- [ ] **Tests include concurrency**: Multiple goroutines accessing same resources
- [ ] **Tests include blocking I/O**: Actual file operations, not just in-memory
- [ ] **Tests use realistic scale**: Enough objects (500+) to trigger contention
- [ ] **Tests verify no hangs**: Timeout-based tests that fail if operations hang

## Post-Implementation Verification

After implementing, verify:

- [ ] **Run race detector**: `go test -race ./...`
- [ ] **Run stress tests**: High concurrency, realistic data volumes
- [ ] **Check for goroutine leaks**: Verify all goroutines complete
- [ ] **Profile lock contention**: Use `pprof` to identify hot locks
- [ ] **Review lock hold times**: Log or measure how long locks are held

## Specific Patterns to Check

### Pattern: HashRegistry.Save() (FIXED)
**What we missed**: 
- Acquired RLock, did blocking `file.Sync()`, then tried to acquire Lock
- Multiple goroutines could all hold RLock and block on file I/O
- Lock() would wait for all RLock holders, causing deadlock

**How to catch it**:
1. Grep for `registry.Save()` calls
2. Read `HashRegistry.Save()` implementation
3. Check for lock acquisitions
4. Check for blocking operations (`file.Sync()`)
5. Check for nested locks (RLock → Lock)

### Pattern: checkObjectWithCacheAndContent (FIXED)
**What we missed**:
- Function re-read file from disk even though content was already provided
- Redundant I/O caused unnecessary blocking

**How to catch it**:
1. Trace validation function call chain
2. Check if file is read multiple times
3. Verify content parameter is used instead of re-reading

### Pattern: Callback Blocking (FIXED)
**What we missed**:
- Callback did synchronous JSON marshaling
- This blocked validation goroutines

**How to catch it**:
1. Check what callback does
2. Verify no blocking operations (JSON marshal, file I/O)
3. Make callback async if it does blocking work

## Tools and Commands

### Static Analysis
```bash
# Find all mutex acquisitions
grep -r "\.Lock()" pkg/ cmd/ --include="*.go"

# Find all file I/O operations
grep -r "os\.ReadFile\|os\.WriteFile\|file\.Sync" pkg/ cmd/ --include="*.go"

# Find nested lock patterns
# (Manual review needed - grep can't easily detect this)
```

### Dynamic Analysis
```bash
# Race detector
go test -race ./...

# Goroutine dump (when hung)
kill -QUIT <pid>

# CPU profile
go test -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof
```

### Test Commands
```bash
# Run deadlock tests
go test ./pkg/validation/... -v -run "Deadlock"

# Run with timeout
go test ./pkg/validation/... -timeout 30s

# Stress test
go test ./pkg/validation/... -run "Stress" -count=10
```

## Lessons Learned

1. **Don't assume thread safety**: Always read the implementation of "thread-safe" objects
2. **Trace the full call chain**: Don't stop at the first level of indirection
3. **Check for blocking I/O**: File operations, network calls, system calls can all block
4. **Test with real functions**: Simplified mocks hide real issues
5. **Look for nested locks**: RLock → Lock upgrades are particularly dangerous
6. **Check lock hold times**: Locks should be held for minimal time
7. **Verify all access paths**: Every read/write to shared resource needs protection

## Action Items

- [ ] Apply this checklist to all concurrent code paths
- [ ] Create automated checks for lock+blocking I/O patterns
- [ ] Add deadlock detection tests to CI/CD
- [ ] Review all existing concurrent code using this checklist

