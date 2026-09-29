# Diamond Scale: Multi-Axis Quality Grading Model

> [!NOTE]
> **Purpose:** Multidimensional architectural quality grading system separating independent software engineering concerns into eight primary vector axes and a five-tier evaluation ladder.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Grading Engine) |
| **Target Roles** | Lead Integrators, Lens Specialists, Technical Program Managers |
| **Foundational Standards** | ISO/IEC 25010 Product Quality Model, Gemological Multi-Axis Evaluation Principles |

---

## 1. Quality Axes Overview

True engineering quality cannot be collapsed into a single scalar score or marketing slogan. In gemological practice, independent attributes (such as the GIA cut, color, clarity, and carat weight) are assessed separately to ensure objective appraisal. Similarly, software quality requires evaluating independent, often competing dimensions derived from the ISO/IEC 25010 product quality standard.

### Core Diamond Axes

| Axis ID | Dimension Name | Evaluation Invariant |
| :--- | :--- | :--- |
| `RDB` | Readability | Can a skilled engineer unfamiliar with the codebase understand architectural intent without tribal knowledge? |
| `MNT` | Maintainability | Can changes and refactors land safely with local reasoning and bounded blast radius? |
| `TST` | Testability | Can system behavior and invariants be locked with fast, deterministic, reproducible automated tests? |
| `REL` | Reliability | Does the system behave correctly and consistently under expected load, edge cases, and component failures? |
| `OBS` | Observability | Can operators inspect runtime state, trace causality, and diagnose failure modes without attaching debuggers? |
| `RCV` | Recoverability | Can operators and automation recover state, heal partitions, and resume consistent execution after unexpected faults? |
| `SEC` | Security | Are confidentiality, integrity, authorization barriers, and abuse resistance engineered by default? |
| `ROB` | Robustness | Does the system fail closed, bound resource consumption, and reject malformed inputs deterministically? |

### Optional Extension Axes

Optional axes must be reported alongside the core eight and never buried within other scores:

| Axis ID | Dimension Name | Applicability & Criteria |
| :--- | :--- | :--- |
| `CMP` | Completeness | Advertised public interface surface versus actual implemented and tested reality. |
| `OPS` | Operability | Ease of installation, configuration, operational upgrade, and administrative diagnostics. |
| `MOD` | Modularity | Boundary clarity, decoupling strength, and the mechanical cost to replace or extract a subsystem. |

---

## 2. Grade Ladder & Calibration Definitions

Each axis is assigned an integer rating between 1 and 5, accompanied by an explicit confidence factor between 0.0 and 1.0:

| Grade | Rating Tier | Operational Definition & Criteria |
| :---: | :--- | :--- |
| `5` | Flawless (Exhibition) | Exemplary engineering under the axis rubric. Negligible material findings; comprehensive evidence and test proofs. |
| `4` | Fine (Production-Grade) | Solid professional quality with minor contained issues. Zero systemic failure modes or critical architectural defects. |
| `3` | Commercial (Acceptable Debt) | Fully deployable with documented technical debts. Mixed architectural patterns with bounded, manageable risks. |
| `2` | Rough (High Change Risk) | Significant structural vulnerabilities, brittle contracts, or high regression risk. Weak automated verification. |
| `1` | Cull (Untrusted) | Dangerous, unstable, or opaque under this axis. System must not be trusted in production without major remediation. |

> [!NOTE]
> **ISO/IEC 25010 Alignment:** While ISO/IEC 25010 informs which quality attributes are evaluated, the 1–5 grading ladder represents the operational scoring mechanism for agentic and human evaluators.

---

## 3. Scorecard Operational Rules

Lead integrators must observe the following rules when constructing the evaluation scorecard:

1. **Independent Evaluation** — Grade every axis strictly independently. Never average a critical security defect (`SEC=1`) with high readability (`RDB=5`).
2. **Mandatory Confidence Disclosure** — Scant or ambiguous evidence yields a low confidence score, not an inflated middle grade.
3. **Traceable Top Drivers** — Every axis score must cite up to five specific finding IDs that directly influenced the grade.
4. **Explicit Tradeoff Documentation** — When two dimensions trade off (such as fail-closed robustness versus ergonomic usability), explicitly document the architectural tradeoff in the axis notes.
5. **Radar Projection Standard** — The framework publishes the complete multidimensional radar vector. Composite single-number GPA summaries are strictly disallowed.

### Scorecard Data Schema

The canonical scorecard format conforms to `schemas/scorecard.schema.json`:

```json
{
  "cef_version": "0.1.0",
  "axes": {
    "RDB": {
      "grade": 3,
      "confidence": 0.7,
      "drivers": ["F-CODE-001", "F-DOCS-004"],
      "notes": "Clear core abstractions but inconsistent naming conventions in legacy adapters."
    },
    "MNT": {
      "grade": 2,
      "confidence": 0.8,
      "drivers": ["F-ARCH-002"],
      "notes": "Tight coupling between transport and storage layers."
    }
  },
  "optional_axes": {},
  "integrator": "role:integrator",
  "assessed_at": "2026-09-29T16:00:00Z"
}
```

---

## 4. Architectural Analysis & Tradeoffs

| Advantage | Consideration & Risk Mitigation |
| :--- | :--- |
| Decouples cosmetic readability from runtime safety and crash recoverability. | Mitigates halo-effect bias by requiring independent adversarial auditing per lens. |
| Fully portable across languages, runtimes, and distributed architectures. | Coarse 1–5 scale requires mandatory finding driver citations to explain nuances. |
| Adheres to internationally recognized ISO/IEC 25010 quality taxonomy. | Clarifies that evaluations provide objective diagnostic truth maps rather than marketing badges. |
| Exposes completeness and operability explicitly as first-class metrics. | Prevents silent omissions of deployment and operational usability concerns. |
