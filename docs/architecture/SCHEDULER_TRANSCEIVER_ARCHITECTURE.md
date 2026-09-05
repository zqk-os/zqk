# Scheduler Transceiver Architecture

**Last Verified:** 2026-08-31


**Date:** 2026-01-05  
**Status:** Proposed Architecture  
**Version:** 1.0.0  
**Related:** BLI-911, Job Output Mechanisms

## Overview

A protocol-agnostic transceiver/router that handles all scheduler job communication patterns (job-to-job, external-to-internal, internal-to-external) with runtime-configurable routing rules. This decouples communication semantics from transport protocols, enabling protocol flexibility while maintaining consistent message handling.

## Core Concept

**Semantic Context (Protocol-Agnostic):**
- Message routing rules
- Event types and payloads
- Authentication/authorization
- Retry logic
- Delivery guarantees

**Transport Layer (Protocol-Specific):**
- HTTP/HTTPS (webhooks)
- gRPC (high-performance)
- WebSockets (real-time)
- Message queues (RabbitMQ, Kafka)
- Unix sockets (local)
- Named pipes (Windows)

## Architecture

### Component: Transceiver Router

```
┌─────────────────────────────────────────────────────────┐
│              Transceiver Router                         │
│  ┌──────────────────────────────────────────────────┐  │
│  │         Routing Engine (Runtime Rules)            │  │
│  │  - Load routing rules from config/storage         │  │
│  │  - Match messages to routes                       │  │
│  │  - Apply transformations                          │  │
│  │  - Handle retries, timeouts, errors               │  │
│  └──────────────────────────────────────────────────┘  │
│                          │                              │
│        ┌─────────────────┼─────────────────┐          │
│        │                 │                 │          │
│  ┌─────▼─────┐   ┌──────▼──────┐   ┌─────▼─────┐    │
│  │  HTTP     │   │   gRPC       │   │  WebSocket│    │
│  │  Adapter  │   │   Adapter    │   │  Adapter  │    │
│  └───────────┘   └──────────────┘   └───────────┘    │
│        │                 │                 │          │
│        └─────────────────┼─────────────────┘          │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │   Message Bus        │                  │
│              │   (Internal Events)   │                  │
│              └───────────────────────┘                  │
└─────────────────────────────────────────────────────────┘
```

### Routing Rules (Runtime Configuration)

```yaml
# Routing rule example
routes:
  - name: job-completion-to-slack
    match:
      event_type: scheduler_job_completed
      job_category: maintenance
    actions:
      - protocol: webhook
        endpoint: https://hooks.slack.com/services/...
        retry:
          max_attempts: 3
          backoff: exponential
      - protocol: event
        event_name: maintenance.job.completed
        internal: true

  - name: job-error-to-pagerduty
    match:
      event_type: scheduler_job_failed
      severity: high
    actions:
      - protocol: webhook
        endpoint: https://api.pagerduty.com/incidents
        auth:
          type: bearer
          token: ${PAGERDUTY_TOKEN}
        retry:
          max_attempts: 5
          backoff: exponential

  - name: job-to-job-trigger
    match:
      event_type: scheduler_job_completed
      job_id: SCH-002
    actions:
      - protocol: event
        event_name: trigger.SCH-003
        internal: true
```

## Benefits

### 1. Protocol Flexibility

**Current State:**
- Webhooks: HTTP only
- Commands: Local execution only
- Events: Placeholder only

**With Transceiver:**
- Add new protocols without changing job definitions
- Switch protocols at runtime (e.g., HTTP → gRPC for performance)
- Support multiple protocols simultaneously
- Test with mock protocols (in-memory, test adapters)

### 2. Semantic Consistency

**Message Semantics Stay Constant:**
```json
{
  "event_type": "scheduler_job_completed",
  "job_id": "SCH-002",
  "timestamp": "2026-01-05T12:00:00Z",
  "payload": { ... }
}
```

**Transport Changes:**
- HTTP: POST to URL
- gRPC: Unary RPC call
- WebSocket: Send message
- Queue: Publish to topic
- Event: Emit to bus

