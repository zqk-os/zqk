# Design Patterns Library

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Comprehensive catalog of design patterns used in zqk for request/response and pub/sub interactions

## Overview

This document catalogs the design patterns used throughout zqk to solve common interaction problems. These patterns provide reusable solutions for request/response flows, pub/sub messaging, context management, and cross-cutting concerns.

## Pattern Categories

### 1. Request/Response Patterns

#### 1.1 Handler Pattern

**Problem**: Routing method calls to appropriate handlers with clean separation of concerns.

**Solution**: Use `Handler` interface with `MethodRouter` for method-based routing.

**Implementation**:
- **Location**: `pkg/mcp/handler.go`
- **Interface**: `Handler` with `Handle(ctx, method, params) (result, error)`
- **Router**: `MethodRouter` maps method names to handlers
- **Registration**: `router.RegisterFunc("method_name", handlerFunc)`

**Example**:
```go
router := NewMethodRouter()
router.RegisterFunc("tools/list", s.handleToolsList)
router.RegisterFunc("tools/call", s.handleToolsCall)
result, err := router.Handle(ctx, "tools/list", params)
```

**Benefits**:
- Clean separation between routing and business logic
- Easy to test individual handlers
- Supports default handlers for unregistered methods
- Pluggable handler architecture

**Use Cases**:
- MCP method routing (`pkg/mcp/server_handlers.go`)
- CLI command routing (via Cobra)
- API endpoint routing

---

#### 1.2 Middleware Pattern (Decorator)

**Problem**: Adding cross-cutting concerns (logging, auth, metrics) without modifying handlers.

**Solution**: Wrap handlers with middleware functions that add behavior before/after execution.

**Implementation**:
- **Location**: `pkg/mcp/handler.go`
- **Type**: `Middleware func(Handler) Handler`
- **Chaining**: `Chain(middlewares...)` combines multiple middlewares
- **Execution**: Middlewares wrap handlers, executing in reverse order

**Example**:
```go
// Create middleware
traceMiddleware := TraceMiddleware(traceWriter)

// Chain multiple middlewares
handler := Chain(
    traceMiddleware,
    authMiddleware,
    metricsMiddleware,
)(router)

// Handler is now wrapped with all middlewares
result, err := handler.Handle(ctx, method, params)
```

**Benefits**:
- Separation of concerns (logging, auth, metrics)
- Composable behavior
- No modification to core handler logic
- Easy to add/remove cross-cutting concerns

**Use Cases**:
- Request/response tracing (`TraceMiddleware`)
- Authentication/authorization
- Metrics collection
- Error handling

---

### 2. Pub/Sub Patterns

#### 2.1 Event Emitter Pattern

**Problem**: Decoupling event producers from consumers, enabling reactive workflows.

**Solution**: Use `EventEmitter` with `EventSubscriber` interface for pub/sub messaging.

**Implementation**:
- **Location**: `pkg/mcp/event_emitter.go`
- **Emitter**: `EventEmitter` manages subscriptions and emission
- **Subscriber**: `EventSubscriber` interface for event consumers
- **Types**: Type-based event filtering (`EventType`)
- **Indexing**: Fast lookup by event type

**Example**:
```go
// Create emitter
emitter := NewEventEmitter(100)

// Subscribe
subscriber := NewMCPEventSubscriber(subID, eventTypes, writeFunc, timeout)
emitter.Subscribe(subscriber)

// Emit event
event := &Event{
    Type: EventTypeLogError,
    Message: "Something went wrong",
    Fields: map[string]interface{}{"component": "storage"},
}
emitter.Emit(event) // Automatically sent to all subscribers of EventTypeLogError
```

**Benefits**:
- Loose coupling between producers and consumers
- Multiple subscribers per event type
- Automatic cleanup of inactive subscribers
- Type-based filtering
- Non-blocking event delivery

**Use Cases**:
- MCP event subscriptions (`events/subscribe`)
- Log-to-event bridging (`LoggerEventAdapter`)
- Real-time notifications to clients
- System event broadcasting

---

#### 2.2 Listener Pipeline Pattern

**Problem**: Processing context objects through multiple stages with async/sync listeners.

**Solution**: Use `ContextListenerRegistry` with state-based listener execution.

**Implementation**:
- **Location**: `pkg/context/listeners.go`, `pkg/context/pipeline.go`
- **Registry**: `ContextListenerRegistry` maps states to listeners
- **Pipeline**: `ProcessContext()` executes listeners based on context state
- **States**: `pending`, `processing`, `completed`, `failed`, `cancelled`
- **Execution**: Sync listeners first, then async listeners in parallel

