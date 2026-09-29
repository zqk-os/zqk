# CEF Constitution: Binding Rules & Evidence Protocol

> [!NOTE]
> **Purpose:** Non-negotiable constitutional invariants, evidence grades (E0–E3), budget density rules, and zero-fix discipline governing all codebase evaluations. All specialist evaluators, adversarial auditors, and lead integrators must adhere to this specification before producing findings.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Binding) |
| **Target Roles** | Mechanical Preflight, Lens Specialists, Adversarial Auditors, Lead Integrators |
| **Applicability** | Universal (Language-agnostic; independent of host project or product roadmap) |

---

## 1. Intent & Truth Map Principle

The primary goal of the Codebase Evaluation Framework is to produce a **truth map**: an objective, portable, and reproducible assessment of codebase quality across the multidimensional diamond axes (detailed in [`DIAMOND_SCALE.md`](./DIAMOND_SCALE.md)).

Evaluations must remain strictly decoupled from host-specific timelines, product launches, or individual programming language preferences. Context-specific lenses, compliance overlays, and language-specific tooling must be introduced exclusively through **adapter packs** applied after the baseline truth map is established.

---

## 2. Hard Invariants & Analysis Rules

All evaluation passes are bound by the following constitutional rules:

1. **Zero Modifications During Analysis** — Do not edit production code to prove a finding. Proof is reproducible evidence, not an ad-hoc patch.
2. **Zero Silent Invention** — Every recommendation and finding must cite at least one authoritative standard from the citation taxonomy (§5) and attach concrete evidence (§4).
3. **Density-Aware Thoroughness** — Adhere strictly to the budget density classifications (§6): exhaustive for discrete mechanical criteria; ranked Top-N for architectural and philosophical assessments.
4. **Mandatory Adversarial Pairing** — No specialist finding enters the final handoff package without an independent adversarial review and a documented resolution status (§7).
5. **Architectural Legibility** — Major subsystems, non-obvious algorithms, and state machines require referentially anchored diagrams conforming to [`DIAGRAM_CONTRACT.md`](./DIAGRAM_CONTRACT.md).
6. **Tooling Signal Over Speculation** — Leverage available analyzers, linters, AST scanners, and targeted tests when they raise signal. Missing tools must be recorded as tooling gaps rather than excuses for incomplete analysis.
7. **Targeted Test Execution** — Full test suite runs are discouraged during evaluation. Prefer reading test suites as executable documentation. When executing a test is required to understand system behavior, record package, test name, timeout, and rationale.
8. **Portable Domain Vocabulary** — Findings must remain intelligible without host-specific jargon. Host-specific identifiers are permitted exclusively in `local_refs[]`.

---

## 3. Operational Roles & Responsibilities

The evaluation lifecycle separates concerns across four distinct operational roles:

| Operational Role | Lifecycle Phase | Core Responsibility |
| :--- | :--- | :--- |
| `mechanical_preflight` | Wave 0 | Inventory discovery, automated tool execution, and baseline structural metrics. |
| `specialist` | Waves 1–3 | Deep inspection of a single lens using its rubric and evaluator prompt. |
| `adversarial` | Waves 1–3 | Rigorous challenge and falsification pass targeting the specialist artifact. |
| `integrator` | Wave 4 | Resolution arbitration, evidence grade verification, scorecard emission, and handoff assembly. |

> [!IMPORTANT]
> Specialists must not mint tracker or backlog objects directly. Downstream task generation is managed exclusively through [`HANDOFF_SCHEMA.md`](./HANDOFF_SCHEMA.md).

---

## 4. Evidence Grades & Handoff Criteria

Every finding must record an `evidence_grade` and contain one or more `evidence[]` items with `kind` matching: `path_line`, `command_output`, `metric`, `ast_match`, `test_result`, or `external_citation`.

| Evidence Grade | Definition & Verification Requirement | Handoff Eligibility |
| :--- | :--- | :--- |
| `E0` | Subjective opinion, unverified impression, or stylistic preference. | Ineligible (Never permitted). |
| `E1` | General citation to an industry standard, book, or style guide without code anchors. | Informational only (Ineligible for handoff). |
| `E2` | Concrete repository artifact: file path, symbol anchor, AST match, command output, or metric. | Provisional (Permitted in dense lenses; requires human waiver in soft lenses). |
| `E3` | `E2` artifact confirmed by adversarial review (`stand` or `reframe`) with factual agreement. | Canonical (Default handoff standard). |

---

## 5. Citation Taxonomy & Precedence Hierarchy

Every citation in a finding must declare its authoritative `class`:

| Citation Class | Authoritative Sources & Examples |
| :--- | :--- |
| `industry_standard` | OWASP ASVS, NIST SSDF, CISQ, ISO/IEC 25010, Google Style Guides, Effective Go, Bloch's *Effective Java*, Martin's *Clean Architecture*, Kleppmann's *Designing Data-Intensive Applications*, SEI/CMU engineering standards. |
| `language_idiom` | Official language specifications, standard library conventions, language memory models, and recognized community pattern catalogs. |
| `empirical` | Quantifiable coverage measurements, race detector outputs, microbenchmarks, crash reproduction rates, and AST cyclomatic complexity metrics. |
| `project_law` | Host repository conventions, internal lint rules, and project-specific checklists (Applicable only via language or project adapter packs; never required for core CEF). |