### 3. Isolation & Testing

**Transceiver Component:**
- Can be tested in isolation
- Mock protocol adapters for unit tests
- Integration tests with real protocols
- Performance tests with different transports

**Client-Facing Interfaces:**
- Trivial to build (just configure routing rules)
- No protocol-specific code in jobs
- Consistent API regardless of transport

### 4. Runtime Configuration

**No Code Changes:**
- Add new routes via configuration
- Change protocols without redeployment
- Enable/disable routes dynamically
- A/B test different protocols

## Integration with Current System

### Migration Path

**Phase 1: Transceiver Implementation**
```go
type TransceiverRouter struct {
    routes      []RoutingRule
    adapters    map[string]ProtocolAdapter
    messageBus  *EventBus
}

type RoutingRule struct {
    Name    string
    Match   MessageMatcher
    Actions []Action
}

type ProtocolAdapter interface {
    Send(ctx context.Context, message Message) error
    Receive(ctx context.Context, handler MessageHandler) error
}
```

**Phase 2: Replace Callback Handlers**
```go
// Old: Direct webhook/command execution
h.executeWebhookCallback(ctx, url, payload)
h.executeCommandCallback(ctx, command, payload)

// New: Route through transceiver
transceiver.Route(ctx, Message{
    EventType: "scheduler_job_completed",
    JobID: job.ID,
    Payload: payload,
})
```

**Phase 3: Job Configuration Simplification**
```yaml
# Old: Protocol-specific configuration
callback_on_completion: https://api.example.com/webhook
callback_type: webhook

# New: Semantic configuration
outputs:
  - event_type: completion
    route: job-completion-to-slack  # References routing rule
```

## Protocol Adapters

### HTTP/Webhook Adapter
```go
type HTTPAdapter struct {
    client *http.Client
}

func (a *HTTPAdapter) Send(ctx context.Context, msg Message) error {
    // Convert message to HTTP POST
    // Handle retries, timeouts
    // Return error on failure
}
```

### gRPC Adapter
```go
type GRPCAdapter struct {
    conn *grpc.ClientConn
}

func (a *GRPCAdapter) Send(ctx context.Context, msg Message) error {
    // Convert message to gRPC request
    // Higher performance than HTTP
    // Binary protocol
}
```

### WebSocket Adapter
```go
type WebSocketAdapter struct {
    conn *websocket.Conn
}

func (a *WebSocketAdapter) Send(ctx context.Context, msg Message) error {
    // Real-time bidirectional communication
    // Persistent connection
}
```

### Message Queue Adapter
```go
type QueueAdapter struct {
    producer MessageProducer
}

func (a *QueueAdapter) Send(ctx context.Context, msg Message) error {
    // Publish to queue (RabbitMQ, Kafka, etc.)
    // Guaranteed delivery
    // Decoupled systems
}
```

### Event Bus Adapter (Internal)
```go
type EventBusAdapter struct {
    bus *EventBus
}

func (a *EventBusAdapter) Send(ctx context.Context, msg Message) error {
    // Emit to internal event bus
    // In-memory, high performance
    // Multiple subscribers
}
```

## Routing Rule Schema

```yaml
routes:
  - name: unique-route-name
    description: Human-readable description
    enabled: true
    match:
      # Message matching criteria
      event_type: scheduler_job_completed | scheduler_job_failed | scheduler_job_started
      job_id: SCH-002  # Specific job
      job_category: maintenance  # Category filter
      job_type: run_wrapper  # Job type filter
      severity: high | medium | low  # Severity filter
      conditions:  # Complex conditions
        - field: duration
          operator: gt
          value: 60  # seconds
    actions:
      - protocol: webhook | grpc | websocket | queue | event
        endpoint: https://api.example.com/webhook
        transform:  # Optional payload transformation
          include_fields: [job_id, timestamp, payload]
          exclude_fields: [stderr]
          add_fields:
            source: zqk-scheduler
        auth:
          type: bearer | basic | jwt | x509
          credentials: ${ENV_VAR} | secret:key
        retry:
          max_attempts: 3
          backoff: exponential | linear | fixed
          initial_delay: 1s
          max_delay: 30s
        timeout: 5s
        priority: high | medium | low
```