**Example**:
```go
// Register listener for a state
RegisterListener(StatePending, func(ctx context.Context, obj interface{}) (interface{}, error) {
    checkCtx := obj.(*BlockingCheckContext)
    // Perform blocking check
    return nil, nil
}, ListenerConfig{
    Name: "blocking_check",
    Async: false,
    Required: true,
})

// Process context through pipeline
resultChan := ProcessContext(blockingCtx)
result := <-resultChan
```

**Benefits**:
- State-based processing
- Async/sync listener support
- Required/optional listener configuration
- Timeout support per listener
- Parallel execution for async listeners

**Use Cases**:
- Blocking check validation (`BlockingCheckContext`)
- Cache invalidation (`CacheInvalidationContext`)
- Post-operation hooks
- Event-driven workflows

---

### 3. Context Object Patterns

#### 3.1 Context Object Pattern

**Problem**: Scattered boolean checks and repeated logic for related operations.

**Solution**: Encapsulate related state and logic in context objects with computed properties.

**Implementation**:
- **Pattern**: Context objects group related checks and provide computed values
- **Compute Method**: Lazy evaluation of derived properties
- **Caching**: Computed values cached until reset

**Examples**:

**QueryContext** (`pkg/context/query_context.go`):
```go
queryCtx := NewQueryContext(storageCtx, limit, offset, groupBy, sortBy, sortAsc)
queryCtx.Compute() // Groups all pagination/grouping/sorting checks
if queryCtx.ShouldPaginate() { /* ... */ }
if queryCtx.ShouldGroup() { /* ... */ }
```

**ClientEventContext** (`pkg/mcp/client_event_context.go`):
```go
eventCtx := s.getClientEventContext()
eventCtx.RecordEvent("tools_call", map[string]interface{}{
    "tool": toolName,
    "duration": duration,
})
// Encapsulates sequenceID/clientID retrieval and mutex locks
```

**LoggingDecisionContext** (`pkg/context/logging_decision_context.go`):
```go
decisionCtx := NewLoggingDecisionContext().
    WithLoggingContext(loggingCtx).
    WithSecurityContext(secCtx).
    WithComponent("mcp_server")
logger := GetLoggerFromDecisionContext(decisionCtx, projectRoot)
// Makes context-aware logging decisions
```

**Benefits**:
- DRY: Eliminates repeated boolean checks
- Encapsulation: Groups related logic
- Computed properties: Derived values calculated once
- Maintainability: Changes centralized in one place
- Readability: Clear intent through method names

**Use Cases**:
- Query pagination/grouping/sorting (`QueryContext`)
- Event recording (`ClientEventContext`)
- Logging decisions (`LoggingDecisionContext`)
- Blocking checks (`BlockingCheckContext`)
- Cache invalidation (`CacheInvalidationContext`)

---

### 4. Builder Patterns

#### 4.1 Fluent Builder Pattern

**Problem**: Constructing complex objects with many optional parameters.

**Solution**: Use fluent builder API with method chaining.

**Implementation**:
- **Location**: `internal/cli/context/builder.go`, `pkg/context/chain_builder.go`
- **Pattern**: Builder methods return `*Builder` for chaining
- **Build**: Final `Build()` method creates the object

**Example**:
```go
// ContextBuilderPattern (fluent API)
ctx, err := clictx.NewContextBuilderPattern().
    WithSystemDefaults(systemDefaults).
    WithUserConfig(userConfig).
    WithProjectConfig(projectRoot, projectConfig).
    WithCommandFlags(commandFlags).
    Build()

// ChainBuilder
chain, errors := pkgctx.NewChainBuilder().
    AddContext(systemCtx).
    AddContext(userCtx).
    AddContext(projectCtx).
    Build()
```

**Benefits**:
- Readable: Clear intent through method names
- Flexible: Optional parameters via method chaining
- Type-safe: Compile-time checking
- Fluent: Natural language-like API

**Use Cases**:
- Context building (`ContextBuilder`, `ChainBuilder`)
- Complex object construction
- Configuration assembly

---

#### 4.2 Strategy Pattern

**Problem**: Selecting algorithms or behaviors at runtime based on configuration or context.

**Solution**: Define strategy interface, implement multiple strategies, select at runtime.

**Implementation**:
- **Location**: `pkg/storage/bucketing_config.go`, `pkg/mcp/server_handlers.go`
- **Interface**: Strategy interface defines contract
- **Implementations**: Multiple concrete strategies
- **Selection**: Runtime selection based on config/context

