# Codebase Evaluation Framework (CEF) v0

**Purpose:** Produce a cold, harsh, reusable **truth map** of any software codebase — readable, maintainable, testable, reliable, observable, recoverable, secure, and robust — with **cited rubrics**, **evidence-graded findings**, **specialist + adversarial prompt pairs**, and **mandatory architecture diagrams**.

**Non-goals (v0):**
- Not a launch/open-core decision kit (optional later extension).
- Not biased to any one language or product (language packs are **adapters**).
- Not a mandate to fix code during analysis.
- Not exhaustive philosophical debate disguised as pedantry.

**Primary mode:** Truth map (Mode A). Launch triage (Mode B) is deferred.

---

## Quick start

1. Read [`CONSTITUTION.md`](./CONSTITUTION.md) (binding rules for all agents).
2. Skim [`DIAMOND_SCALE.md`](./DIAMOND_SCALE.md) (multi-axis grades).
3. Run Wave 0 mechanical inventory per [`WAVE_PLAN.md`](./WAVE_PLAN.md).
4. Dispatch lens specialists + adversarial twins from [`prompts/`](./prompts/).
5. Emit findings conforming to [`schemas/finding.schema.json`](./schemas/finding.schema.json).
6. Integrator merges **E3** (or waived E2) into [`HANDOFF_SCHEMA.md`](./HANDOFF_SCHEMA.md) for downstream process systems (ZQK or otherwise).

---

## Framework Groupings & Architecture

The Codebase Evaluation Framework is organized into four clean, functional tiers:

### 1. Framework Core & Governance

| Document | Focus Area & Purpose |
| :--- | :--- |
| **[`CONSTITUTION.md`](./CONSTITUTION.md)** | Non-negotiable constitutional invariants, evidence grades (E0–E3), budget density rules, and zero-fix discipline. |
| **[`DIAMOND_SCALE.md`](./DIAMOND_SCALE.md)** | Multi-axis Diamond Scale grading model across all 8 architectural quality dimensions. |
| **[`WAVE_PLAN.md`](./WAVE_PLAN.md)** | Multi-wave phased execution plan (Preflight → Foundations → Runtime → Craftsmanship → Synthesis). |
| **[`LENSES.md`](./LENSES.md)** | Full lens catalog, density classes (D-HIGH exhaustive vs D-LOW Top-N), and ownership boundaries. |
| **[`DIAGRAM_CONTRACT.md`](./DIAGRAM_CONTRACT.md)** | Mandatory architectural illumination, Mermaid diagram types, and concrete symbol anchoring rules. |
| **[`OPERATOR.md`](./OPERATOR.md)** | Single-page operator runbook, preflight checklist, and execution guidelines. |
| **[`KICKOFF_PROMPT.md`](./KICKOFF_PROMPT.md)** | Standardized, deterministic solo-operator kickoff prompt for A/B comparable evaluations. |
| **[`HANDOFF_SCHEMA.md`](./HANDOFF_SCHEMA.md)** | Downstream objectification and remediation handoff contract for process systems. |
| **[`EXTENSIONS.md`](./EXTENSIONS.md)** | Framework extension architecture for adding custom lenses, compliance overlays, and language packs. |
| **[`adapters/go.md`](./adapters/go.md)** | Go language-specific analysis adapter, tooling heuristics, and AST scanner contracts. |

---

### 2. Evaluation Lenses Matrix (Rubrics & Agent Prompt Pairs)

Each evaluation dimension is governed by an authoritative rubric and paired with a specialist evaluator and an adversarial auditor:

