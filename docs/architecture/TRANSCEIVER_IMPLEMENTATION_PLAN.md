# Transceiver Router Implementation Plan

**Date:** 2026-01-05  
**Status:** Implementation Plan  
**Version:** 1.0.0  
**Related:** SCHEDULER_TRANSCEIVER_ARCHITECTURE.md

## Overview

This document outlines the implementation plan for the transceiver router architecture, showing how to evolve the current callback system into a protocol-agnostic message routing system.

## Current State Analysis

### Existing Components

1. **CallbackListenerHandler** (`pkg/scheduler/handlers_callback_listener.go`)
   - ✅ Already has routing logic (`registerRoutes`, `handleCallback`)
   - ✅ Supports runtime route configuration (`RouteHandlers`)
   - ✅ Has authentication hooks
   - ❌ HTTP-only (hardcoded)
   - ❌ Route handlers are hardcoded switch statements

2. **RunWrapperHandler** (`pkg/scheduler/handlers_run_wrapper.go`)
   - ✅ Has webhook, command, event callbacks
   - ✅ Protocol detection logic
   - ❌ Protocol logic embedded in handler
   - ❌ No routing rules (direct execution)

### Evolution Path

**Current:**
```
Job → Handler → Direct Protocol Execution
```

**Target:**
```
Job → Handler → Transceiver Router → Protocol Adapter → Destination
```

## Implementation Plan

### Phase 1: Core Transceiver Interface

**File:** `pkg/scheduler/transceiver/router.go`

```go
package transceiver

import (
    "context"
    "time"
)

// Message represents a protocol-agnostic message
type Message struct {
    EventType   string                 // Semantic event type
    Source      string                 // Source identifier
    Destination string                 // Destination identifier (optional)
    Timestamp   time.Time              // Message timestamp
    Payload     map[string]interface{} // Message payload
    Metadata    map[string]string      // Additional metadata
}

// RoutingRule defines how messages are routed
type RoutingRule struct {
    Name        string
    Description string
    Enabled     bool
    Match       MessageMatcher
    Actions     []Action
    Priority    int // Higher priority routes evaluated first
}

// MessageMatcher defines matching criteria
type MessageMatcher struct {
    EventType   string            // Exact match or pattern
    Source      string            // Source filter
    JobID       string            // Specific job
    JobCategory string            // Category filter
    JobType     string            // Job type filter
    Severity    string            // Severity filter
    Conditions  []Condition       // Complex conditions
}

// Condition defines a field-based condition
type Condition struct {
    Field    string
    Operator string // eq, ne, gt, lt, gte, lte, contains, regex
    Value    interface{}
}

// Action defines what happens when a route matches
type Action struct {
    Protocol  string                 // webhook, grpc, websocket, queue, event, command
    Endpoint  string                 // Protocol-specific endpoint
    Transform *PayloadTransform      // Optional payload transformation
    Auth      *AuthConfig            // Authentication configuration
    Retry     *RetryConfig           // Retry configuration
    Timeout   time.Duration          // Action timeout
}

// ProtocolAdapter defines the interface for protocol implementations
type ProtocolAdapter interface {
    // Send sends a message via this protocol
    Send(ctx context.Context, message Message, action Action) error
    
    // Name returns the protocol name (e.g., "webhook", "grpc")
    Name() string
    
    // Validate validates action configuration for this protocol
    Validate(action Action) error
}

// Router is the main transceiver router
type Router struct {
    rules      []RoutingRule
    adapters   map[string]ProtocolAdapter
    rulesMu    sync.RWMutex
    adaptersMu sync.RWMutex
    logger     logging.Logger
}

// NewRouter creates a new transceiver router
func NewRouter(logger logging.Logger) *Router {
    return &Router{
        rules:    make([]RoutingRule, 0),
        adapters: make(map[string]ProtocolAdapter),
        logger:   logger,
    }
}

// RegisterAdapter registers a protocol adapter
func (r *Router) RegisterAdapter(adapter ProtocolAdapter) {
    r.adaptersMu.Lock()
    defer r.adaptersMu.Unlock()
    r.adapters[adapter.Name()] = adapter
}

// LoadRules loads routing rules (from config, storage, etc.)
func (r *Router) LoadRules(rules []RoutingRule) {
    r.rulesMu.Lock()
    defer r.rulesMu.Unlock()
    r.rules = rules
    // Sort by priority (higher first)
    sort.Slice(r.rules, func(i, j int) bool {
        return r.rules[i].Priority > r.rules[j].Priority
    })
}

// Route routes a message through matching rules
func (r *Router) Route(ctx context.Context, message Message) error {
    r.rulesMu.RLock()
    rules := make([]RoutingRule, len(r.rules))
    copy(rules, r.rules)
    r.rulesMu.RUnlock()
    
    // Find matching rules
    var matchedRules []RoutingRule
    for _, rule := range rules {
        if !rule.Enabled {
            continue
        }
        if r.matches(message, rule.Match) {
            matchedRules = append(matchedRules, rule)
        }
    }
    
    // Execute actions for matched rules
    var lastErr error
    for _, rule := range matchedRules {
        for _, action := range rule.Actions {
            if err := r.executeAction(ctx, message, action); err != nil {
                r.logger.Warn("Action execution failed",
                    logging.String("rule", rule.Name),
                    logging.String("protocol", action.Protocol),
                    logging.Error(err))
                lastErr = err
                // Continue with other actions (best-effort)
            }
        }
    }
    
    return lastErr
}

// matches checks if a message matches the matcher criteria
func (r *Router) matches(message Message, matcher MessageMatcher) bool {
    // Event type matching
    if matcher.EventType != "" && message.EventType != matcher.EventType {
        return false
    }
    
    // Source matching
    if matcher.Source != "" && message.Source != matcher.Source {
        return false
    }
    
    // Job ID matching (from metadata)
    if matcher.JobID != "" {
        if jobID, ok := message.Metadata["job_id"]; !ok || jobID != matcher.JobID {
            return false
        }
    }
    
    // Condition matching
    for _, condition := range matcher.Conditions {
        if !r.evaluateCondition(message, condition) {
            return false
        }
    }
    
    return true
}

// executeAction executes an action using the appropriate adapter
func (r *Router) executeAction(ctx context.Context, message Message, action Action) error {
    r.adaptersMu.RLock()
    adapter, exists := r.adapters[action.Protocol]
    r.adaptersMu.RUnlock()
    
    if !exists {
        return fmt.Errorf("protocol adapter not found: %s", action.Protocol)
    }
    
    // Apply payload transformation if configured
    transformedMessage := message
    if action.Transform != nil {
        transformedMessage = r.transformPayload(message, action.Transform)
    }
    
    // Execute with retry if configured
    if action.Retry != nil {
        return r.executeWithRetry(ctx, adapter, transformedMessage, action)
    }
    
    // Execute with timeout
    if action.Timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, action.Timeout)
        defer cancel()
    }
    
    return adapter.Send(ctx, transformedMessage, action)
}
```