> [!NOTE]
> **Precedence Hierarchy:** When establishing portable truth maps, `industry_standard` and `empirical` evidence strictly supersede `project_law`. Internal project conventions explain local exceptions; they do not redefine universal quality standards.

---

## 6. Budget Density & Exhaustiveness Rules

To prevent analysis fatigue and balance thoroughness with execution cost, each lens operates under an assigned density class:

| Density Class | Applicable Scope & Criteria | Thoroughness Requirement |
| :--- | :--- | :--- |
| `D-HIGH` | Discrete, enumerable criteria (banned APIs, magic literals, license headers, secret leak patterns, circular dependencies). | Exhaustive inspection across all declared repository scope roots. |
| `D-MED` | Semi-structured patterns (error handling paths, layering boundary violations, goroutine lifecycles). | Systematic scan with exhaustive sampling of hot paths. Report all severe findings; cap moderate findings at Top-N. |
| `D-LOW` | Philosophical and architectural designs with multiple valid approaches. | Top-N ranking (default N=10) ordered by severity × blast radius × irreversibility. |

The default budget is N=10 unless overridden in the wave execution plan. Evaluators must explicitly declare their active N parameter in the report preamble.

---

## 7. Adversarial Audit Protocol & Resolution Enums

Adversarial auditors systematically challenge each specialist finding against six validation criteria:

1. **False Positive Detection** — Does the reported finding represent intended behavior, a standard idiom, or valid design?
2. **Severity Calibration** — Is the assigned severity proportional to actual blast radius and failure probability?
3. **Root Cause Accuracy** — Does the finding diagnose the fundamental defect rather than a superficial symptom?
4. **Premature Abstraction** — Does the proposed recommendation introduce unnecessary complexity, indirection, or gold-plating?
5. **Lens Boundary Clarity** — Is the finding better categorized and governed under a different evaluation lens?
6. **Evidence Calibration** — Is the declared evidence grade supported by reproducible artifacts?

### Audit Resolutions

Every audited finding must be assigned one of the four mandatory resolution statuses:

| Resolution Enum | Meaning & Criteria | Operational Action |
| :--- | :--- | :--- |
| `stand` | Auditor validates the finding, severity, and evidence. | Finding proceeds to handoff at declared or elevated grade. |
| `downgrade` | Auditor identifies lower actual impact or partial mitigation. | Severity or evidence grade is reduced accordingly. |
| `retract` | Auditor demonstrates finding is a false positive or invalid. | Finding is excluded from the final handoff manifest. |
| `reframe` | Auditor agrees on the core fact but reinterprets root cause. | Finding description and remediation guidance are revised. |

---

## 8. Run Scope Declaration Contract

Prior to Wave 0 preflight, the operator or orchestrator must freeze the evaluation scope in `run_scope.yaml`:

```yaml
cef_version: "0.1.0"
mode: "truth_map"
repo_root: "."
include_globs:
  - "**/*"
exclude_globs:
  - ".git/**"
  - "vendor/**"
  - "node_modules/**"
  - "**/*.pb.go"
languages_detected: []   # Populated during Wave 0 preflight
adapters_enabled: []     # e.g., ["go"]
top_n_default: 10
notes: ""
```

---

## 9. Mandatory Run Output Artifacts

Every completed CEF evaluation run must generate the following standard package artifacts:

| Output Artifact | Format | Description & Purpose |
| :--- | :--- | :--- |
| `scorecard.json` | JSON | Eight-axis Diamond Scale scores, confidence ratings, and top driver finding references. |
| `findings.jsonl` | JSON Lines | Consolidated findings passing evidence filters, conforming to `schemas/finding.schema.json`. |
| `diagrams/` | Mermaid / D2 / SVG | Architectural models anchored to source symbols per [`DIAGRAM_CONTRACT.md`](./DIAGRAM_CONTRACT.md). |
| `adversarial_resolutions.jsonl` | JSON Lines | Documented auditor challenges and resolutions mapped to each finding ID. |
| `tooling_gaps.md` | Markdown | Identified tooling deficiencies and automated scanner gaps discovered during evaluation. |
| `run_log.md` | Markdown | Operational execution journal detailing executed commands, timeouts, waivers, and skipped scope. |

---

## 10. Intellectual Honesty & Assessment Clauses

All participants in the evaluation process must adhere to the following principles of intellectual honesty:

- **Acknowledge Assessment Boundaries** — Explicitly record "unknown or unassessed" rather than projecting fabricated certainty.
- **Recognize Pattern Conflicts** — Report explicit design conflicts (Pattern A versus Pattern B) rather than forcing synthetic architectural purity.
- **Differentiate Completeness from Style** — Classify unfinished or partial implementations under completeness (`CMP`) rather than aesthetic criticism.
- **Objective Depersonalization** — Maintain detached, objective technical rigor. Critique repository artifacts, architecture, and evidence without personal commentary.
