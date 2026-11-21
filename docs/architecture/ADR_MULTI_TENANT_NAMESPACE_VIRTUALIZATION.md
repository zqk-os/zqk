# ADR: Multi-Tenant Namespace Virtualization & Boundary Enforcement

## Status
Accepted

## Date
2026-11-21

## Context
As the ZQK Knowledge Kernel scales to host multiple independent teams, organizations, and autonomous agent swarms on shared control-plane infrastructure, strict multi-tenant isolation becomes a foundational requirement. Without deterministic namespace virtualization and boundary enforcement, noisy-neighbor workloads can exhaust shared token budgets or concurrency capacity, and misconfigured or adversarial callers could attempt cross-tenant impersonation or unauthorized state access.

In Cycle 164, we introduce Multi-Tenant Namespace Virtualization & Boundary Enforcement in `pkg/community/tenant_namespace.go` to guarantee sovereign tenant isolation, per-tenant quota governance, and fail-closed boundary containment.

The architecture must:
1. Enforce **sovereign namespace partitioning & quota invariants**: each tenant is bound to an isolated virtualized namespace with immutable identity boundaries, explicit maximum concurrency slots, and bounded token quota allocations.
2. Manage **concurrency slots, token quota deduction, and usage telemetry**: provide atomic admission control for acquiring and releasing execution slots, deterministic token quota deduction prior to or during workload execution, and thread-safe per-tenant usage telemetry for observability and accounting.
3. Implement **cross-tenant impersonation defense and fail-closed boundary rejection**: validate caller identity and target namespace ownership on every operation, rejecting blank identifiers, unregistered namespaces, quota exhaustion, concurrency saturation, and cross-tenant spoofing attempts with explicit fail-closed errors.
4. Adhere to declarative Go validation (`strutil.ErrorOnBlank`) and concurrency-safe state management without external side effects.

## Decision
1. Implement the multi-tenant namespace controller, quota governance primitives, and boundary enforcement guards in `pkg/community/tenant_namespace.go`.
2. Partition all tenant state (quota limits, active concurrency slots, cumulative token consumption, and telemetry counters) by validated sovereign namespace identifiers, ensuring zero shared mutable quota pools across tenants.
3. Enforce atomic slot acquisition/release and token deduction invariants so that concurrent agent dispatches cannot exceed a tenant's configured concurrency ceiling or token budget.
4. Enforce fail-closed boundary verification on all namespace-scoped operations: any mismatch between authenticated caller tenant context and target namespace, or any invalid/unregistered namespace reference, is immediately rejected without partial state mutation.
5. Provide comprehensive verification in `pkg/community/tenant_namespace_test.go` covering the Three-Fold Proof:
   - **Static Floor Invariant**: Valid namespace registration, quota configuration defaults and bounds, and immutable tenant partition initialization.
   - **Operational Dynamic Proof**: Concurrent slot acquisition/release, deterministic token quota deduction, and accurate usage telemetry under parallel multi-tenant load.
   - **Negative Adversarial Boundary**: Fail-closed rejection of cross-tenant impersonation attempts, blank or malformed namespace/caller identifiers, concurrency slot exhaustion, and token quota overruns.
6. Register this ADR in the Knowledge Kernel CAS via `zqk docman register` and link it to the governing requirement.

## Consequences
- Guarantees strong sovereign isolation between tenants sharing the same ZQK runtime.
- Prevents noisy-neighbor resource starvation via deterministic per-tenant concurrency and token quota enforcement.
- Eliminates cross-tenant impersonation vectors through fail-closed boundary checks on every namespace operation.
- Provides structured usage telemetry per tenant to support capacity planning, auditing, and quota governance.
