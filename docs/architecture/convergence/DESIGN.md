# Execution Convergence Controller: Architecture Design

**Last Verified:** 2026-08-31

## Overview
The `ConvergenceController` is an autonomous actor that ensures the ZQK system's actual execution progress aligns with the roadmap defined in the Knowledge Kernel. It continuously reconciles real-time telemetry (scheduler outcomes) against planned milestones (`MIL-` objects).

## Core Mechanisms
1. **Telemetry Ingestion**: Monitors the system audit stream for job outcomes (`success`, `failure`, `stall`).
2. **Reconciliation Loop**: Periodically queries the Knowledge Kernel to compare current performance metrics against the `status` of our `BacklogItem`s and `Milestone`s.
3. **Drift Remediation**: If drift is detected (e.g., a backlog item is delayed or a milestone is missed), the controller triggers a self-correction event, which the `OrchestrationManager` uses to re-prioritize or re-allocate agent resources.

## Interface Contract
```go
package convergence

import (
    "context"
    "github.com/zqk/pkg/objects"
)

// ConvergenceController interface for reconciling roadmap state.
type ConvergenceController interface {
    // Reconcile evaluates a specific priority plan against latest telemetry.
    Reconcile(ctx context.Context, planID string) error
    
    // DetectDrift identifies backlog items that have missed their execution window.
    DetectDrift(ctx context.Context, planID string) ([]DriftReport, error)
}
```

## Implementation Strategy
- **Background Worker**: Managed as a standard ZQK background task.
- **Observability**: All reconciliation decisions are logged as high-priority audit events.
- **Policy Bound**: All remediation actions must be validated by the `PolicyNegotiator` to ensure they respect system-wide safety constraints.