## Testing Strategy

### Unit Tests (Isolated Transceiver)

```go
func TestTransceiverRouter_Route(t *testing.T) {
    // Mock protocol adapters
    mockAdapter := &MockProtocolAdapter{}
    
    router := NewTransceiverRouter()
    router.RegisterAdapter("mock", mockAdapter)
    
    // Load routing rules
    rules := []RoutingRule{
        {
            Name: "test-route",
            Match: MessageMatcher{EventType: "test"},
            Actions: []Action{
                {Protocol: "mock", Endpoint: "test-endpoint"},
            },
        },
    }
    router.LoadRules(rules)
    
    // Route message
    msg := Message{EventType: "test", Payload: map[string]interface{}{}}
    err := router.Route(context.Background(), msg)
    
    // Verify adapter was called
    assert.True(t, mockAdapter.SendCalled)
}
```

### Integration Tests (Real Protocols)

```go
func TestTransceiverRouter_HTTPIntegration(t *testing.T) {
    // Start test HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
    }))
    defer server.Close()
    
    // Configure router with real HTTP adapter
    router := NewTransceiverRouter()
    router.RegisterAdapter("webhook", NewHTTPAdapter())
    
    // Test routing
    // ...
}
```

### Performance Tests

```go
func BenchmarkTransceiverRouter_HTTP(b *testing.B) {
    // Benchmark HTTP adapter
}

func BenchmarkTransceiverRouter_gRPC(b *testing.B) {
    // Benchmark gRPC adapter (should be faster)
}

func BenchmarkTransceiverRouter_EventBus(b *testing.B) {
    // Benchmark event bus (should be fastest)
}
```

## Client-Facing Interfaces

### Simplified Job Configuration

**Before (Protocol-Specific):**
```yaml
callback_on_completion: https://api.example.com/webhook
callback_on_error: /usr/local/bin/alert.sh
callback_type: webhook
```

**After (Semantic):**
```yaml
outputs:
  - event_type: completion
    route: job-completion-notification
  - event_type: error
    route: job-error-alert
```

**Routing rules defined separately:**
```yaml
# routing-rules.yaml
routes:
  - name: job-completion-notification
    match:
      event_type: scheduler_job_completed
    actions:
      - protocol: webhook
        endpoint: https://api.example.com/webhook
  - name: job-error-alert
    match:
      event_type: scheduler_job_failed
    actions:
      - protocol: command
        endpoint: /usr/local/bin/alert.sh
```

### Benefits

1. **Separation of Concerns**: Job definition vs. routing configuration
2. **Reusability**: Same route used by multiple jobs
3. **Flexibility**: Change routing without modifying jobs
4. **Testing**: Test routing rules independently

## Implementation Phases

### Phase 1: Core Transceiver (MVP)

**Scope:**
- Basic routing engine
- HTTP adapter (existing webhook logic)
- Command adapter (existing command logic)
- Event adapter (internal event bus)
- Routing rule loading from config

**Deliverables:**
- `pkg/scheduler/transceiver/` package
- Routing rule schema
- Protocol adapter interface
- Basic adapters (HTTP, command, event)

### Phase 2: Enhanced Protocols

**Scope:**
- gRPC adapter
- WebSocket adapter
- Message queue adapters (RabbitMQ, Kafka)
- Advanced routing (conditions, transformations)

**Deliverables:**
- Additional protocol adapters
- Routing rule enhancements
- Performance optimizations

### Phase 3: Runtime Management

**Scope:**
- Dynamic rule loading/reloading
- Rule validation
- Route metrics and monitoring
- Route A/B testing

**Deliverables:**
- Runtime rule management API
- Metrics collection
- Monitoring dashboard

### Phase 4: Migration

**Scope:**
- Migrate existing callbacks to routing rules
- Update job configurations
- Deprecate old callback fields