### Phase 2: Protocol Adapters

**File:** `pkg/scheduler/transceiver/adapters/http_adapter.go`

```go
package transceiver

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "time"
)

type HTTPAdapter struct {
    client *http.Client
    logger logging.Logger
}

func NewHTTPAdapter(logger logging.Logger) *HTTPAdapter {
    return &HTTPAdapter{
        client: &http.Client{
            Timeout: 5 * time.Second,
        },
        logger: logger,
    }
}

func (a *HTTPAdapter) Name() string {
    return "webhook"
}

func (a *HTTPAdapter) Validate(action Action) error {
    if action.Endpoint == "" {
        return fmt.Errorf("endpoint required for webhook protocol")
    }
    // Validate URL format
    return nil
}

func (a *HTTPAdapter) Send(ctx context.Context, message Message, action Action) error {
    // Marshal message to JSON
    jsonData, err := json.Marshal(message.Payload)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }
    
    // Create HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", action.Endpoint, bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "zqk-Transceiver/1.0")
    
    // Apply authentication if configured
    if action.Auth != nil {
        a.applyAuth(req, action.Auth)
    }
    
    // Execute request
    resp, err := a.client.Do(req)
    if err != nil {
        return fmt.Errorf("webhook request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        return nil
    }
    
    return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
}
```

**File:** `pkg/scheduler/transceiver/adapters/command_adapter.go`

```go
package transceiver

import (
    "context"
    "encoding/json"
    "os/exec"
    "time"
)

type CommandAdapter struct {
    logger logging.Logger
}

func NewCommandAdapter(logger logging.Logger) *CommandAdapter {
    return &CommandAdapter{logger: logger}
}

func (a *CommandAdapter) Name() string {
    return "command"
}

func (a *CommandAdapter) Validate(action Action) error {
    if action.Endpoint == "" {
        return fmt.Errorf("endpoint (command) required for command protocol")
    }
    return nil
}

func (a *CommandAdapter) Send(ctx context.Context, message Message, action Action) error {
    // Marshal message to JSON for stdin
    jsonData, err := json.Marshal(message.Payload)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }
    
    // Parse command
    parts := strings.Fields(action.Endpoint)
    if len(parts) == 0 {
        return fmt.Errorf("empty command")
    }
    
    // Create command with timeout
    cmdCtx := ctx
    if action.Timeout > 0 {
        var cancel context.CancelFunc
        cmdCtx, cancel = context.WithTimeout(ctx, action.Timeout)
        defer cancel()
    }
    
    cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
    cmd.Stdin = bytes.NewReader(jsonData)
    
    return cmd.Run()
}
```

