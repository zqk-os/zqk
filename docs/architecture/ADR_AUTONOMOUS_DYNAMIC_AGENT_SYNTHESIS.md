# ADR: Autonomous Dynamic Agent Synthesis & Transient Worker Materialization

## Status
Accepted

## Date
2026-11-20

## Context
Static, pre-provisioned agent personas cannot efficiently cover every specialized intent encountered by an autonomous swarm. Maintaining long-lived idle worker runtimes for niche capabilities wastes memory, widens the active tool-permission surface, and risks cross-task context contamination. Conversely, spawning ad-hoc workers without strict structural validation, token budgeting, or lifecycle teardown guarantees introduces unbounded resource consumption and sandbox escape vectors.

In Cycle 163, we introduce an Autonomous Dynamic Agent Synthesis and Transient Worker Materialization subsystem in `pkg/community/dynamic_agent_synthesizer.go`. This subsystem synthesizes specialized ephemeral agent micro-runtimes directly from structured intent requirements, executes them within strictly bounded capabilities, and guarantees deterministic resource reclamation upon completion or failure.

## Decision
1. **Dynamic Agent Manifest Specification & Static Invariants**:
   - Every synthesized worker is defined by a validated agent manifest specifying its role identifier, allowed tool capability set, token budget ceiling, memory fence limit, and sandbox isolation policy.
   - Manifest validation enforces static floor and ceiling invariants before any runtime materialization:
     - Role identifiers and intent specifications must be non-blank.
     - Requested tool capabilities must be a strict subset of the caller's authorized capability envelope; wildcard or undeclared capability escalation is rejected fail-closed.
     - Sandbox isolation policy must explicitly declare filesystem, network, and process boundaries (defaulting to least-privilege read-only or workspace-scoped access with no implicit host escalation).
     - Memory and execution-duration ceilings must be positive and bounded within configured operational limits.

2. **Token-Budgeted Prompt Synthesis & Transient Worker Materialization Lifecycle**:
   - Prompt synthesis constructs a minimal, intent-tailored system prompt bounded by an explicit token budget, prioritizing relevant constraints and omitting extraneous ambient context.
   - If synthesized prompt content exceeds the manifest's token budget, deterministic truncation or compaction is applied according to priority tiers, or synthesis fails closed if mandatory policy invariants cannot fit within the budget.
   - Transient worker materialization follows a deterministic state machine (`synthesizing` -> `materialized` -> `executing` -> `tearing_down` -> `terminated`), issuing unique ephemeral worker identifiers and isolated runtime contexts per invocation.

3. **Fail-Closed Sandbox Escape Containment & Guaranteed Worker Teardown**:
   - Runtime execution is guarded by fail-closed sandbox barriers that intercept unauthorized tool calls, out-of-bounds path/network access, and memory fence breaches, immediately halting execution and triggering containment teardown.
   - Guaranteed worker teardown executes unconditionally via `defer`-backed and context-cancellation-aware cleanup hooks on normal completion, error, panic recovery, or timeout:
     - Revokes ephemeral capability tokens and active leases.
     - Releases allocated memory buffers, goroutines, and temporary scratch handles to prevent memory or goroutine leaks.
     - Transitions the worker record to a terminal `terminated` state so no zombie or reusable stale runtime remains addressable.

## Consequences
- Enables just-in-time specialization of ephemeral workers tailored to arbitrary intent specifications without maintaining permanent idle runtimes.
- Bounds context bloat and inference cost through strict token-budgeted prompt synthesis.
- Enforces defense-in-depth via fail-closed capability validation, sandbox escape containment, and guaranteed post-task teardown.