**Deliverables:**
- Migration scripts
- Updated job specs
- Backward compatibility layer

## Example: Job-to-Job Messaging

### Current (Manual)

```yaml
# SCH-002: Aggregation job
id: SCH-002
job_type: audit_event_aggregation

# SCH-003: Cleanup job (triggered after aggregation)
id: SCH-003
job_type: cleanup
trigger_type: event
event_filter: scheduler_job_completed
# Manually check if SCH-002 completed
```

### With Transceiver (Automatic)

```yaml
# Routing rule
routes:
  - name: aggregation-triggers-cleanup
    match:
      event_type: scheduler_job_completed
      job_id: SCH-002
    actions:
      - protocol: event
        event_name: trigger.SCH-003
        internal: true
```

**Benefits:**
- No manual event filter configuration
- Semantic routing (aggregation → cleanup)
- Easy to add more downstream jobs
- Testable in isolation

## Example: External-to-Internal

### Current (Callback Listener)

```yaml
# SCH-006: Callback listener
id: SCH-006
job_type: callback_listener
listener_port: 8080
listener_path: /callbacks
auth_type: jwt
```

### With Transceiver

```yaml
# Routing rule for incoming webhooks
routes:
  - name: external-webhook-to-job
    match:
      source: external
      path: /callbacks/trigger
    actions:
      - protocol: event
        event_name: trigger.${payload.job_id}
        internal: true
```

**Benefits:**
- Unified routing for all communication
- Protocol-agnostic (webhook, gRPC, WebSocket)
- Consistent authentication/authorization
- Easy to add new external integrations

## Performance Characteristics

### Protocol Comparison

| Protocol | Latency | Throughput | Reliability | Complexity |
|----------|---------|-----------|-------------|------------|
| HTTP | 50-500ms | High | Medium | Low |
| gRPC | 10-100ms | Very High | High | Medium |
| WebSocket | 1-10ms | Very High | High | Medium |
| Queue | 100-1000ms | Very High | Very High | High |
| Event Bus | < 1ms | Highest | High | Low |

### Transceiver Overhead

- **Routing Logic**: < 1ms (in-memory matching)
- **Adapter Selection**: < 0.1ms (map lookup)
- **Total Overhead**: ~1-2ms (negligible compared to network)

## Security Model

### Authentication/Authorization

**Per-Route Configuration:**
```yaml
routes:
  - name: secure-webhook
    match: ...
    actions:
      - protocol: webhook
        endpoint: https://api.example.com/webhook
        auth:
          type: jwt
          credentials: secret:jwt-secret
          validate: true
```

**Unified Security:**
- All protocols use same auth model
- Credentials stored securely (not in routing rules)
- Per-route permissions
- Audit logging for all routes

## Monitoring & Observability

### Metrics Per Route

- Message count (sent, received, failed)
- Latency (p50, p95, p99)
- Error rate
- Retry count
- Protocol-specific metrics

### Tracing

- End-to-end message tracing
- Route decision logging
- Protocol adapter performance
- Error propagation

## Migration Strategy

### Backward Compatibility

**Phase 1: Coexistence**
- Transceiver handles new routing rules
- Old callback fields still work
- Jobs can use either approach

**Phase 2: Deprecation**
- Warn when using old callback fields
- Provide migration tool
- Document migration path

**Phase 3: Removal**
- Remove old callback fields
- All communication via transceiver
- Simplified job configuration

## Implementation

**See:** [Transceiver Implementation Plan](./TRANSCEIVER_IMPLEMENTATION_PLAN.md) for detailed implementation steps and code structure.

## Related Documentation

- [Job Output Mechanisms](../system-health/JOB_OUTPUT_MECHANISMS.md)
- [Job Output Decision Guide](../system-health/JOB_OUTPUT_DECISION_GUIDE.md)
- [Scheduler Architecture](./scheduler-coordination-kernel.md)
- [Transceiver Implementation Plan](./TRANSCEIVER_IMPLEMENTATION_PLAN.md)

