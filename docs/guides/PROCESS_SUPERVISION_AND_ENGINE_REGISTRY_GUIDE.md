# Process Supervision, Engine Registry & Metrics Extractor Subsystem Architectural & Operational Guide

## 1. Executive Summary

The **Process Supervision, Engine Registry & Metrics Extractor Subsystem** (`pkg/supervision`) provides structured telemetry extraction, contrastive retry prompt formatting, negative hypothesis boundary synthesis, and canonical exemplar catalog resolution across the **ZQK Knowledge Kernel**.

Governed by core system policy **POL-DEFAULT-142277d3**, autonomous agent swarms and task runners must never engage in cyclical retry thrashing or blind, unguided iterations upon task failure. The subsystem transforms raw terminal failures (stderr/stdout streams, non-zero exit codes, panic dumps, and compiler errors) into actionable, localized telemetry wrapped in a **Supervision Envelope**. This envelope illuminates the failure delta, invalidates failed execution hypotheses, injects idiomatic design patterns, and constrains execution scope to narrow sub-gates before re-attempting full verification suites.

The package achieves **100.0% statement test coverage**, strictly exceeding the mandatory 85.0% static floor invariant, and guarantees thread-safe, concurrent execution across high-throughput agent swarms.

---

## 2. Architectural Overview & Core Principles

Autonomous AI software engineering agents operating in large codebases often fail due to cyclical retry loops—repeatedly applying the same failing patch, ignoring root-cause compiler warnings, or thrashing against full test suites without localized reproduction. 

The `pkg/supervision` subsystem solves this by implementing **Adaptive Task Supervision** structured around four complementary pillars:

```
+-----------------------------------------------------------------------------------+
|                           Failed Task Execution Event                             |
|               (Raw stderr, stdout, exit code, phase, duration)                    |
+-----------------------------------------------------------------------------------+
                                         |
                                         v
+-----------------------------------------------------------------------------------+
|             Telemetry & Metrics Extractor (pkg/supervision/extractor.go)          |
|      1. ExtractFailureAnchor: Pinpoints file:line:col or test failure symbol      |
|      2. ExtractFocusedDiagnostic: Strips build noise, preserves error context     |
|      3. BuildFailureDiagnostic: Constructs structured FailureDiagnostic telemetry |
+-----------------------------------------------------------------------------------+
                                         |
                                         v
+-----------------------------------------------------------------------------------+
|               Adaptive Supervision Envelope Construction (types.go)               |
+-----------------------------------------------------------------------------------+
       |                                |                             |
       v                                v                             v
[Pillar 1: Diagnostic Delta]   [Pillar 2: Negative Hypotheses] [Pillar 3: Exemplars]
  - Failing Phase & Exit Code    - Invalidated Approaches        - Canonical Patterns
  - Pinned Failure Anchor        - Forbidden Action Guardrails   - Anti-Pattern Rules
  - High-Signal Diagnostic       - Anti-Thrashing Constraints    - Idiomatic Models
       |                                |                             |
       +--------------------------------+-----------------------------+
                                         |
                                         v
               [Pillar 4: Progressive Scope Narrowing]
                 - Localized sub-gate verification command
                 - Narrowed iteration target & rationale
                                         |
                                         v
+-----------------------------------------------------------------------------------+
|         Contrastive Retry Formatter (pkg/supervision/formatter.go)                |
|             FormatContrastiveRetryPrompt(basePrompt, envelope)                    |
+-----------------------------------------------------------------------------------+
                                         |
                                         v
+-----------------------------------------------------------------------------------+
|                        Illuminated Autonomous Agent Prompt                        |
|        (Guiding targeted fix with clear constraints and reference patterns)       |
+-----------------------------------------------------------------------------------+
```

### The Four Pillars of Task Supervision

1. **Focused Diagnostic Delta:** Filters verbose build logs to isolate critical error markers (`FAIL`, `Error`, `panic:`, `undefined:`, `cannot use`), capturing adjacent context while pruning compilation banners and noise.
2. **Negative Hypotheses (Invalidated Paths):** Derives explicit forbidden actions from prior attempts, preventing the agent from repeating the same code modifications or strategies that triggered failures.
3. **Canonical Exemplars:** Injects curated, category-specific code references from an in-memory catalog, showing both idiomatic implementations and common anti-patterns to avoid.
4. **Progressive Scope Narrowing:** Directs the retry toward a tight, sub-second feedback loop (e.g., a single package test or file lint) before escalating to the full integration test suite.

