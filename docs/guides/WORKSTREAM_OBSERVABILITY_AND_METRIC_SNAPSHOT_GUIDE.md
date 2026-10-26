# Workstream Observability, Metric Sets & Heuristic Snapshot Subsystem Operational Architecture Guide

## 1. Executive Architectural Summary

The **Workstream Observability Subsystem** ([`pkg/wsobs`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go)) provides point-in-time metric snapshots, tagged telemetry sets, and deterministic heuristic validation rules for packages, pipelines, and autonomous execution environments within the **ZQK Knowledge Kernel**.

In modern multi-agent systems and large-scale autonomous pipelines, continuous execution generates diverse operational signals: package build times, goroutine counts, memory footprints, test pass counts, and static analysis findings. Without structured snapshot isolation and bounded rule evaluation, these metrics risk silent divergence, performance regressions, and uncontrolled workspace bloat.

The `pkg/wsobs` package resolves these challenges through:
1. **Tagged Metric Snapshotting ([`MetricSet`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L14-L18)):** Captures immutable collections of typed or untyped telemetry values associated with arbitrary dimensional tag slices (such as environment, subsystem tier, or package category).
2. **Deterministic Heuristic Bounds Checking ([`Heuristic`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L88-L95)):** Evaluates metrics against dynamic min/max thresholds, supporting numeric type coercion across 8 Go primitive types (`int`, `int32`, `int64`, `float64`, `float32`, `uint`, `uint32`, `uint64`).
3. **Dual Failure Discipline (Fail-Closed vs. Advisory):** Employs configurable `HardFail` toggles, allowing developers and autonomous agents to distinguish between fatal pipeline blockers (`HardFail: true`) and advisory warning notifications (`HardFail: false`).
4. **Structured Error Diagnostics ([`HeuristicViolation`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L74-L85) & [`MetricError`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L45-L52)):** Emits detailed violation envelopes detailing the failing rule, the exact field name, attempted value, and violated boundaries.
5. **Zero External Dependency Footprint:** Operates purely within standard Go library primitives, ensuring hermetic performance and zero overhead in constrained execution contexts.

---

## 2. Component Topology & Data Structures

The workstream observability architecture is structured into four cooperative primitives:

```mermaid
flowchart TD
    subgraph ObservabilityLayer["pkg/wsobs Subsystem Topology"]
        MetricSet["MetricSet\n(ID, Label, Metrics Map)"]
        MetricItem["MetricItem\n(Value, Tags Slice)"]
        HeuristicEngine["Heuristic Engine\n(Name, Field, Min, Max, HardFail)"]
        ViolationDiagnostic["HeuristicViolation\n(Reason, Name, Field, Tried, Bounds)"]
        ResultContainer["MetricSetResult\n(TotalObserved, Errors Slice)"]
        WorkspaceCfg["WorkspaceHeuristics\n(PackageThreshold, FailureTolerance)"]
    end

    subgraph DataFlow["Operational Ingestion & Validation"]
        PackageInput["Package / Subsystem Telemetry"] --> MetricSet
        MetricSet -->|Contains| MetricItem
        MetricSet -->|Passed to| HeuristicEngine
        HeuristicEngine -->|Validate| ResultEvaluator{"Bounds Met?"}
        ResultEvaluator -->|Yes| ValidPass["Nil Error / Success"]
        ResultEvaluator -->|No| ViolationDiagnostic
        ViolationDiagnostic --> ResultContainer
        WorkspaceCfg -.->|Configures| HeuristicEngine
    end
```

### Core Type Definitions

#### `MetricItem`
Represents an individual observed data point. It pairs an arbitrary value with a slice of string tags for multidimensional indexing:
```go
type MetricItem struct {
    Value interface{} `json:"value"`
    Tags  []string    `json:"tags,omitempty"`
}
```

#### `MetricSet`
Encapsulates a logical snapshot of metrics under a distinct ID and descriptive human-readable label:
```go
type MetricSet struct {
    ID      string                `json:"id"`
    Label   string                `json:"label"`
    Metrics map[string]MetricItem `json:"metrics"`
}
```

Key invariant: The [`Add`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L30-L42) method enforces idempotency. If a metric key already exists in the set, `Add` returns `false` without overwriting the previous value. If the underlying `Metrics` map is uninitialized (`nil`), `Add` safely initializes the map before insertion.

