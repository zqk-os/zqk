# ADR-0046: Autonomous Provider Config Schema Validation & Secrets Injection v0.3.6

- **Title: Autonomous Provider Config Schema Validation & Secrets Injection v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

Community providers often require structured configuration (e.g. endpoint URLs, timeouts, concurrency limits) and sensitive credentials (API tokens, mutual TLS private keys). If configuration is unvalidated at initialization time, runtime panics occur. Furthermore, storing secrets in plaintext configuration violates kernel security invariants.

## Decision

1. **Schema-Enforced Configuration Parsing**:
   - Introduce `ProviderConfigValidator` enforcing strict JSON schema definitions for community provider initialization.
   - Reject malformed fields, missing required properties, or out-of-bound numerical constraints prior to provider activation.

2. **Scoped Secret Injection & Redaction**:
   - Provide `ProviderSecretStore` capable of scoped runtime secret resolution.
   - Ensure credentials are provided as ephemeral in-memory references and never persisted to plain disk manifests or leaked into execution logs.

3. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution priority plans.
   - Complies fully with `POL-DOC-001` character budgets and prohibition of internal cycle sprint narratives in public docs.

## Consequences

- Provider configurations are strictly validated prior to loading, preventing initialization-time crashes.
- Secrets are securely managed and quarantined from public metadata exports.
- Extends the core community SDK with production-ready security posture.