---

## 3. Component Topology & Subsystem Architecture

The subsystem is organized into five modular functional components engineered for high concurrency, zero swallowed errors, and language-agnostic resilience:

```mermaid
flowchart TD
    subgraph ExecutionLayer["Execution & Verification Layer"]
        Runner["Task / Test Runner"]
    end

    subgraph SupervisionSubsystem["Process Supervision Subsystem (pkg/supervision)"]
        Runner -->|"Failure Output (stderr/stdout)"| Extractor["Telemetry Extractor (extractor.go)"]

        Extractor --> ExtAnchor["ExtractFailureAnchor()"]
        Extractor --> ExtSignal["ExtractFocusedDiagnostic()"]
        Extractor --> BuildDiag["BuildFailureDiagnostic()"]

        BuildDiag --> DiagObj["FailureDiagnostic Telemetry"]

        DiagObj --> Synthesizer["Hypothesis Synthesizer"]
        Synthesizer --> NegHypo["SynthesizeNegativeHypothesis()"]

        CatalogReq["Category Request"] --> Registry["Exemplar Registry (registry.go)"]
        Registry --> ResolveFn["Resolve(categories...)"]

        DiagObj --> Envelope["SupervisionEnvelope Container (types.go)"]
        NegHypo --> Envelope
        ResolveFn --> Envelope

        Envelope --> Formatter["Contrastive Formatter (formatter.go)"]
        Formatter --> PromptOut["FormatContrastiveRetryPrompt()"]
    end

    PromptOut --> AgentSession["Autonomous Agent Retry Session"]
```

### Component Manifest

| Component | Source File | Core Responsibility |
| :--- | :--- | :--- |
| **`FailureDiagnostic`** | `types.go` | Encapsulates structured attempt telemetry, exit codes, failure phase, primary source anchor, raw streams, and filtered diagnostics. |
| **`SupervisionPolicy`** | `types.go` | Configures attempt limits, hypothesis pruning toggles, exemplar injection flags, progressive scope thresholds, and watchdog timeouts. |
| **`ExemplarRegistry`** | `registry.go` | Thread-safe in-memory catalog mapping categories to `CanonicalExemplar` records. Provides seed defaults, case-insensitive resolution, and deduplication. |
| **`Extractor Engine`** | `extractor.go` | High-efficiency regex parsing engine that pinpoints source anchors and extracts high-signal diagnostics from noisy compiler outputs. |
| **`Contrastive Formatter`** | `formatter.go` | Assembles formatted markdown prompts illuminating failure deltas, forbidden actions, exemplars, and sub-gate execution targets. |

---

## 4. Supervision Lifecycle & Engine Lookup Sequence

The end-to-end lifecycle from task failure detection to retry prompt formulation is detailed in the sequence below:

```mermaid
sequenceDiagram
    autonumber
    actor Agent as Autonomous Agent / Worker
    participant Runner as Execution Runner
    participant Extractor as Telemetry Extractor
    participant Registry as Exemplar Registry
    participant Formatter as Contrastive Formatter

    Agent->>Runner: Execute Task Command (e.g. Test / Build)
    Runner-->>Agent: Task Failure (Exit Code != 0, Stderr / Stdout)

    Agent->>Extractor: ExtractFailureAnchor(stderr, stdout)
    Extractor-->>Agent: Return Failure Anchor (e.g., "pkg/core/handler.go:42:15")

    Agent->>Extractor: ExtractFocusedDiagnostic(stderr, stdout, exitCode, maxLines)
    Extractor-->>Agent: Return Filtered Error Excerpt (Noise Removed)

    Agent->>Extractor: BuildFailureDiagnostic(attempt, exitCode, phase, stderr, stdout, anomalyDesc)
    Extractor-->>Agent: Return FailureDiagnostic

    Agent->>Extractor: SynthesizeNegativeHypothesis(attempt, diag, triedApproach)
    Extractor-->>Agent: Return NegativeHypothesis with ForbiddenActions

    Agent->>Registry: Resolve("testing", "error_handling")
    Registry-->>Agent: Return Deduplicated Canonical Exemplars

    Agent->>Formatter: FormatContrastiveRetryPrompt(basePrompt, envelope)
    Formatter-->>Agent: Return Illuminated Markdown Retry Prompt

    Agent->>Runner: Execute Narrowed Sub-Gate Command (Progressive Scope)
    Runner-->>Agent: Verification Succeeded
```