**File:** `pkg/scheduler/transceiver/adapters/event_adapter.go`

```go
package transceiver

import (
    "context"
)

type EventAdapter struct {
    eventBus *EventBus
    logger   logging.Logger
}

func NewEventAdapter(eventBus *EventBus, logger logging.Logger) *EventAdapter {
    return &EventAdapter{
        eventBus: eventBus,
        logger:   logger,
    }
}

func (a *EventAdapter) Name() string {
    return "event"
}

func (a *EventAdapter) Validate(action Action) error {
    if action.Endpoint == "" {
        return fmt.Errorf("endpoint (event name) required for event protocol")
    }
    return nil
}

func (a *EventAdapter) Send(ctx context.Context, message Message, action Action) error {
    // Emit to internal event bus
    return a.eventBus.Emit(ctx, action.Endpoint, message.Payload)
}
```

### Phase 3: Integration with Scheduler

**File:** `pkg/scheduler/transceiver/integration.go`

```go
package transceiver

import (
    "github.com/lanceman/zqk/pkg/scheduler"
)

// IntegrateTransceiver integrates transceiver with scheduler
func IntegrateTransceiver(s *scheduler.Scheduler, router *Router) {
    // Replace callback execution in RunWrapperHandler
    // Route messages through transceiver instead of direct execution
}

// CreateMessageFromJob creates a Message from scheduler job execution
func CreateMessageFromJob(eventType string, job *scheduler.ScheduledJob, payload map[string]interface{}) Message {
    return Message{
        EventType: eventType,
        Source:    "scheduler",
        Timestamp: time.Now().UTC(),
        Payload:   payload,
        Metadata: map[string]string{
            "job_id":     job.ID,
            "job_type":   job.JobType,
            "category":   job.Category,
        },
    }
}
```

## Testing Strategy

### Unit Tests (Isolated)

```go
func TestRouter_Route(t *testing.T) {
    router := NewRouter(logger)
    
    // Register mock adapter
    mockAdapter := &MockAdapter{name: "test"}
    router.RegisterAdapter(mockAdapter)
    
    // Load routing rules
    rules := []RoutingRule{
        {
            Name: "test-route",
            Match: MessageMatcher{EventType: "test"},
            Actions: []Action{
                {Protocol: "test", Endpoint: "test-endpoint"},
            },
        },
    }
    router.LoadRules(rules)
    
    // Route message
    msg := Message{EventType: "test", Payload: map[string]interface{}{}}
    err := router.Route(context.Background(), msg)
    
    assert.NoError(t, err)
    assert.True(t, mockAdapter.SendCalled)
}
```

### Integration Tests

```go
func TestRouter_HTTPIntegration(t *testing.T) {
    // Test with real HTTP adapter
    server := httptest.NewServer(...)
    defer server.Close()
    
    router := NewRouter(logger)
    router.RegisterAdapter(NewHTTPAdapter(logger))
    
    // Test routing
    // ...
}
```

## Migration from Current System

### Step 1: Extract Routing Logic

**Current:** `CallbackListenerHandler.registerRoutes()`
**Target:** Move to `Router.LoadRules()`

### Step 2: Extract Protocol Logic

**Current:** `RunWrapperHandler.executeWebhookCallback()`
**Target:** Move to `HTTPAdapter.Send()`

### Step 3: Integrate Transceiver

**Current:** Direct callback execution
**Target:** `router.Route(message)`

### Step 4: Update Job Configuration

**Current:**
```yaml
callback_on_completion: https://api.example.com/webhook
callback_type: webhook
```

**Target:**
```yaml
outputs:
  - route: job-completion-notification
```

## Benefits Realized

### 1. Protocol Flexibility ✅
- Add gRPC: Implement `GRPCAdapter`, no job changes
- Add WebSocket: Implement `WebSocketAdapter`, no job changes
- Switch protocols: Update routing rules, no code changes

### 2. Semantic Consistency ✅
- Message structure stays constant
- Only transport changes
- Easy to test (mock adapters)

### 3. Isolation & Testing ✅
- Router tested independently
- Adapters tested independently
- Integration tests with real protocols

### 4. Client-Facing Interfaces ✅
- Jobs just reference route names
- Routing rules defined separately
- Easy to add new integrations

## Next Steps

1. **Create transceiver package structure**
2. **Implement core router interface**
3. **Migrate existing adapters (HTTP, command, event)**
4. **Add routing rule loading from config**
5. **Integrate with scheduler handlers**
6. **Add tests (unit, integration, performance)**
7. **Document routing rule schema**
8. **Create migration guide**

