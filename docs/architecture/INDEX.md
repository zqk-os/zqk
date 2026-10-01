# ZQK Knowledge Kernel Architecture Index

<!-- tags: architecture, index, storage, membrane, governance, cli, daemons, orchestration, security -->

Welcome to the architectural specifications and design documents for the **ZQK Knowledge Operating System (KOS)**. This index categorizes all core design documents, invariants, threat models, and subsystem specifications.

---

## 1. Storage & Membrane Architecture
Documents governing Content-Addressable Storage (CAS), append-only WAL streams, memory isolation, and privileged filesystem access.

- **[Cellular Membrane Mode B Configuration](./CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.md)**: Mode A (developer standalone) vs Mode B (cellular membrane lockdown with isolated `PrivilegedWriterDaemon` and UNIX domain socket IPC).
- **[Cellular Specialization Tiers & Rubber Room](./CELLULAR_SPECIALIZATION_TIERS.md)**: Biological organ specialization tiers (`Neuron`, `Muscle`, `Heart`, `Lung`) and sandboxed side-effect execution in the Rubber Room (`ModeShielded` / `ShadowSpine`).
- **[Tiered Storage and Archival Lifecycle](./TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.md)**: Three-tier storage hierarchy: hot active CAS, warm index layers, and cold compressed stream archives.
- **[Storage Write Queue & Observer Governance](./STORAGE_WRITE_QUEUE_OBSERVER_GOVERNANCE.md)**: Non-blocking asynchronous event queues, observer fan-out, and memory hygiene.
- **[Project-Scoped Reverse Reference Index and Thread-Safe Caches](./PROJECT_SCOPED_REVERSE_REFERENCE_INDEX_AND_THREAD_SAFE_CACHES.md)**: Reverse-reference graph lookup performance, atomic index caches, and memory-safe cache invalidation.

---

## 2. Mutation & Data Integrity Governance
Documents governing state transitions, provenance enforcement, and fail-closed validation.

- **[Membrane Mutation Governance & Integrity Specification](./MUTATION_GOVERNANCE.md)**: System provenance fields (`created_at`, `cas_address`, `hash`), directed state graph transitions, audited `--override` elevation, and interactive TTY friction controls.
- **[Lifecycle State Machine Architecture](./LIFECYCLE_STATE_MACHINE.md)**: Formal lifecycle definitions, state transitions, and precondition verification engines.
- **[Fail-Closed Error Propagation & Transaction Resilience](./FAIL_CLOSED_ERROR_PROPAGATION_AND_TRANSACTION_RESILIENCE.md)**: Fail-closed boundary guarantees across storage, networking, and validation boundaries.
- **[Fail-Closed Panic Resilience](./FAIL_CLOSED_PANIC_RESILIENCE.md)**: Subprocess and thread isolation preventing daemon crashes and unhandled panic propagation.

---

## 3. CLI Command Taxonomy & Ergonomics
Documents governing the Cobra CLI command surface, taxonomy standards, builders, and verb mappings.

- **[CLI Command Taxonomy Standards](./CLI_COMMAND_TAXONOMY_STANDARDS.md)**: Canonical noun-verb command standards, grammar rules, YAML specification inventory, and regression guardrails.
- **[Orphan Verbs Retirement & Canonical Domain Mapping](./ORPHAN_VERBS_RETIREMENT_AND_CANONICAL_DOMAIN_MAPPING.md)**: Ergonomic root shortcuts (`zqk do`, `zqk inspect`, `zqk mutate`) mapped to canonical domain commands (`zqk workflow do`, `zqk object inspect`, etc.).
- **[Acronym Vocabulary Scheme & Progressive Disclosure](./ACRONYM_VOCABULARY_SCHEME_AND_PROGRESSIVE_DISCLOSURE.md)**: Glossary scheme for technical acronyms (CAS, VDS, DoD, BLI, REQ, CRIT) and cognitive load management.

