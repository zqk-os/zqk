# Diagram Contract & Architectural Anchoring

> [!NOTE]
> **Purpose:** Rigorous architectural illumination contract mandating referentially anchored diagrams, falsifiable claims, and sequence failure paths across major subsystems.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Evidence Standard) |
| **Target Roles** | Lens Specialists, Adversarial Auditors, System Architects |
| **Foundational Standards** | Simon Brown's C4 Model, Mermaid Specification, ISO/IEC 25010 Understandability |

---

## 1. Mandatory Architectural Invariants

In the Codebase Evaluation Framework, diagrams are **reproducible evidence**, not cosmetic decoration. Every diagram must make non-obvious runtime structures legible and must be referentially anchored to concrete symbols within the codebase.

Diagrams are required under the following conditions:

| Evaluation Scope | Lifecycle Phase | Mandatory Diagram Types |
| :--- | :--- | :--- |
| Repository Truth Map | Wave 1 | System Context (C4-L1) and Container Architecture (C4-L2). |
| Major Subsystems | Wave 1+ | Component Architecture (C4-L3) or Module Dependency Graph. |
| Cross-Service Protocols | Waves 2–3 | Paired Sequence Diagrams: Happy Path and Fail-Closed / Error Recovery Path. |
| Non-Obvious Algorithms | Waves 1–3 | State Machine Transition Diagram or Algorithmic Flowchart with rationale. |
| Concurrency & Locks | Wave 2 | Interaction Sequence or Collaboration Diagram identifying locks and queues. |

> [!NOTE]
> **Major Subsystem Definition:** Any package, module, or directory owning a distinct architectural responsibility (such as storage, API, scheduler, consensus, authentication, or UI) with >1,000 lines of code or explicit domain boundaries.

---

## 2. Referentially Anchored Metadata Contract

To prevent speculative abstraction and model hallucinations, every diagram file must include an authoritative YAML metadata frontmatter block:

```yaml
diagram_id: D-STORAGE-01
type: c4_container
title: "Content-Addressable Storage and Write-Ahead Log Subsystem"
anchors:
  - path: pkg/storage/file_object_storage.go
    symbol: WriteObject
    note: "Primary write path committing CAS blobs"
  - path: pkg/storage/wal/wal.go
    symbol: AppendEntry
    note: "Write-ahead journal serialization"
claims:
  - "Writes are committed to the write-ahead journal before content indexing."
  - "Index lookups fail closed on cryptographic hash mismatch."
evidence_grade: E2
```

### Verification Criteria

- **Concrete Symbol Anchors** — All declared anchors must resolve to physical file paths and exported symbols in the audited commit. Unanchored diagrams remain at evidence grade `E1` and cannot proceed to handoff.
- **Falsifiable Claims** — Every diagram must state explicit, falsifiable architectural assertions. Adversarial auditors specifically test and challenge these claims.

---

## 3. Supported Notation & Diagram Technologies

Evaluators must adhere to the following technological priority order:

1. **Native Mermaid in Markdown** — Standard format for portable, pull-request-reviewable architectural diagrams rendered natively across documentation portals.
2. **Declarative D2 Specifications** — Preferred for complex dependency graphs and topological layouts.
3. **Structured ASCII Art** — Fallback representation when automated rendering engines are unavailable in execution environments.
4. **External Modeling Engines** — PlantUML or Structurizr may only be used when explicitly authorized by the operator; otherwise recorded as a tooling gap.

---

## 4. Sequence Diagram Invariants

When modeling inter-process, network, or asynchronous workflows, sequence diagrams must satisfy the following minimum criteria:

- **Literal Entity Naming** — Actor, service, and component names must correspond exactly to structs, packages, or processes in the source code.
- **Explicit Protocol Messages** — Request, response, and event labels must reflect real API endpoints, method signatures, or channel types.
- **Mandatory Failure Paths** — Every happy-path sequence must be paired with an explicit error, timeout, or cancellation flow.
- **Concurrency & Timeouts** — Diagrams must indicate explicit timeout values, retry limits, and mutex boundaries documented in the implementation.

---

## 5. Architectural Tradeoffs & Anti-Hallucination Controls

| Advantage | Consideration & Risk Mitigation |
| :--- | :--- |
| Forces evaluators to inspect real implementations rather than imagining high-level boxes. | Mandatory AST anchor validation prevents synthetic architecture generation. |
| C4 modeling scales consistently from high-level context down to individual components. | Evaluators must focus on complex hotspots to prevent budget exhaustion on trivial CRUD handlers. |
| Explicit fail-path sequences systematically expose reliability and error recovery vulnerabilities. | Diagram metadata includes commit Git SHA to prevent drift across continuous refactors. |