| Lens Code | Focus Dimension | Evaluation Rubric | Specialist Prompt | Adversarial Auditor Prompt |
| :--- | :--- | :--- | :--- | :--- |
| **`L-PREFLIGHT`** | Mechanical Inventory & Tooling | [`rubrics/L-PREFLIGHT.md`](./rubrics/L-PREFLIGHT.md) | — *(Handled in Kickoff)* | — |
| **`L-ARCHITECTURE`** | Architecture & Package Boundaries | [`rubrics/L-ARCHITECTURE.md`](./rubrics/L-ARCHITECTURE.md) | [`prompts/L-ARCHITECTURE/specialist.md`](./prompts/L-ARCHITECTURE/specialist.md) | [`prompts/L-ARCHITECTURE/adversarial.md`](./prompts/L-ARCHITECTURE/adversarial.md) |
| **`L-CODE-QUALITY`** | Code Quality & Craftsmanship | [`rubrics/L-CODE-QUALITY.md`](./rubrics/L-CODE-QUALITY.md) | [`prompts/L-CODE-QUALITY/specialist.md`](./prompts/L-CODE-QUALITY/specialist.md) | [`prompts/L-CODE-QUALITY/adversarial.md`](./prompts/L-CODE-QUALITY/adversarial.md) |
| **`L-CONCURRENCY`** | Concurrency & Race Safety | [`rubrics/L-CONCURRENCY.md`](./rubrics/L-CONCURRENCY.md) | [`prompts/L-CONCURRENCY/specialist.md`](./prompts/L-CONCURRENCY/specialist.md) | [`prompts/L-CONCURRENCY/adversarial.md`](./prompts/L-CONCURRENCY/adversarial.md) |
| **`L-DOCS-MODEL`** | Documentation & Domain Model | [`rubrics/L-DOCS-MODEL.md`](./rubrics/L-DOCS-MODEL.md) | [`prompts/L-DOCS-MODEL/specialist.md`](./prompts/L-DOCS-MODEL/specialist.md) | [`prompts/L-DOCS-MODEL/adversarial.md`](./prompts/L-DOCS-MODEL/adversarial.md) |
| **`L-OBSERVABILITY`** | Observability & Diagnostics | [`rubrics/L-OBSERVABILITY.md`](./rubrics/L-OBSERVABILITY.md) | [`prompts/L-OBSERVABILITY/specialist.md`](./prompts/L-OBSERVABILITY/specialist.md) | [`prompts/L-OBSERVABILITY/adversarial.md`](./prompts/L-OBSERVABILITY/adversarial.md) |
| **`L-PERFORMANCE`** | Performance & Complexity | [`rubrics/L-PERFORMANCE.md`](./rubrics/L-PERFORMANCE.md) | [`prompts/L-PERFORMANCE/specialist.md`](./prompts/L-PERFORMANCE/specialist.md) | [`prompts/L-PERFORMANCE/adversarial.md`](./prompts/L-PERFORMANCE/adversarial.md) |
| **`L-RELIABILITY`** | Reliability & Crash Recovery | [`rubrics/L-RELIABILITY.md`](./rubrics/L-RELIABILITY.md) | [`prompts/L-RELIABILITY/specialist.md`](./prompts/L-RELIABILITY/specialist.md) | [`prompts/L-RELIABILITY/adversarial.md`](./prompts/L-RELIABILITY/adversarial.md) |
| **`L-SECURITY`** | Security & Threat Modeling | [`rubrics/L-SECURITY.md`](./rubrics/L-SECURITY.md) | [`prompts/L-SECURITY/specialist.md`](./prompts/L-SECURITY/specialist.md) | [`prompts/L-SECURITY/adversarial.md`](./prompts/L-SECURITY/adversarial.md) |
| **`L-SUPPLY-RELEASE`** | Supply Chain & Packaging | [`rubrics/L-SUPPLY-RELEASE.md`](./rubrics/L-SUPPLY-RELEASE.md) | [`prompts/L-SUPPLY-RELEASE/specialist.md`](./prompts/L-SUPPLY-RELEASE/specialist.md) | [`prompts/L-SUPPLY-RELEASE/adversarial.md`](./prompts/L-SUPPLY-RELEASE/adversarial.md) |
| **`L-TESTING`** | Test Strategy & Invariants | [`rubrics/L-TESTING.md`](./rubrics/L-TESTING.md) | [`prompts/L-TESTING/specialist.md`](./prompts/L-TESTING/specialist.md) | [`prompts/L-TESTING/adversarial.md`](./prompts/L-TESTING/adversarial.md) |
| **`L-USABILITY`** | Developer Usability & Ergonomics | [`rubrics/L-USABILITY.md`](./rubrics/L-USABILITY.md) | [`prompts/L-USABILITY/specialist.md`](./prompts/L-USABILITY/specialist.md) | [`prompts/L-USABILITY/adversarial.md`](./prompts/L-USABILITY/adversarial.md) |
| **`L-INTEGRATOR`** | Multi-Agent Quality Synthesis | — | [`prompts/L-INTEGRATOR/specialist.md`](./prompts/L-INTEGRATOR/specialist.md) | — *(Final Synthesis & Judgment)* |

> [!NOTE]
> All evaluation prompts share common operational directives defined in [`prompts/_SHARED_PREAMBLE.md`](./prompts/_SHARED_PREAMBLE.md).

---

### 3. Historical Audit Reports & Verification Evidence

Completed multi-dimensional evaluation runs and verified evidence reports are maintained in [`docs/eval/`](../../eval/README.md):
- **[Maintainability Audit Report (MNT)](../../eval/MNT-code-quality-maintainability.md)**: Package consolidation, DRY compliance, and technical debt analysis.
- **[Observability Audit Report (OBS)](../../eval/OBS-observability-diagnostics.md)**: Structured logging, change journals, and Prometheus metric telemetry.
- **[Package Boundaries Audit Report (RDB)](../../eval/RDB-architecture-package-boundaries.md)**: Layering purity, cyclic graph checks, and module decoupling.
- **[Reliability & Crash Recovery Report (REL)](../../eval/REL-reliability-error-recovery.md)**: WAL compaction, CAS assertions, and lock safety invariants.
- **[Security Threat Vectors Report (SEC)](../../eval/SEC-security-threat-vectors.md)**: Cryptographic integrity, CAS isolation, and privilege escalation guards.
- **[Test Strategy Invariant Proofs (TST)](../../eval/TST-test-strategy-invariant-proofs.md)**: Test-to-criteria lineage, race condition elimination, and DoD compliance.
- **[Diamond Scale Convergence Synthesis](../../eval/SYNTHESIS-diamond-envelope-convergence.md)**: Overall quality convergence scorecard and release certification.

---

## Relationship to other quality artifacts (this repo)

If present in a host project, these are **optional sensors**, not CEF itself:

- File-level vetting matrices / VDS profiles under `docs/quality/`
- Project policies, checklists, linters, AST tools

CEF must remain copyable to a non-ZQK tree and still make sense.

---

## Versioning

- **cef_version:** `0.1.0`
- Breaking changes to finding schema or diamond axes require a minor/major bump and a short changelog entry in this README.

### Changelog

| Version | Date | Notes |
|---------|------|-------|
| 0.1.0 | 2026-08-13 | Initial framework: constitution, diamond scale, lenses, rubrics, specialist/adversarial prompts, wave plan, handoff, extensions, schemas |
