# Convergence Policy Orchestrator & Autonomous Remediation Engine

## Status
- **Author**: PER-COMMUNITY-SYSTEM-ARCHITECT
- **Cycle**: Cycle 75
- **Status**: Approved / Draft

---

## 1. Executive Summary & Context

The Autonomous Convergence Loop in ZQK orchestrates autonomous agent swarms, continuous verification gates, and background daemon execution to drive systems toward target invariants. However, unexpected environmental interruptions, flaky subprocess concurrency, resource exhaustion, or transient CI errors can threaten loop forward velocity.

Under `POL-AGENT-TPM-INTAKE-FIREWALL-001`, chat-reported bugs and failures must never distract or derail ongoing plan execution. Analogously, runtime pipeline and build telemetry failures must not stall execution loops or trigger unconstrained thrashing.

**Cycle 75 introduces the Convergence Policy Orchestrator & Autonomous Remediation Engine** (`pkg/convergence`), providing:
1. **Convergence Policy Rule Engine**: Pluggable, structurally validated rule sets that match failure signatures from build and CI telemetry against predefined remediations.
2. **Deterministic Remediation Action Dispatcher**: Safe dispatching of bounded self-healing actions (e.g., deterministic retry with jitter, cache purging, task isolation, and agent dispatch).
3. **Anti-Thrashing & Acyclic Loop Safeguards**: Mathematical damping and fail-closed bounds that prevent cyclic remediation loops or runaway trigger cascades.

---

## 2. Core Architecture & Components

```
   ┌────────────────────────────────────────────────────────┐
   │                Failure Telemetry Payload               │
   │      (Job Name, Step, Exit Code, Error Signature)       │
   └───────────────────────────┬────────────────────────────┘
                               │
                               ▼
   ┌────────────────────────────────────────────────────────┐
   │          Convergence Policy Evaluator Engine           │
   │    - Match TargetFilter and Severity Classification    │
   │    - Evaluate Structural Weight & Invariant Bounds     │
   └───────────────────────────┬────────────────────────────┘
                               │
                               ▼
   ┌────────────────────────────────────────────────────────┐
   │             Remediation Dispatcher & Guard             │
   │    - Cyclic Trigger Prevention (Acyclic Check)         │
   │    - Max Retry & Exponential Backoff Clamping          │
   │    - Deterministic Remediation Execution Receipt       │
   └────────────────────────────────────────────────────────┘
```

### 2.1 Domain Models & Interfaces

```go
package convergence

import (
	"context"
	"time"
)

type ImpactClassification string

const (
	ImpactCritical ImpactClassification = "critical"
	ImpactHigh     ImpactClassification = "high"
	ImpactMedium   ImpactClassification = "medium"
	ImpactLow      ImpactClassification = "low"
)

type ActionType string

const (
	ActionRetry         ActionType = "retry"
	ActionCachePurge    ActionType = "cache_purge"
	ActionTaskIsolation ActionType = "task_isolation"
	ActionAgentDispatch ActionType = "agent_dispatch"
)

type PolicyRule struct {
	ID             string               `json:"id" yaml:"id"`
	Name           string               `json:"name" yaml:"name"`
	TargetFilter   string               `json:"target_filter" yaml:"target_filter"`
	Impact         ImpactClassification `json:"impact" yaml:"impact"`
	Action         ActionType           `json:"action" yaml:"action"`
	MaxRetries     int                  `json:"max_retries" yaml:"max_retries"`
	BackoffBaseMs  int                  `json:"backoff_base_ms" yaml:"backoff_base_ms"`
	Weight         int                  `json:"weight" yaml:"weight"`
	Enabled        bool                 `json:"enabled" yaml:"enabled"`
}

type TelemetryPayload struct {
	SessionID string            `json:"session_id"`
	JobID     string            `json:"job_id"`
	StepName  string            `json:"step_name"`
	ExitCode  int               `json:"exit_code"`
	ErrorMsg  string            `json:"error_msg"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type RemediationReceipt struct {
	ReceiptID    string     `json:"receipt_id"`
	RuleID       string     `json:"rule_id"`
	Action       ActionType `json:"action"`
	SessionID    string     `json:"session_id"`
	Attempt      int        `json:"attempt"`
	BackoffDelay time.Duration `json:"backoff_delay"`
	DispatchedAt time.Time  `json:"dispatched_at"`
	Executed     bool       `json:"executed"`
}
```

### 2.2 Invariant Guarantees & Safeguards

1. **Static Floor Invariant**:
   - `PolicyRule` configurations must satisfy immutable structural bounds: `ID` and `Name` cannot be empty; `Impact` must be one of the 4 defined enum constants; `Weight` must be within `[1, 100]`; `MaxRetries` must be within `[0, 10]`; `BackoffBaseMs` must be positive.
2. **Operational Dynamic Proof**:
   - Given a failure telemetry payload matching an enabled rule's filter, the orchestrator evaluates the highest-weight matching rule and safely dispatches a `RemediationReceipt` with correctly calculated backoff delay.
3. **Negative Adversarial Boundary**:
   - Cyclic triggers (repeated failure loops exceeding `MaxRetries` on the same session/job) fail-closed by rejecting automatic dispatch and transitioning the session to quarantine.
   - Corrupted or invalid payloads (e.g. empty `SessionID`, negative exit codes, or unregistered actions) are rejected with deterministic error returns.

---

## 3. Verification & Traceability

- **Unified Test Case**: Three-fold proof test suite
- **Target Implementation**: `pkg/convergence/orchestrator.go`, `pkg/convergence/evaluator.go`, `pkg/convergence/orchestrator_test.go`