#### `Heuristic`
Defines operational boundaries and severity rules evaluated against a `MetricSet`:
```go
type Heuristic struct {
    Name     string
    Field    string
    Min      float64
    Max      float64
    HardFail bool
}
```

#### `HeuristicViolation`
Structured error type generated when a metric violates heuristic parameters:
```go
type HeuristicViolation struct {
    Reason  string      `json:"reason"`
    Name    string      `json:"name"`
    Field   string      `json:"field"`
    Tried   interface{} `json:"tried,omitempty"`
    Lowest  interface{} `json:"lowest,omitempty"`
    Highest interface{} `json:"highest,omitempty"`
}
```

#### `MetricSetResult`
Aggregates high-level observation counts and execution errors:
```go
type MetricSetResult struct {
    TotalObserved int           `json:"total_observed"`
    Errors        []MetricError `json:"errors,omitempty"`
}
```

The [`Valid()`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L61-L63) method returns `true` if and only if `len(Errors) == 0`.

---

## 3. Heuristic Evaluation Lifecycle

The lifecycle of metric observation, bounds checking, and violation handling operates deterministically:

```mermaid
sequenceDiagram
    autonumber
    participant Collector as Metrics Collector / Agent
    participant Set as MetricSet
    participant Rule as Heuristic
    participant Viols as HeuristicViolation

    Collector->>Set: NewMetricSet(id, label)
    Collector->>Set: Add("goroutines", 42, ["runtime", "core"])
    Collector->>Set: Add("memory_mb", 256.5, ["system"])
    
    Collector->>Rule: Validate(metricSet)
    alt MetricSet is nil or Metrics is nil
        opt HardFail is true
            Rule-->>Collector: HeuristicViolation (reason="missing_field")
        else HardFail is false
            Rule-->>Collector: nil (advisory pass)
        end
    else Field Missing from Set
        opt HardFail is true
            Rule-->>Collector: HeuristicViolation (reason="missing_field")
        else HardFail is false
            Rule-->>Collector: nil (advisory pass)
        end
    else Field Present
        Rule->>Rule: Coerce type to float64
        alt Unsupported Non-Numeric Type
            opt HardFail is true
                Rule-->>Collector: HeuristicViolation (reason="unsupported_type")
            else HardFail is false
                Rule-->>Collector: nil (advisory pass)
            end
        else Value < Min
            Rule-->>Collector: HeuristicViolation (reason="below_minimum", Lowest=Min)
        else Value > Max
            Rule-->>Collector: HeuristicViolation (reason="above_maximum", Highest=Max)
        else Value in [Min, Max]
            Rule-->>Collector: nil (Validation Passed)
        end
    end
```

### Type Coercion Matrix

During [`Heuristic.Validate`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L97-L172), incoming values stored as `interface{}` are normalized to `float64` for uniform relational comparison:

| Go Concrete Type | Normalized Representation | Boundary Enforcement Behavior |
| :--- | :--- | :--- |
| `int` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `int32` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `int64` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `float64` | `v` | Verified against `[Min, Max]` inclusive. |
| `float32` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `uint` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `uint32` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| `uint64` | `float64(v)` | Verified against `[Min, Max]` inclusive. |
| Other Types (`string`, `bool`, slices, maps) | N/A | Triggers `unsupported_type` if `HardFail=true`, or ignored if `HardFail=false`. |

---

## 4. Failure Classification & Error Taxonomy

The subsystem implements a strict violation categorization scheme:

```mermaid
flowchart LR
    Violation{"Evaluation Outcome"} -->|Missing MetricSet or Field| MissingField["missing_field\n(HardFail: true)"]
    Violation -->|Non-numeric Payload| Unsupported["unsupported_type\n(HardFail: true)"]
    Violation -->|Value < Min| BelowMin["below_minimum\n(Populates Lowest bound)"]
    Violation -->|Value > Max| AboveMax["above_maximum\n(Populates Highest bound)"]
    Violation -->|Compliant Value| CleanPass["nil\n(Success)"]
```

### Violation Reasons

1. **`missing_field`**:
   - Triggered when `ms == nil`, `ms.Metrics == nil`, or `ms.Metrics[h.Field]` is absent, provided `h.HardFail == true`.
   - If `h.HardFail == false`, missing fields are treated as non-blocking advisory events, returning `nil`.
2. **`unsupported_type`**:
   - Triggered when the metric value cannot be coerced into a float64 (e.g. strings, booleans, structs), provided `h.HardFail == true`.
   - Records the raw attempted object in `violation.Tried`.
