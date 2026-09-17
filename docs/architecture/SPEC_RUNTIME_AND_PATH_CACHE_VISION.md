# Vision: path-keyed cache, migration-safe FS, spec-as-objects, “classloader” loading

**Last Verified:** 2026-08-31


**Status:** Direction note (not an implementation plan)  
**Related:** [CLI_OBJECT_MUTATION_LATENCY.md](./CLI_OBJECT_MUTATION_LATENCY.md), [MATRIX_CLI_STRATEGY.md](./MATRIX_CLI_STRATEGY.md), [DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md](./DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md), `docs/quality/matrix_registry.yaml`

## Problem being solved

Once **everything that touches disk** flows through a **path cache with a stable key**, most of the application can **ignore physical layout**: change the key mapping, and callers keep using the same logical handles.

Only the **filesystem / storage layer** must:

- Apply **reliable, repeatable** moves on disk (or moves of **pointers** to content-addressed blobs).
- **Recognize and finish** migrations that were interrupted mid-flight (crash, kill, full disk).
- Expose a narrow contract upward: “this key resolves here now,” with explicit **migration state**.

## Data cell / stream pattern (next hardening)

Stand up and **harden** the **data cell / stream** pattern, then tie it to:

- **Spec creation** and **glossary** insertions (same durability and traceability as other objects).
- Criteria/backlog links where product requires measurable rollout.

## Specs as first-order system objects

Target state: **object specs** (and related definitions) are **ordinary persisted objects**—not a parallel file-only world—so they participate in the same CAS, audit, and convergence machinery as backlog and sessions.

**Codegen** should eventually **write** spec-shaped objects during a **pre-init** phase (before most user commands), analogous to a **classloader** in the Java runtime:

- **Bootstrap** loads core kinds and field metadata.
- **Early caches** can still be built and reused by scheduler, CLI, and agents—through a **single standardized “spec load” path**—instead of ad-hoc reads scattered across packages.

## Runtime metaphor

**ZQK** behaves somewhat like **stop-motion**: discrete, durable frames of state. The **scheduler** and **runtime caches** are the components that **depend on a long-lived service**; other data can be **parked** (PARK stage, batch queues, stream buffers) for fast access without requiring every reader to hold a live connection.

This note does **not** prescribe timelines; it aligns engineering bets with a **stable persistence engine** and **keyed path resolution** so spec and stream work can land without another full storage rewrite.