**Example**:
```go
// BucketingStrategy
type BucketingStrategy string

const (
    BucketingStrategyNone BucketingStrategy = "none"
    BucketingStrategyChronologicalMonthly BucketingStrategy = "chronological_monthly"
    BucketingStrategyCategorical BucketingStrategy = "categorical"
)

// Strategy selection
switch config.Strategy {
case BucketingStrategyChronologicalMonthly:
    bucket = fmt.Sprintf("%s/%s", basePath, time.Now().Format("2006-01"))
case BucketingStrategyCategorical:
    bucket = fmt.Sprintf("%s/%s", basePath, category)
}
```

**Benefits**:
- Runtime algorithm selection
- Easy to add new strategies
- Clean separation of strategy logic
- Testable: Each strategy independently testable

**Use Cases**:
- Storage bucketing strategies (`BucketingStrategy`)
- Authentication strategies (`loadEnabledAuthStrategies`)
- Kind discovery strategies (`discoverKindForDirectory`)
- Processing modes (sequential, hierarchical, hybrid)

---

### 5. Decorator/Wrapper Patterns

#### 5.1 Adapter Pattern

**Problem**: Bridging incompatible interfaces or systems.

**Solution**: Create adapter that translates between interfaces.

**Implementation**:
- **Location**: `pkg/mcp/logger_event_adapter.go`, `pkg/mcp/cli_bridge.go`
- **Adapter**: Wraps one interface to match another
- **Translation**: Converts calls/events between formats

**Example**:
```go
// LoggerEventAdapter bridges logging to events
type LoggerEventAdapter struct {
    emitter *EventEmitter
    enabled bool
}

func (a *LoggerEventAdapter) Log(entry *LogEntry) {
    if !a.enabled {
        return
    }
    event := &Event{
        Type: mapLogLevelToEventType(entry.Level),
        Message: entry.Message,
        Fields: entry.Fields,
    }
    a.emitter.Emit(event)
}
```

**Benefits**:
- Interface compatibility
- Reuse existing systems
- Gradual migration path
- Separation of concerns

**Use Cases**:
- Log-to-event bridging (`LoggerEventAdapter`)
- CLI-to-MCP bridging (`CLIBridge`)
- Storage provider abstraction

---

### 6. Request/Response Flow Patterns

#### 6.1 Request/Response Abstraction

**Problem**: Supporting multiple transport protocols (stdio, HTTP, WebSocket) with same handlers.

**Solution**: Abstract transport layer, handlers work with request/response objects.

**Implementation**:
- **Location**: `pkg/mcp/protocol.go`, `pkg/mcp/transport.go`
- **Transport**: `Transport` interface for read/write operations
- **Protocol**: `Request`/`Response` objects independent of transport
- **Format**: Message format abstraction (JSON-RPC, raw JSON)

**Example**:
```go
// Transport abstraction
type Transport interface {
    ReadMessage(reader io.Reader) ([]byte, *MessageFormat, error)
    WriteMessage(writer io.Writer, data []byte, format *MessageFormat) error
}

// Protocol objects
type Request struct {
    JSONRPC string
    ID      interface{}
    Method  string
    Params  json.RawMessage
}

// Handler works with protocol, not transport
result, err := handler.Handle(ctx, req.Method, req.Params)
resp := NewResponse(req.ID, result, err)
data, _ := resp.Marshal()
transport.WriteMessage(writer, data, format)
```

**Benefits**:
- Transport independence
- Protocol abstraction
- Easy to add new transports
- Testable: Mock transport for testing

**Use Cases**:
- MCP server (stdio transport)
- Future: HTTP, WebSocket transports
- Protocol version negotiation

---

## Pattern Interaction

### Common Combinations

1. **Handler + Middleware + Event Emitter**:
   - Handler processes request
   - Middleware adds logging/metrics
   - Event emitter broadcasts events

2. **Context Object + Builder + Strategy**:
   - Builder creates context object
   - Context object uses strategy pattern internally
   - Strategy selected based on context

3. **Listener Pipeline + Event Emitter**:
   - Event emitter triggers listeners
   - Listeners process context objects
   - Pipeline coordinates async/sync execution

4. **Adapter + Handler**:
   - Adapter bridges incompatible systems
   - Handler processes adapted requests
   - Clean separation of concerns

## When to Use Which Pattern

### Use Handler Pattern When:
- Routing method/command calls
- Need clean separation of routing and logic
- Multiple handlers for different methods

### Use Middleware Pattern When:
- Adding cross-cutting concerns
- Need composable behavior
- Don't want to modify core logic

