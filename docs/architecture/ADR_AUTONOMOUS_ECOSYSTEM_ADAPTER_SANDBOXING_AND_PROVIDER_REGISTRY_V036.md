# ADR-0039: Autonomous Ecosystem Adapter Sandboxing & Provider Registry v0.3.6

- **Title: Autonomous Ecosystem Adapter Sandboxing & Provider Registry v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

With the introduction of the Provider SDK (v0.3.6), the ZQK open-core ecosystem enables third-party providers and community adapters to extend runtime capabilities. However, untrusted, buggy, or misconfigured adapters could introduce memory leaks, orphaned goroutines, or unhandled panics that destabilize core kernel execution. A formal sandboxing model and typed registry resolution engine are required to isolate external adapters fail-closed.

## Decision

1. **Adapter Sandboxing & Process Isolation**:
   - Execute all third-party adapter invocations within isolated execution boundaries that intercept runtime panics and convert them to typed `ErrHookPanicked` errors.
   - Enforce bounded context timeouts (default 5s, hard maximum 30s) to prevent runaway or deadlocking third-party code.
   - Reject unmanaged background goroutines and verify process isolation.

2. **Dynamic Typed Provider Registry**:
   - Provide atomic, thread-safe registration, lookup, and enumeration of ecosystem adapters via `ProviderRegistry`.
   - Validate adapter metadata, naming conventions (alphanumeric, hyphens, underscores), and target engine version compatibility.
   - Prevent duplicate adapter registration and disallow empty capability sets.

3. **Ontological Traceability & Quality Gate**:
   - Map all sandboxing contracts directly to Knowledge Kernel CAS objects (`goal`, `milestone`, `requirement`, `criteria`, `backlog_item`, `test_case`).
   - Verify zero leaked project-specific kernel IDs in public guides per `POL-DOC-001`.

## Consequences

- Third-party adapters execute securely within guaranteed memory and timeout boundaries.
- The ZQK core runtime remains resilient against external panics, timeouts, and resource exhaustion.
- Developer experience is enhanced through clear, deterministic error reporting and capability discovery.
