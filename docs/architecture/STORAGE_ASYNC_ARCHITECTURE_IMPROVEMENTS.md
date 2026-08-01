# Storage Layer Async Architecture Improvements

## Problem Statement

The WaitGroup panic is difficult to reproduce and isolate because the architecture has unclear separation of concerns and hidden async behavior. Good architecture should have:
- Clear intent with the code
- Clean, clear lines of separation
- Easy to test
- No mysterious behavior

## Current Architectural Issues

### 1. **Inconsistent WaitGroup Management Patterns**

**Problem**: Multiple patterns for managing WaitGroups across the codebase:
- Some use `goroutinelabels.WithWaitGroup()` (only calls `Done()`)
- Some manually call `wg.Add(1)` before starting
- Some manually call `wg.Done()` in defer (causes double Done())
- No centralized lifecycle management

**Impact**: Easy to introduce bugs, hard to reason about, difficult to test

### 2. **Hidden Async Operations**

**Problem**: Storage operations trigger hidden async operations:
- `storage.Create()` → audit event creation (may be async)
- `storage.Create()` → metrics collection (may be async)
- `storage.Create()` → cache invalidation (may be async)
- `OperationExecutor` workers start on-demand (hidden from caller)
- `IOQueueManager` has background workers (hidden from caller)

**Impact**: Caller doesn't know about async operations, can't coordinate lifecycle, can't test easily

### 3. **Unclear Ownership and Lifecycle**

**Problem**: 
- Who owns the WaitGroup lifecycle?
- When does a WaitGroup get created vs reused?
- What happens if an operation fails mid-way?
- Multiple WaitGroups can exist for the same logical operation

**Impact**: Race conditions, panics, difficult to debug

### 4. **Mixed Responsibilities**

**Problem**: Storage layer mixes:
- Synchronous operations (Create, Read, Update, Delete)
- Asynchronous operations (metrics, audit events, cache invalidation)
- Background workers (OperationExecutor, IOQueueManager)
- Queue management

**Impact**: Hard to test, hard to reason about, hard to maintain

## Proposed Architectural Improvements

### 1. **Explicit Async Operation Interface**

Create a clear interface for async operations:

```go
// AsyncOperation represents an operation that can be executed asynchronously
type AsyncOperation interface {
    // Execute executes the operation synchronously
    Execute(ctx context.Context) error
    
    // ExecuteAsync executes the operation asynchronously, returns a Future
    ExecuteAsync(ctx context.Context) (*Future, error)
}

// Future represents the result of an async operation
type Future struct {
    wg sync.WaitGroup
    err error
    mu sync.Mutex
}

func (f *Future) Wait() error {
    f.wg.Wait()
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.err
}
```

**Benefits**:
- Clear intent: caller knows if operation is async
- Testable: can test sync and async paths separately
- Observable: can track async operations

### 2. **Centralized WaitGroup Manager**

Create a single point of responsibility for WaitGroup lifecycle:

```go
// WaitGroupManager manages WaitGroup lifecycle for async operations
type WaitGroupManager struct {
    wgs map[string]*sync.WaitGroup
    mu  sync.RWMutex
}

func (m *WaitGroupManager) CreateGroup(id string) *sync.WaitGroup {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    wg := &sync.WaitGroup{}
    m.wgs[id] = wg
    return wg
}

func (m *WaitGroupManager) Add(id string, delta int) {
    m.mu.RLock()
    wg, exists := m.wgs[id]
    m.mu.RUnlock()
    
    if !exists {
        panic(fmt.Sprintf("WaitGroup %s does not exist", id))
    }
    
    wg.Add(delta)
}

func (m *WaitGroupManager) Done(id string) {
    m.mu.RLock()
    wg, exists := m.wgs[id]
    m.mu.RUnlock()
    
    if !exists {
        panic(fmt.Sprintf("WaitGroup %s does not exist", id))
    }
    
    wg.Done()
}

func (m *WaitGroupManager) Wait(id string) {
    m.mu.RLock()
    wg, exists := m.wgs[id]
    m.mu.RUnlock()
    
    if !exists {
        return // Group doesn't exist, nothing to wait for
    }
    
    wg.Wait()
}
```

**Benefits**:
- Single source of truth for WaitGroup lifecycle
- Can track all WaitGroups
- Can detect leaks (WaitGroups that never complete)
- Easier to test and debug