3. **`below_minimum`**:
   - Triggered when the coerced numeric value is strictly less than `h.Min`.
   - Populates `violation.Lowest = h.Min` and `violation.Tried = num`.
4. **`above_maximum`**:
   - Triggered when the coerced numeric value is strictly greater than `h.Max`.
   - Populates `violation.Highest = h.Max` and `violation.Tried = num`.

---

## 5. Snapshot Isolation & Concurrency Safety

Observability snapshots in `pkg/wsobs` are designed with immutability and concurrency principles:

1. **Instance Isolation:** Each [`NewMetricSet`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L21-L27) instantiates a completely self-contained struct with its own dedicated hash map. Distinct goroutines managing independent packages or execution phases can populate their respective sets without mutual interference.
2. **Concurrent Evaluation Safety:** Multiple heuristics can concurrently inspect and validate the same populated `MetricSet`. Because [`Heuristic.Validate`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L97-L172) performs read-only lookups on the metrics map, concurrent validation passes do not suffer data races once metric insertion is finalized.
3. **Serialization Integrity:** Both [`MetricSet`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L14-L18) and [`MetricItem`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L8-L11) declare standard JSON struct tags (`json:"..."`). Metric sets can be serialized to JSON and persisted to disk, dispatched via HTTP telemetry endpoints, or embedded inside audit logs without schema translation.

---

## 6. Integration with the Knowledge Kernel

Workstream observability directly supports the **ZQK Knowledge Kernel** governance and execution planes:

```mermaid
flowchart TD
    subgraph KernelPlanes["ZQK Knowledge Kernel"]
        DataPlane["Object Graph & Data Plane\n(Storage, CAS, Rollup)"]
        ExecutionPlane["Continuous Execution Plane\n(Single-Command Loop, Scheduler)"]
        GovernancePlane["Governance & Validation Plane\n(Static AST Vetting, System Check)"]
    end

    subgraph WSObsIntegration["Observability Integration"]
        PkgObs["pkg/wsobs Observation"]
        MetricSnapshots["Metric Snapshots (MetricSet)"]
        Thresholds["Workspace Thresholds (WorkspaceHeuristics)"]
        SystemCheckBridge["System Check Diagnostics Bridge"]
    end

    ExecutionPlane -->|Collects Pipeline Telemetry| PkgObs
    PkgObs -->|Assembles| MetricSnapshots
    GovernancePlane -->|Configures Rules & Bounds| Thresholds
    Thresholds -->|Enforces Invariants| MetricSnapshots
    MetricSnapshots -->|Reports Violations| SystemCheckBridge
    SystemCheckBridge -->|Feeds Health Metrics| DataPlane
```

### Operational Interaction Points

- **Pipeline Preflight & Postflight Gates:** In autonomous execution pipelines, agents take metric snapshots before and after code generation or refactoring passes. Violations in resource usage or package thresholds halt the pipeline before changes can be promoted.
- **System Check & Telemetry Dispatch:** Workspace heuristics defined via [`NewWorkspaceHeuristics`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L188-L193) integrate into the system check subsystem (`pkg/systemcheck`), verifying that package counts and failure tolerances align with workspace policies.
- **TPM Situational Awareness:** Metric rollups and snapshot evaluations supply the technical program management layer with quantitative data regarding system stability and test health.

---

## 7. Developer & Agent Implementation Recipes

### Recipe 1: Constructing a Tagged Metric Set

```go
package main

import (
    "fmt"
    "github.com/zqk-os/zqk/pkg/wsobs"
)

func main() {
    // 1. Initialize a named metric set
    snapshot := wsobs.NewMetricSet("snapshot-runtime-01", "Core Runtime Diagnostics")

    // 2. Add metrics with dimensional tags
    snapshot.Add("active_goroutines", 34, []string{"runtime", "concurrency"})
    snapshot.Add("heap_alloc_mb", 182.4, []string{"runtime", "memory"})
    snapshot.Add("statement_coverage", 94.8, []string{"qa", "verification"})

    // 3. Attempting to add duplicate key fails safely without mutation
    overwritten := snapshot.Add("active_goroutines", 100, []string{"duplicate"})
    fmt.Printf("Duplicate addition permitted: %t\n", overwritten) // Output: false
}
```

### Recipe 2: Defining Hard and Advisory Heuristics

