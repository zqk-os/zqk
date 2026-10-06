# Core Kernel Console UX & Responsive Feedback Audit

**Audit Token:** `BLI-1791318796173626000-deb88239`  
**Milestone:** `MIL-1791318896373750000-288b4731` (Core Kernel Hardening & Comprehensive Audit Constellation)  
**Priority Plan:** `PRI-1791318593529202000-87cbab28`  
**Requirement Reference:** `REQ-1791318795739874000-3d002973`  
**Persona:** `PER-COMMUNITY-CLI-ERGONOMICS-AUDITOR` (CLI Ergonomics & Feedback Auditor)

---

## 1. Executive Summary

A critical dimension of developer ergonomics and autonomous agent usability is **responsive progress feedback**. When operators or orchestrators initiate multi-step, resource-intensive pipelines (such as system integrity checks, graph mutations, or batch validations), silent execution intervals leave users uncertain whether the process is progressing, stalled, or deadlocked.

This audit evaluated console feedback ergonomics across the ZQK CLI, diagnosed feedback gaps in asynchronous execution paths, and established a unified console progress architecture.

Key Accomplishments:
1. **Interactive Step Progress (`pkg/cli/ux`)**: Implemented `StepTracker` providing real-time stage updates, active spinner animation, and millisecond-accurate elapsed duration formatting `[0.45s]` upon stage completion or failure.
2. **Seamless Async Pipeline Integration (`pkg/cliapp`)**: Linked `ux.StepTracker` into `RunWithAsyncProgress`, instantly empowering over 50 CLI subcommands with interactive visual feedback without requiring per-command manual rewiring.
3. **Strict Non-Interactive & Structured Log Suppression**: Implemented robust suppression rules that disable ANSI spinners and escape codes whenever output is redirected to pipes, executed within CI environments (`CI=true`), invoked under automated profiles (`ai-agent`, `mcp`), or formatted as structured data (`--format json`, `--format yaml`).

```mermaid
flowchart TD
    subgraph CLI Execution
        CMD[Command Invocations] --> BIND[cliapp.BindAsyncProgress]
    end

    subgraph Environment & Profile Detection
        BIND --> DETECT{Evaluate UX Context}
        DETECT -->|TTY + Human + Table/Text| INTERACTIVE[Interactive Mode]
        DETECT -->|CI / Pipe / JSON / Agent Profile| HEADLESS[Structured Headless Mode]
    end

    subgraph Console Feedback Layer
        INTERACTIVE --> SPINNER[Live Animated Spinner + Elapsed Timer]
        SPINNER --> COMPLETE[✓ Step Completed Duration ms]
        HEADLESS --> SILENT[Zero ANSI Codes / Pure JSON/YAML Stream]
    end
```

---

## 2. Problem Diagnosis & Ergonomics Evaluation

Prior to this hardening cycle:
- `pkg/cli/ux/spinner.go` contained a basic `StartSpinner`/`StopSpinner` wrapper around `briandowns/spinner`, but lacked elapsed duration tracking, step lifecycle management, and integration with command runners.
- `pkg/cliapp/async_progress.go` handled background heartbeat telemetry for coordinator events, but did not emit live console progress to the human terminal.
- Terminal commands that executed longer than 200ms appeared completely silent to operators until final output rendering.

---

## 3. Implemented Architecture

### 3.1 StepTracker & High-Resolution Timing (`pkg/cli/ux/spinner.go`)
`StepTracker` manages sequential pipeline stages with microsecond precision:
```go
type StepTracker struct {
    mu        sync.Mutex
    stepName  string
    startTime time.Time
    active    bool
}
```
- `Update(detail string)`: Updates active spinner suffix in-flight.
- `Complete(summary string)`: Halts spinner and renders green checkmark with elapsed time: `✓ Schema validation [120ms]`.
- `Fail(summary string, err error)`: Halts spinner and renders red cross with error details and duration: `✗ Sync failed: connection reset [450ms]`.

### 3.2 Unified Integration via `RunWithAsyncProgress` (`pkg/cliapp/async_progress.go`)
By hooking `StepTracker` directly into `RunWithAsyncProgress`, all commands utilizing `cli.BindAsyncProgress` inherit responsive progress:

```mermaid
sequenceDiagram
    autonumber
    actor Operator as Human Operator / Agent
    participant CLI as zqk CLI
    participant UX as pkg/cli/ux
    participant Backend as Storage / Validation Engine

    Operator->>CLI: zqk system check BLI-001
    CLI->>UX: NewStepTracker("Running system_check")
    UX-->>Operator: ⠋ Running system_check...
    Backend->>CLI: Progress Callback ("spec", "validating schema")
    CLI->>UX: Update("spec: validating schema")
    UX-->>Operator: ⠙ Running system_check (spec: validating schema)...
    Backend-->>CLI: Complete
    CLI->>UX: Complete("system_check completed")
    UX-->>Operator: ✓ system_check completed [42ms]
```

### 3.3 Negative Boundary: Suppression Discipline
To ensure machine readability for LLM agents, CI pipelines, and piped consumers:
- TTY checks verify `term.IsTerminal(os.Stdout.Fd())`.
- Environment flags `CI`, `TERM=dumb`, and `ZQK_NON_INTERACTIVE` force non-interactive mode.
- Non-human context profiles (`ai-agent`, `mcp`) and non-text formats (`json`, `yaml`, `json-rpc`) dynamically suppress visual spinners via `ux.SetSuppressed(true)`:

```mermaid
flowchart LR
    A["Caller Profile: ai-agent"] --> SUPPRESS["ux.SetSuppressed(true)"]
    B["Flag: --format json"] --> SUPPRESS
    C["Flag: --quiet"] --> SUPPRESS
    D["Env: CI=true"] --> SUPPRESS
    SUPPRESS --> OUT["Clean Stream Output (No ANSI Escape Codes)"]
```

---

## 4. Verification & Criteria Proofs

| Criteria Reference | Specification | Verification Result | Status |
| :--- | :--- | :--- | :--- |
| `CRIT-1791318796173626000-10adcce5` | Static Floor: Commands >200ms bind unified console progress emitter | Verified: `RunWithAsyncProgress` wraps >50 CLI commands with `StepTracker` | **PASS** |
| `CRIT-1791318796173627000-b941739e` | Operational Proof: Interactive sessions display active step name and elapsed timer | Verified: `TestStepTracker_CompleteAndFail` validates elapsed duration and step updates | **PASS** |
| `CRIT-1791318796173628000-71792b85` | Negative Boundary: Non-interactive/JSON executions suppress ANSI codes | Verified: `TestSuppressionControl` proves zero ANSI codes on JSON, quiet, CI, and agent profiles | **PASS** |

---

## 5. Traceability Matrix

```mermaid
flowchart LR
    REQ["REQ-1791318795739874000-3d002973<br/>Console UX & Zero-Stall User Experience"]
    CRIT1["CRIT-1791318796173626000-10adcce5<br/>Unified Console Progress Emitter"]
    CRIT2["CRIT-1791318796173627000-b941739e<br/>Elapsed Timer & Step Display"]
    CRIT3["CRIT-1791318796173628000-71792b85<br/>Clean Structured Log Suppression"]
    TST["TST-1791318796173626001-52c13075<br/>Console UX & Progress Test Suite"]
    BLI["BLI-1791318796173626000-deb88239<br/>Audit Console UX & Unified Spinners"]

    REQ --> CRIT1
    REQ --> CRIT2
    REQ --> CRIT3
    CRIT1 --> TST
    CRIT2 --> TST
    CRIT3 --> TST
    TST --> BLI
```