### 3. **Separate Sync and Async Operations**

Make async operations explicit and optional:

```go
// StorageConfig configures storage behavior
type StorageConfig struct {
    // AsyncOperations enables async operations (metrics, audit events)
    // If false, all operations are synchronous
    AsyncOperations bool
    
    // AsyncOperationTimeout is the timeout for async operations
    AsyncOperationTimeout time.Duration
}

// FileObjectStorage with explicit async configuration
type FileObjectStorage struct {
    // ... existing fields ...
    
    config StorageConfig
    asyncManager *AsyncOperationManager
}

// Create with explicit async handling
func (f *FileObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
    // Synchronous core operation
    if err := f.createSync(ctx, secCtx, obj); err != nil {
        return err
    }
    
    // Explicit async operations (if enabled)
    if f.config.AsyncOperations {
        f.asyncManager.ScheduleAuditEvent(ctx, secCtx, "create", obj)
        f.asyncManager.ScheduleMetrics(ctx, "create", obj)
    }
    
    return nil
}
```

**Benefits**:
- Clear separation: sync vs async
- Testable: can disable async for testing
- Observable: can track async operations
- Predictable: caller knows what's async

### 4. **Operation Context with Lifecycle Tracking**

Track operation lifecycle explicitly:

```go
// OperationContext tracks the lifecycle of an operation
type OperationContext struct {
    ID string
    OperationType string
    StartTime time.Time
    WaitGroups []string // IDs of WaitGroups created for this operation
    mu sync.Mutex
}

func (oc *OperationContext) AddWaitGroup(id string) {
    oc.mu.Lock()
    defer oc.mu.Unlock()
    oc.WaitGroups = append(oc.WaitGroups, id)
}

func (oc *OperationContext) WaitForAll() error {
    oc.mu.Lock()
    wgIDs := make([]string, len(oc.WaitGroups))
    copy(wgIDs, oc.WaitGroups)
    oc.mu.Unlock()
    
    for _, id := range wgIDs {
        waitGroupManager.Wait(id)
    }
    
    return nil
}
```

**Benefits**:
- Can track all WaitGroups for an operation
- Can detect leaks (WaitGroups that never complete)
- Can wait for all async operations to complete
- Easier to debug

### 5. **Testable Architecture**

Make it easy to test:

```go
// StorageTestConfig for testing
type StorageTestConfig struct {
    // DisableAsync disables all async operations
    DisableAsync bool
    
    // WaitGroupManager for tracking WaitGroups in tests
    WaitGroupManager *WaitGroupManager
    
    // OperationContext for tracking operations in tests
    OperationContext *OperationContext
}

// Test helper
func NewTestStorage(config StorageTestConfig) *FileObjectStorage {
    storage := &FileObjectStorage{
        config: StorageConfig{
            AsyncOperations: !config.DisableAsync,
        },
        waitGroupManager: config.WaitGroupManager,
    }
    
    return storage
}
```

**Benefits**:
- Easy to test sync operations without async noise
- Can track WaitGroups in tests
- Can verify no leaks in tests

## Implementation Plan

### Phase 1: Centralize WaitGroup Management
1. Create `WaitGroupManager` interface
2. Replace all direct `sync.WaitGroup` usage with `WaitGroupManager`
3. Add tracking and observability

### Phase 2: Make Async Operations Explicit
1. Create `AsyncOperationManager` interface
2. Move all async operations (metrics, audit events) to `AsyncOperationManager`
3. Make async operations optional via config

### Phase 3: Add Operation Context
1. Create `OperationContext` for tracking operation lifecycle
2. Pass `OperationContext` through all operations
3. Track all WaitGroups per operation

### Phase 4: Improve Testability
1. Create test helpers for disabling async operations
2. Add test utilities for tracking WaitGroups
3. Update existing tests to use new architecture

## Benefits

1. **Clear Intent**: Code clearly shows what's sync vs async
2. **Easy to Test**: Can test sync operations without async noise
3. **Observable**: Can track all WaitGroups and async operations
4. **Debuggable**: Can see exactly what's happening
5. **Maintainable**: Clear separation of concerns

## Migration Strategy

1. Start with new code using new architecture
2. Gradually migrate existing code
3. Keep old code working during migration
4. Add tests for new architecture
5. Remove old code once migration complete
