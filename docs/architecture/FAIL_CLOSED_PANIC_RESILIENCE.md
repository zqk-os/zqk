# Fail-Closed Panic Elimination and Resilient Fallback Architecture

## Overview

This architecture document defines the governance rules and patterns for eliminating unhandled raw panics across production runtime and utility pathways within the ZQK Knowledge Kernel (specifically `pkg/ambient`, `pkg/coordination`, and `pkg/logging`).

## Problem Statement

Unhandled raw `panic()` invocations in production code paths violate fail-closed resilience:
1. **Uncontrolled Process Termination**: An invalid argument or uninitialized resource terminating the process halts daemon and background worker execution.
2. **Missing Degradation Fallbacks**: When optional telemetry, coordination helpers, or ambient buffers receive edge-case inputs, crashing prevents core operations from completing.
3. **Flaky Concurrency and Observability**: Subsystems interacting with ambient telemetry or CLI progress notifiers should fail safely or degrade to no-op handlers rather than crashing.

## Architecture & Implementation Patterns

### 1. Defensively Clamped Ring Buffers (`pkg/ambient`)
In `AutonomyInbox`, capacity allocation handles non-power-of-two and out-of-bound inputs defensively:
- Below `MinInboxCapacity` (16): Automatically clamped up to `MinInboxCapacity`.
- Above `MaxInboxCapacity` (1<<20): Automatically clamped down to `MaxInboxCapacity`.
- Non-power-of-two inputs: Rounded up to the nearest power of two via bit manipulation.
- Zero raw `panic()` calls on construction or buffer overflow; push overflows return `ErrInboxFull` and increment atomic drop counters.

### 2. Resilient Inverted Adapters (`pkg/coordination`)
In `NewCLINotifierWithCoordinator`:
- When mandatory parameters (`projectRoot`, `storageProvider`) are absent or empty, the function returns a valid, non-nil `storage.NoopOperationNotifier{}`.
- Callers receive an operational interface implementation without risking `nil` dereferences or runtime crashes.

### 3. Static AST Guardrails (`pkg/ambient/reliability_hardening_test.go`)
Automated Go AST audits dynamically scan production source trees for unauthorized `panic()` statements. Any raw `panic` in daemon, coordination, or telemetry runtime paths causes test failures, enforcing an architectural static floor against regressions.

## Invariants

1. **Zero Raw Panics in Production Code**: Subsystems return structured errors or resilient no-op implementations instead of calling `panic()`.
2. **Graceful Clamping**: Capacity constraints clamp to safe operational bounds rather than failing closed on non-critical sizing options.
3. **Deterministic AST Audit**: CI and verification test suites continually parse ASTs to assert panic-free runtime floors across all production packages.