---

## 5. State Transitions & Failing Phase Taxonomy

Every execution failure is classified into a deterministic `FailingPhase`. This prevents downstream components from applying compile-time remedies to runtime timeouts or verification discrepancies:

```mermaid
stateDiagram-v2
    [*] --> PhaseUnknown: Initial Attempt

    state ExecutionLifecycle {
        [*] --> PhaseCompile: Compiler / Build Error
        [*] --> PhaseExecution: Non-Zero Exit / Crash
        [*] --> PhaseTimeout: Watchdog Duration Exceeded
        [*] --> PhaseVerification: Unit / Integration Test Failure
        [*] --> PhaseValidation: Exit Code 0 with Semantic Invariant Failure
    }

    PhaseCompile --> Resolved: Syntax / Symbol Fixed
    PhaseExecution --> Resolved: Runtime Exception Resolved
    PhaseTimeout --> Resolved: Algorithm / Resource Optimized
    PhaseVerification --> Resolved: Test Assertions Satisfied
    PhaseValidation --> Resolved: Domain Policy Invariant Passed

    Resolved --> [*]
```

### Phase Definitions

- **`PhaseCompile`:** The build or compilation step failed due to syntax errors, missing symbols, or mismatched types.
- **`PhaseExecution`:** The target process crashed, exited abnormally, or encountered an unhandled panic during active runtime.
- **`PhaseTimeout`:** Execution exceeded the configured `ExecutionWatchdogGrace` window, requiring process group termination.
- **`PhaseVerification`:** Automated test runners reported test case assertion failures (`--- FAIL:`).
- **`PhaseValidation`:** The process terminated with exit code 0, but domain contracts or post-execution verification checks failed.
- **`PhaseUnknown`:** Unclassified failure category defaulting to generic telemetry analysis.

---

## 6. Concurrency & Thread-Safety Guarantees

The `pkg/supervision` subsystem is engineered to support high-throughput, multi-agent swarms operating concurrently:

1. **Read/Write Mutex Protection:** The `ExemplarRegistry` synchronizes all catalog modifications and lookup operations using a `sync.RWMutex`. Multiple reader goroutines can resolve patterns simultaneously without contention, while registration locks the catalog exclusively.
2. **Immutable Telemetry Instances:** Once constructed by `BuildFailureDiagnostic`, `FailureDiagnostic` and `SupervisionEnvelope` structs are passed by value or treated as read-only references, eliminating race conditions across worker boundaries.
3. **Goroutine Labeling Compliance:** All background routines and concurrent test workers utilize `pkg/goroutinelabels` (`goroutinelabels.NewGoroutine`) to preserve runtime traceability and prevent resource leaks.
4. **Deterministic Resolution:** Category queries in `Resolve()` normalize inputs by trimming whitespace and folding case (`strings.ToLower`), ensuring consistent pattern resolution across diverse agent hosts.

---

## 7. Operational Recipes & Implementation Walkthrough

### Recipe 1: Constructing a Supervision Envelope on Verification Failure

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/supervision"
)