---

## 4. Daemons, Schedulers & Concurrency
Documents governing persistent host processes, background watchers, and job concurrency.

- **[Daemon Singleton Mutual Exclusion](./DAEMON_SINGLETON_MUTUAL_EXCLUSION.md)**: Cross-platform PID lock acquisition, single-instance enforcement, and automatic recovery of dead daemon leases.
- **[Seat Worker Supervisor Architecture](./SEAT_WORKER_SUPERVISOR_ARCHITECTURE.md)**: OS-level supervisor daemons (`launchd` on macOS, `systemd` on Linux) managing persistent 24/7 background agent seats.
- **[Ambient Signal Action Rubric](./AMBIENT_SIGNAL_ACTION_RUBRIC.md)**: Ambient filesystem watcher events, heuristics evaluation, and proactive maintenance signals.
- **[Async Check Discovery Decoupling](./ASYNC_CHECK_DISCOVERY_DECOUPLING.md)**: Decoupling disk discovery from validation pipelines to prevent CLI timeout stalls.
- **[Service Adapter Specification](./SERVICE_ADAPTER_SPECIFICATION.md)**: Unified abstraction across macOS `launchd`, Linux `systemd`, and standalone foreground runners.

---

## 5. Swarms, Orchestration & Extensibility
Documents governing multi-agent coordination, bounded domain packages, and extensible modules.

- **[Pack Composition & Extensibility Architecture](./PACK_COMPOSITION_AND_EXTENSIBILITY.md)**: Architectural distinction between Go Code Packs (`packs/<domain>`) and Runtime Swarm Orchestration Packs (`swarm.yaml`), and composition rules.
- **[Composite Execution Organizer Verification](./COMPOSITE_EXECUTION_ORGANIZER_VERIFICATION.md)**: End-to-end trace pipelines, deterministic goal hierarchy composition, and verification gates.
- **[Adaptive Task Supervision Specification](./ADAPTIVE_TASK_SUPERVISION_SPECIFICATION.md)**: Multi-agent supervisor coordination, heartbeat monitoring, and automated unsticking protocols.
- **[Adaptive Agent Disambiguation](./ADAPTIVE_AGENT_DISAMBIGUATION.md)**: Persona resolution, skill linkage, and intent disambiguation for agent execution seats.
- **[Orchestration Runtime Taxonomy](./ORCHESTRATION_RUNTIME_TAXONOMY.md)**: Classification of swarm coordinators, seat workers, and task queues.
- **[Package Consolidation Governance](./PACKAGE_CONSOLIDATION_GOVERNANCE.md)**: Package boundary hygiene, circular import prevention, and internal vs public API separation.
- **[SpecBuilder CodeGen & Enum Consolidation](./SPECBUILDER_CODEGEN_CONSOLIDATION.md)**: SpecBuilder code generation, AST pruning, and domain enum aggregation.
- **[Synthetic Constant Eradication Governance](./SYNTHETIC_CONSTANT_ERADICATION_GOVERNANCE.md)**: Elimination of stringly-typed magic constants in favor of canonical spec-derived constants.

---

## 6. Security, Sovereign Execution & Cryptography
Documents governing cryptographic provenance, keys, sovereign local execution, and execution isolation.

- **[Sovereign Air-Gapped AI Engineering](./SOVEREIGN_AIRGAPPED_AI_ENGINEERING.md)**: Offline execution with local LLMs (Ollama / Apple Silicon), local CAS persistence, and deterministic AST verification.
- **[Keystore Security & Fallback Specification](./KEYSTORE_SECURITY_AND_FALLBACK_SPECIFICATION.md)**: Ed25519 signing keys, fail-closed key discovery, and environment variable resolution.
- **[Tray Command Cryptographic Security](./TRAY_COMMAND_CRYPTOGRAPHIC_SECURITY.md)**: Authenticated RPC protocol between background daemons and desktop tray applications.