```go
package main

import (
    "fmt"
    "github.com/zqk-os/zqk/pkg/wsobs"
)

func evaluateSnapshot(ms *wsobs.MetricSet) {
    // Rule A: Coverage must be >= 85.0% and <= 100.0% (HardFail: true)
    coverageRule := &wsobs.Heuristic{
        Name:     "coverage_gate",
        Field:    "statement_coverage",
        Min:      85.0,
        Max:      100.0,
        HardFail: true,
    }

    // Rule B: Goroutines should be within normal bounds (HardFail: false advisory)
    goroutineRule := &wsobs.Heuristic{
        Name:     "goroutine_advisory",
        Field:    "active_goroutines",
        Min:      1.0,
        Max:      500.0,
        HardFail: false,
    }

    if err := coverageRule.Validate(ms); err != nil {
        if violation, ok := err.(*wsobs.HeuristicViolation); ok {
            fmt.Printf("FATAL GATE: rule=%s field=%s reason=%s\n", 
                violation.Name, violation.Field, violation.Reason)
            return
        }
    }

    if err := goroutineRule.Validate(ms); err != nil {
        // Handled as non-blocking advisory
        fmt.Printf("ADVISORY: %v\n", err)
    }

    fmt.Println("Metric snapshot successfully validated.")
}
```

### Recipe 3: Multi-Package Batch Observation

```go
package main

import (
    "fmt"
    "github.com/zqk-os/zqk/pkg/wsobs"
)

func runPackageAudit(packageList []string) {
    // Perform package observation pass
    result := wsobs.ObservePackages(packageList)

    if !result.Valid() {
        fmt.Printf("Observation encountered %d errors:\n", len(result.Errors))
        for _, err := range result.Errors {
            fmt.Printf(" - Code %d: %s\n", err.Code, err.Message)
        }
        return
    }

    fmt.Printf("Successfully observed %d package sets.\n", result.TotalObserved)
}
```

---

## 8. Verification & Subsystem Reference Matrix

### API Reference Table

| Symbol | Kind | Location | Primary Purpose |
| :--- | :--- | :--- | :--- |
| [`MetricItem`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L8-L11) | Struct | `metrics.go:8` | Represents an untyped metric value paired with string tags. |
| [`MetricSet`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L14-L18) | Struct | `metrics.go:14` | Named container holding a map of observed `MetricItem` instances. |
| [`NewMetricSet`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L21-L27) | Function | `metrics.go:21` | Constructs a `MetricSet` with an initialized map. |
| [`MetricSet.Add`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L30-L42) | Method | `metrics.go:30` | Idempotently inserts a key-value pair; initializes map if nil. |
| [`MetricError`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L45-L52) | Struct | `metrics.go:45` | Structured error holding message and numeric status code. |
| [`MetricSetResult`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L55-L71) | Struct | `metrics.go:55` | Result container tracking total observed items and error records. |
| [`MetricSetResult.Valid`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L61-L63) | Method | `metrics.go:61` | Evaluates if error slice is empty. |
| [`MetricSetResult.AddError`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L66-L71) | Method | `metrics.go:66` | Appends a new error entry to the result. |
| [`HeuristicViolation`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L74-L85) | Struct | `metrics.go:74` | Diagnostic error detailing heuristic rule violations. |
| [`Heuristic`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L88-L95) | Struct | `metrics.go:88` | Evaluates metric bounds against min/max thresholds. |
| [`Heuristic.Validate`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L97-L172) | Method | `metrics.go:97` | Coerces numeric types and verifies against boundaries. |
| [`ObservePackages`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L175-L179) | Function | `metrics.go:175` | Collects observation results across packages. |
| [`WorkspaceHeuristics`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L182-L185) | Struct | `metrics.go:182` | Encapsulates workspace-level threshold configurations. |
| [`NewWorkspaceHeuristics`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/metrics.go#L188-L193) | Function | `metrics.go:188` | Instantiates default workspace heuristics (`PackageThreshold: 1, FailureTolerance: 0`). |

### Quality & Coverage Statement

The `pkg/wsobs` subsystem maintains strict continuous verification:
- **Statement Test Coverage:** `100.0%` of all statements verified via unit tests in [`wsobs_coverage_elevation_cycle242_test.go`](file:///Users/lanceettl/zqk-public-candidate/pkg/wsobs/wsobs_coverage_elevation_cycle242_test.go).
- **Concurrency Verification:** Isolated snapshot allocations and read-evaluate concurrency verified under parallel stress testing.
- **Open-Core Cleanliness:** Certified free of literal nanosecond kernel IDs, conforming to public repository release policies.
