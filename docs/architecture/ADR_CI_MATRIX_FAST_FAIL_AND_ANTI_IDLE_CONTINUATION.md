# ADR: CI Matrix Fast-Fail Enforcement and Autonomous Loop Anti-Idleness Continuation

## Context
During high-throughput continuous agent programming (CAP) loops, two major operational inefficiencies emerged:
1. **CI Runner Waste on Matrix Failures**:
   In `.github/workflows/ci.yml`, the unit test matrix previously configured `strategy: fail-fast: false`. When an early partition failed, the remaining test shards continued executing for up to 17 minutes before the job failed closed. This runaway execution burned GitHub Actions runner hours and delayed feedback loops.
2. **Autonomous Agent Idling and Synthetic Polling**:
   Agents frequently fell into synthetic polling patterns (e.g. scheduling 30s/45s/60s sleep timers) while awaiting CI completion or PR auto-merges. Halting tool invocations to wait for external processes transitions agents into `waiting_for_input` (idle), violating the continuous velocity mandate.

## Decision
1. **Mandatory CI Matrix Fail-Fast (`fail-fast: true`)**:
   - The test matrix in `.github/workflows/ci.yml` is codified with `fail-fast: true`. Any partition failure immediately cancels all remaining in-flight matrix partitions, conserving runner compute and accelerating feedback loops.
2. **Outbound Stage Status Notification Hooks**:
   - CI workflow partitions and milestone steps invoke reactive notification hooks upon completion or failure (`if: always()`). Push notifications and webhook dispatches provide real-time state delivery rather than requiring external pollers.
3. **Strict Anti-Polling & In-Flight Pipelining Mandate**:
   - Codify in `.agents/AGENTS.md`, prompt templates, and agent onboarding payloads that setting synthetic interval polling timers (`schedule DurationSeconds: ...`) while awaiting PRs or CI is strictly forbidden.
   - Mandate continuous in-flight pipelining: while CI runs asynchronously, the agent must immediately execute process administration, next-cycle grooming, technical debt remediation, and test case authoring.
4. **Three-Fold Proof Verification**:
   - Automated tests in `pkg/community/ci_anti_idle_test.go` verify that `ci.yml` maintains `fail-fast: true`, AGENTS.md codifies the anti-polling invariant, and slow-fail matrix configurations are rejected.

## Status
Accepted and codified in Knowledge Kernel for Cycle 99 (CI Matrix Fast-Fail Enforcement, Outbound Status Notification Hooks, and Anti-Idleness Protocol Codification).
