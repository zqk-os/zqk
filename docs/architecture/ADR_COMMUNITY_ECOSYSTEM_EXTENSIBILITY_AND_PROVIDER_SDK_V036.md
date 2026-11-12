# ADR-0038: Community Ecosystem Extensibility, Custom Plugin Hooks and Provider SDK v0.3.6

- **Title: Community Ecosystem Extensibility, Custom Plugin Hooks and Provider SDK v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

As the ZQK open-core platform expands, third-party contributors, plugin authors, and cloud ecosystem integrators require robust, isolated, and type-safe extensibility interfaces. Previously, custom integrations required internal package modifications or direct binary patching. Version v0.3.6 introduces a formal Provider SDK and lifecycle hook architecture enabling dynamic provider registration, isolated event interception, and AST-verified contract stability without breaking backward compatibility or compromising kernel security boundaries.

## Decision

1. **Provider SDK Abstraction Boundary**:
   - Establish typed Go interfaces (`ProviderRegistry`, `LifecycleHookHandler`, `PluginContext`) in `pkg/community` and `pkg/clihooks`.
   - Provide deterministic hook registration for pre-execution, post-execution, telemetry emission, and failure interception.
   - Enforce fail-closed isolation: plugin failures must never crash the core kernel or corrupt CAS state.

2. **Validation & Boundary Protection**:
   - Enforce bounded execution timeouts (max 5000ms default per hook invocation) with cancel-safe context propagation.
   - Require explicit semantic version compatibility declarations (`TargetEngineVersion`) for all registered providers.
   - Disallow unsafe memory manipulation, unmanaged goroutine spawning, or unbuffered disk writes inside hook implementations.

3. **Ontological Traceability & Documentation**:
   - Register all architectural contracts, test suites, and schema specifications directly in the Knowledge Kernel CAS.
   - Index this specification via `docman register` and verify compliance using `zqk-vet`.

## Consequences

- Third-party extensions and enterprise provider bridges can be developed independently using a stable, typed SDK.
- The core runtime remains protected against untrusted or crashing plugins via isolation sandboxes and strict timeouts.
- All extension hooks are discoverable, auditable, and covered by automated regression verification.