### Use Event Emitter Pattern When:
- Decoupling producers from consumers
- Multiple subscribers needed
- Real-time notifications required

### Use Context Object Pattern When:
- Scattered boolean checks
- Repeated logic for related operations
- Need computed/derived properties

### Use Builder Pattern When:
- Complex object construction
- Many optional parameters
- Want fluent API

### Use Strategy Pattern When:
- Multiple algorithms for same problem
- Runtime algorithm selection
- Easy to add new algorithms

### Use Adapter Pattern When:
- Bridging incompatible interfaces
- Integrating existing systems
- Gradual migration needed

## Anti-Patterns to Avoid

1. **Direct Mutex Access**: Use context objects instead of scattered mutex locks
2. **Manual Event Broadcasting**: Use EventEmitter instead of direct calls
3. **Hardcoded Strategy Selection**: Use configuration-driven strategy selection
4. **Handler Logic in Router**: Keep routing separate from business logic
5. **Missing Middleware**: Don't add cross-cutting concerns directly in handlers

---

### 7. Concurrency Patterns

#### 7.1 Cross-Process File Locking Pattern

**Problem**: Multiple processes modifying shared file-based resources (queues, registries, caches) simultaneously, causing race conditions, data corruption, or lost updates.

**Solution**: Use advisory file locking (`flock()`) to ensure atomic read-modify-write operations across processes.

**Implementation**:
- **Location**: `pkg/storage/file_lock.go`
- **Component**: `FileLock` provides cross-process file locking
- **Mechanism**: Uses `flock()` for advisory locking (OS-level, automatically released on process exit)
- **Operations**: `Lock()`, `TryLock()`, `LockWithTimeout()`, `Unlock()`
- **Convenience**: `WithLock()`, `WithLockTimeout()` for automatic lock/unlock

**Example**:
```go
// Create lock
fileLock, err := storagepkg.NewFileLock(lockFilePath)
if err != nil {
    return err
}
defer fileLock.Close()

// Acquire lock with timeout
if err := fileLock.LockWithTimeout(5 * time.Second); err != nil {
    return fmt.Errorf("failed to acquire lock: %w", err)
}
defer fileLock.Unlock()

// Perform atomic operation
requests, _ := readQueue()
requests = append(requests, newRequest)
writeQueue(requests)
```

**Or use convenience method**:
```go
fileLock, err := storagepkg.NewFileLock(lockFilePath)
if err != nil {
    return err
}
defer fileLock.Close()

err = fileLock.WithLockTimeout(5*time.Second, func() error {
    // Perform atomic operation
    requests, _ := readQueue()
    requests = append(requests, newRequest)
    return writeQueue(requests)
})
```

**Benefits**:
- **Prevents race conditions**: Only one process can hold lock at a time
- **Prevents data corruption**: Atomic read-modify-write operations
- **Prevents lost updates**: No overwrites of concurrent modifications
- **Automatic cleanup**: Lock released on process exit (OS-level)
- **Observable**: Can log lock acquisition/release, timeouts
- **Efficient**: Non-blocking option (`TryLock()`), timeout support

**Use Cases**:
- Job trigger queue (`pkg/scheduler/job_trigger_queue.go`)
- Hash registry updates (future refactor)
- Object ID cache operations (future refactor)
- PID file writes (future)
- Aggregation operations (future)

**Anti-Pattern**: Direct file writes without locking, causing race conditions and data corruption.

**Documentation**: [Cross-Process File Locking Pattern](./shared-resource-locking.md)

---

## References

- **Handler Pattern**: `pkg/mcp/handler.go`
- **Middleware Pattern**: `pkg/mcp/handler.go:71-82`
- **Event Emitter**: `pkg/mcp/event_emitter.go`
- **Listener Pipeline**: `pkg/context/listeners.go`, `pkg/context/pipeline.go`
- **Context Objects**: `pkg/context/query_context.go`, `pkg/mcp/client_event_context.go`
- **Builder Pattern**: `internal/cli/context/builder.go`, `pkg/context/chain_builder.go`
- **Strategy Pattern**: `pkg/storage/bucketing_config.go`
- **Adapter Pattern**: `pkg/mcp/logger_event_adapter.go`, `pkg/mcp/cli_bridge.go`
- **Cross-Process File Locking**: `pkg/storage/file_lock.go`

## Related Documentation

- [Code Evaluation and Refactoring Policy](./CODE_EVALUATION_POLICY.md): Policy for evaluating code and applying patterns
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md): High-level architecture patterns
- [Cross-Process File Locking Pattern](./shared-resource-locking.md): Detailed documentation for file locking pattern

