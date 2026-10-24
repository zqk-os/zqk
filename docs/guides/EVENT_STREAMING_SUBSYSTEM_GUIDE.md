# Event Streaming & Action Router Subsystem (`pkg/events`)

## 1. Executive Summary & Architectural Overview

The **Event Streaming & Action Router Subsystem** (`pkg/events`) provides a high-throughput, in-memory, thread-safe publish-subscribe message router and localized policy inspection engine. It decouples the core microkernel, Content-Addressable Storage (CAS) layer, and peripheral listeners (such as state machine orchestrators and suspension handlers) without introducing heavy external message broker dependencies.

Instead of requiring expensive full-graph traversals on every state change, `pkg/events` introduces the **Three-Fold Event Dispatch Spine**:
1. **Bitmask Filter Routing**: Microsecond-latency bitwise evaluation of high-frequency events (`EventTypeObjectMutated`, `EventTypeSuspensionTriggered`).
2. **Localized Policy Inspection**: $O(1)$ evaluation of domain invariants (e.g., verifying whether an object mutation moves a goal or milestone into an invalid or blocked state).
3. **Cascading Overlay Propagation**: Deterministic re-publication of suspension and alert shapes directly into the router stream, instantly propagating shockwaves across dependent kernel controllers.

```mermaid
flowchart TD
    Storage["Object Storage / CAS"] -->|Publish Shape| Router["events.Router"]
    Router -->|RLock Evaluation| Inspectors{"Inspector Match"}
    Inspectors -->|Bitmask Match| ChannelSub["Buffered Channel Subscribers"]
    Inspectors -->|Policy Violation| PolicyInsp["PolicyInspector"]
    PolicyInsp -->|O(1) Cascade| CascadePub["Cascade Publisher"]
    CascadePub -->|Re-Publish SuspensionTriggered| Router
```

---

## 2. Core Concepts & Domain Model

### 2.1 The Event Shape (`events.Shape`)

Every mutation or lifecycle transition within the kernel emits a structured `Shape`. The `Shape` carries localized metadata sufficient for downstream inspectors to match without executing database queries:

| Field | Type | Description |
| :--- | :--- | :--- |
| `Type` | `EventType` | Bitmask enum identifying the class of event (`EventTypeSuspensionTriggered`, `EventTypeObjectMutated`). |
| `TargetID` | `string` | Target object identifier (e.g. `GOAL-DEMO-001`, `BLI-DEMO-001`). |
| `SourceID` | `string` | Actor or subsystem originating the mutation (e.g. `storage`, `scheduler`, `user-1`). |
| `Kind` | `string` | Canonical kernel object kind (`goal`, `requirement`, `backlog_item`, `milestone`). |
| `Namespace` | `string` | Partition namespace (e.g. `zqk:kernel`, `zqk:custom`). |
| `Status` | `string` | Effective status following mutation (`active`, `blocked`, `completed`, `failed`). |
| `Payload` | `any` | Optional opaque payload for context-specific consumers. |

### 2.2 Event Bitmasks (`events.EventType`)

Event types are defined as bit flags enabling set operations in a single CPU cycle:

```go
type EventType uint64

const (
    EventTypeUnknown             EventType = 0
    EventTypeSuspensionTriggered EventType = 1 << 0
    EventTypeObjectMutated       EventType = 1 << 1
)
```

---

## 3. Router Architecture & Concurrency Model

### 3.1 Multiplexed Subscription & Non-Blocking Delivery

The `Router` coordinates delivery across arbitrary numbers of concurrent consumers. To protect kernel throughput from slow or stalled subscribers, channel delivery uses non-blocking select drops:

```go
for inspector, channels := range r.subscribers {
    if inspector.Inspect(s) {
        for _, ch := range channels {
            select {
            case ch <- s:
            default:
                // Channel buffer full: drops to prevent pipeline stalling
            }
        }
    }
}
```

### 3.2 Thread-Safe Mutex Topology

- **`sync.RWMutex` (`r.mu`)**:
  - `Publish` acquires a read lock (`r.mu.RLock()`) to read `cascadeInspectors` and evaluate channel subscribers.
  - `Subscribe` and `Unsubscribe` acquire exclusive write locks (`r.mu.Lock()`).
  - Read-write lock separation guarantees high publish concurrency with zero race conditions under concurrent worker swarms.

---

## 4. Localized Policy Inspection & Cascading Overlays

### 4.1 $O(1)$ Policy Invariant Check

The `PolicyInspector` evaluates declarative rules over object kinds, namespaces, and forbidden statuses:

```go
cfg := events.PolicyInspectorConfig{
    Kind:               "milestone",
    Namespace:          "zqk:kernel",
    DisallowedStatuses: []string{objects.ObjectStatusBlocked, objects.ObjectStatusError},
}
inspector := events.NewPolicyInspector(cfg)
```

When an object transitions into one of the `DisallowedStatuses`, `PolicyInspector.Inspect(s)` detects the violation immediately.

### 4.2 Cascading Suspension Wiring

By registering a policy inspector via `router.SubscribeWithCascade(inspector, router)`, violations automatically re-emit an `EventTypeSuspensionTriggered` event into the router:

```go
router.SubscribeWithCascade(inspector, router)
```

This achieves cascading safety boundaries across distributed components without coupling domain logic to low-level pub-sub machinery.

---

## 5. Verification & The Three-Fold Proof

The integrity of `pkg/events` is guaranteed across three rigorous verification dimensions:

### 5.1 Static Floor Invariant
- **Criterion**: Statement test coverage for `pkg/events` must meet or exceed **85.0%** (achieving **100.0%** in Cycle 148).
- **Rule Enforcement**: Verified deterministically via Go test coverage instrumentation:
  ```bash
  go test -cover ./pkg/events/...
  ```

### 5.2 Operational Dynamic Proof
- **Criterion**: Event router dispatch, subscriber filter matching, cascading overlays, and non-blocking buffer drops must execute deterministically across concurrent goroutines without deadlocks or races.
- **Rule Enforcement**: Verified under Go's race detector:
  ```bash
  go test -race ./pkg/events/...
  ```

### 5.3 Negative Adversarial Boundary
- **Criterion**: Boundary conditions, channel overflow handling, unregistration of missing subscribers, zero-bitmask queries, and open-core release hygiene must pass without exceptions or prohibited studio nanos-hex identifiers.
- **Rule Enforcement**: Automated release payload scan:
  ```bash
  sh scripts/open-core/check-public-release-payload.sh .
  ```

---

## 6. Operational Best Practices & Recipes

### 6.1 Creating a Subscriber Channel with Clean Teardown

```go
router := events.NewRouter()
inspector := events.NewBitmaskInspector(events.EventTypeObjectMutated)

ch := make(chan events.Shape, 100)
router.Subscribe(inspector, ch)
defer func() {
    router.Unsubscribe(inspector, ch)
    close(ch)
}()
```

### 6.2 Monitoring Policy Violations with Cascades

```go
policyInsp := events.NewPolicyInspector(events.PolicyInspectorConfig{
    Kind:               "backlog_item",
    Namespace:          "zqk:kernel",
    DisallowedStatuses: []string{objects.ObjectStatusBlocked},
})

// Downstream listener for critical suspensions
suspCh := make(chan events.Shape, 50)
router.Subscribe(events.NewBitmaskInspector(events.EventTypeSuspensionTriggered), suspCh)

// Cascade inspector re-publishes to router
router.SubscribeWithCascade(policyInsp, router)
```
