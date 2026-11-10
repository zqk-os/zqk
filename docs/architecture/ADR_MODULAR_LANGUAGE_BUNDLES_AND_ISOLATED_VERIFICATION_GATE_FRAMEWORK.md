# ADR: Modular Language Bundles and Isolated Verification Gate Framework

## Status
Accepted

## Context
As ZQK expands into multi-language polyglot agent swarms, different language ecosystems require specific architectural standards, AST hygiene rules, static analysis gates, and role directives. Previously, verification gates and AST policies were monolithic or hardcoded to Go. To enable extensible language support while enforcing fail-closed security invariants, a modular language bundle framework is required.

## Decision
We implement a pluggable, isolated Language Bundle Subsystem under `pkg/bundle`:
1. **Manifest Schema (`LanguageBundleManifest`)**: Defines a schema-versioned contract specifying language metadata, architectural standards (`standards`), AST policies (`ast_policies`), verification gates (`gates`), and target agent directives (`agent_directives`).
2. **Hermetic Registry Lifecycle (`Registry`)**: Manages the installation, discovery, retrieval, and removal of language bundles scoped to a configurable root directory, preventing global side-effects.
3. **Static Floor & Adversarial Boundary Invariants**:
   - Manifest validation rejects missing required fields (`name`, `language`, `version`).
   - Rejects directory traversal attempts in bundle names or directive file paths (`..`).
   - Prevents recursive circular dependencies where a bundle depends upon itself.
4. **Agent Rule Dynamic Seeding**: When a bundle is installed, agent directives are automatically materialized into isolated workspace paths (e.g. `.agents/rules/<lang>.md`) to guide language-specific subagents during code synthesis.

## Consequences
- Multi-language verification gates can be dynamically packaged and audited without modifying core kernel binaries.
- Strict path hygiene guarantees that untrusted language bundles cannot escape their designated directory boundary.