func SuperviseFailedTask(ctx context.Context, taskID string, attempt int, stderr, stdout string, exitCode int) string {
	// 1. Build diagnostic telemetry
	diag := supervision.BuildFailureDiagnostic(
		attempt,
		exitCode,
		supervision.PhaseVerification,
		stderr,
		stdout,
		"Test assertion failed during contract verification",
	)

	// 2. Synthesize anti-thrashing negative boundaries
	negativeHypothesis := supervision.SynthesizeNegativeHypothesis(
		attempt,
		&diag,
		"Inlined mock data directly in production handler",
	)

	// 3. Resolve relevant canonical exemplars
	exemplars := supervision.DefaultExemplarRegistry().Resolve("testing", "error_handling")

	// 4. Assemble the supervision envelope
	envelope := supervision.SupervisionEnvelope{
		TaskID:             taskID,
		Attempt:            attempt + 1,
		MaxAttempts:        3,
		Diagnostic:         &diag,
		NegativeBoundaries: []supervision.NegativeHypothesis{negativeHypothesis},
		Exemplars:          exemplars,
		ProgressiveScope: &supervision.ProgressiveScope{
			SuggestedSubGateCommand: "go test -v -timeout 10s ./pkg/example/...",
			FocusedTarget:           diag.FailureAnchor,
			IterationRationale:      "Verify targeted package test before running whole-repo integration suite.",
		},
	}

	// 5. Generate illuminated retry prompt
	basePrompt := "# Task: Refactor Service Invariants"
	return supervision.FormatContrastiveRetryPrompt(basePrompt, envelope)
}
```

### Recipe 2: Registering Custom Exemplars

```go
package main

import "github.com/zqk-os/zqk/pkg/supervision"

func RegisterCustomPatterns(registry *supervision.ExemplarRegistry) {
	registry.Register("resilience", supervision.CanonicalExemplar{
		Category:        "resilience",
		Title:           "Exponential Backoff with Jitter",
		Description:     "Avoid thundering herd retry storms by applying jitter to exponential delays.",
		PatternCode:     "backoff := time.Duration(1<<attempt)*time.Millisecond + randJitter()",
		AntiPatternCode: "time.Sleep(5 * time.Second) // Static unjittered delay",
	})
}
```

---

## 8. Verification & Coverage Invariants

In accordance with kernel quality standards and **POL-DEFAULT-142277d3**, `pkg/supervision` adheres to the following verification invariants:

- **Static Statement Coverage Floor:** Minimum 85.0% statement coverage required across all routines.
- **Achieved Coverage:** **100.0%** statement test coverage (`go test -cover ./pkg/supervision`).
- **Zero Swallowed Errors:** All error returns and diagnostic extractions are explicitly validated without blank assignments (`_ = ...`).
- **Hermetic Unit Testing:** All test suites run with strict 30-second timeouts (`-timeout 30s`) and require zero external network or filesystem dependencies.

| Verification Target | Test Suite | Proof Type | Coverage |
| :--- | :--- | :--- | :--- |
| **Source Anchor Extraction** | `supervision_coverage_elevation_cycle215_test.go` | Regex matching across Go, Rust, Python, and Windows formats | 100.0% |
| **Focused Diagnostic Filtering** | `supervision_coverage_elevation_cycle215_test.go` | Signal marker detection, noise pruning, and tail fallback | 100.0% |
| **Negative Hypothesis Synthesis**| `supervision_coverage_elevation_cycle215_test.go` | Approach preservation, exit code guardrails, and anchor checks | 100.0% |
| **Exemplar Registry Operations** | `supervision_coverage_elevation_cycle215_test.go` | Case-folding, deduplication, and concurrent reader/writer safety | 100.0% |
| **Contrastive Prompt Formatter** | `supervision_coverage_elevation_cycle215_test.go` | Complete vs minimal envelope formatting and markdown structure | 100.0% |

---

## 9. Related Documentation & Subsystem Links

- **[Diagnostics, Auto-Remedies & Self-Healing Guide](./DIAGNOSTICS_SELF_HEALING_AND_REMEDY_GUIDE.md)**: 4-layer health checks and automated system remediation.
- **[Error Formatting, Actionable Diagnostics & Poison Pill Protection Guide](./ERROR_FORMATTING_AND_DIAGNOSTICS_GUIDE.md)**: Structured error formatting and poison pill defense.
- **[Goroutine Identity & Concurrency Labels Subsystem Guide](./GOROUTINE_LABELS_SUBSYSTEM_GUIDE.md)**: Profiler labeling and worker pool governance.
- **[Task Decision Envelope & Ingestion Subsystem Guide](./TASK_DECISION_ENVELOPE_AND_INGESTION_GUIDE.md)**: Boundary contracts for autonomous agent decisions.
