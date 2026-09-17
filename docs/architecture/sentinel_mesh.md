# Tool Pod Mesh: Automated Retries and Sentinel Rollbacks

**Last Verified:** 2026-08-31

## Priority Plan: [REDACTED-ID]

## Overview
This architectural spec defines the automated retry and rollback mechanism within the Orchestrator, deeply integrating with `truth-sentinel` and `enterprise-truth-sentinel`. The core objective is to enforce safe parallelism across Tool Pods. Any test or policy failure must instantly trigger a safe rollback of a subagent's changes and generate an alert for the CAP (Continuous Alignment & Planning) loop.

## Architecture

### 1. Orchestrator Mesh Integration
The Orchestrator manages a mesh of Tool Pods where subagents execute tasks concurrently. To ensure safe parallelism:
- Each subagent operates within an isolated workspace (e.g., branched or shared workspace).
- The Orchestrator monitors the execution state of all subagents in real-time.

### 2. Sentinel Integration
The system relies on two layers of Sentinels for validation:
- **`truth-sentinel`**: Validates local project correctness, including unit tests, linting, and basic policy compliance.
- **`enterprise-truth-sentinel`**: Enforces organizational-wide policies, cross-project dependencies, and broader security/compliance checks.

Before any subagent's work is merged or finalized, the Orchestrator invokes both Sentinels in sequence.

### 3. Automated Retry Mechanism
If a Sentinel detects a failure (e.g., test failure, policy violation):
1. The Orchestrator intercepts the failure signal.
2. An automated retry is triggered, up to a configurable `MAX_RETRIES` limit.
3. The failure context (logs, Sentinel feedback) is injected into the subagent's context to guide the retry attempt.

### 4. Sentinel Rollback & CAP Loop Alert
If the failure persists after `MAX_RETRIES`, or if a critical violation (e.g., severe security policy breach) is detected:
1. **Instant Rollback**: The Orchestrator instantly terminates the subagent and discards its isolated workspace (branched or shared), reverting any intermediate state.
2. **CAP Loop Alert**: An alert is generated and dispatched to the CAP loop. This alert contains:
   - The subagent's execution trace.
   - The specific Sentinel violations.
   - Diff of the attempted changes.
3. The CAP loop can then analyze the failure, adjust the plan, and potentially redefine the task or policies.

## Workflow
1. **Subagent Execution**: Subagent begins work in an isolated workspace.
2. **Sentinel Verification**: Upon completion of a task segment, the Orchestrator triggers `truth-sentinel` and `enterprise-truth-sentinel`.
3. **Success**: If Sentinels pass, the work is committed/merged.
4. **Failure (Retryable)**: If Sentinels fail, the Orchestrator feeds the error back to the subagent and retries.
5. **Failure (Fatal/Limit Reached)**: The Orchestrator triggers a rollback, discards the workspace, and alerts the CAP loop.

## Traceability
Every rollback and retry event is recorded as a system object in the Knowledge Kernel, linking back to the original Priority Plan ([REDACTED-ID]) to ensure full traceability and auditability.
