# Evaluation Lens Catalog & Density Matrix

> **Purpose:** Comprehensive directory of the twelve evaluation lenses, mapping each assessment focus to its budget density class, primary Diamond Scale quality axes, authoritative rubrics, and agent prompts.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Lens Directory) |
| **Target Roles** | Mechanical Operators, Lens Specialists, Adversarial Auditors, Integrators |
| **Foundational Standards** | ISO/IEC 25010, OWASP ASVS, Google SRE Reliability Engineering |

---

## 1. Lens Directory & Density Classifications

Each evaluation lens targets a discrete architectural dimension and operates under an assigned budget density class (governed by Constitution §6) to calibrate thoroughness against operational budget:

| Lens ID | Focus Dimension | Density Class | Primary Axes | Authoritative Rubric | Agent Prompt Pairs |
| :--- | :--- | :---: | :--- | :--- | :--- |
| **L-PREFLIGHT** | Mechanical Preflight & Inventory | D-HIGH | CMP · MOD | [L-PREFLIGHT Rubric](./rubrics/L-PREFLIGHT.md) | Preflight Automated (Wave 0) |
| **L-ARCHITECTURE** | Architecture & Package Boundaries | D-LOW | MNT · MOD · RDB | [L-ARCHITECTURE Rubric](./rubrics/L-ARCHITECTURE.md) | [Specialist](./prompts/L-ARCHITECTURE/specialist.md) · [Auditor](./prompts/L-ARCHITECTURE/adversarial.md) |
| **L-CODE-QUALITY** | Code Quality & Craftsmanship | D-MED / D-HIGH | RDB · MNT | [L-CODE-QUALITY Rubric](./rubrics/L-CODE-QUALITY.md) | [Specialist](./prompts/L-CODE-QUALITY/specialist.md) · [Auditor](./prompts/L-CODE-QUALITY/adversarial.md) |
| **L-SECURITY** | Security & Threat Modeling | D-MED | SEC · ROB | [L-SECURITY Rubric](./rubrics/L-SECURITY.md) | [Specialist](./prompts/L-SECURITY/specialist.md) · [Auditor](./prompts/L-SECURITY/adversarial.md) |
| **L-RELIABILITY** | Reliability & Crash Recovery | D-MED | REL · ROB · RCV | [L-RELIABILITY Rubric](./rubrics/L-RELIABILITY.md) | [Specialist](./prompts/L-RELIABILITY/specialist.md) · [Auditor](./prompts/L-RELIABILITY/adversarial.md) |
| **L-OBSERVABILITY** | Observability & Diagnostics | D-MED | OBS · OPS · RCV | [L-OBSERVABILITY Rubric](./rubrics/L-OBSERVABILITY.md) | [Specialist](./prompts/L-OBSERVABILITY/specialist.md) · [Auditor](./prompts/L-OBSERVABILITY/adversarial.md) |
| **L-TESTING** | Test Strategy & Invariant Proofs | D-MED | TST · REL | [L-TESTING Rubric](./rubrics/L-TESTING.md) | [Specialist](./prompts/L-TESTING/specialist.md) · [Auditor](./prompts/L-TESTING/adversarial.md) |
| **L-USABILITY** | Developer Ergonomics & UX | D-LOW | OPS · RDB · CMP | [L-USABILITY Rubric](./rubrics/L-USABILITY.md) | [Specialist](./prompts/L-USABILITY/specialist.md) · [Auditor](./prompts/L-USABILITY/adversarial.md) |
| **L-SUPPLY-RELEASE** | Supply Chain & Packaging | D-HIGH | SEC · OPS · RCV | [L-SUPPLY-RELEASE Rubric](./rubrics/L-SUPPLY-RELEASE.md) | [Specialist](./prompts/L-SUPPLY-RELEASE/specialist.md) · [Auditor](./prompts/L-SUPPLY-RELEASE/adversarial.md) |
| **L-PERFORMANCE** | Performance & Resource Bounds | D-MED | REL · ROB | [L-PERFORMANCE Rubric](./rubrics/L-PERFORMANCE.md) | [Specialist](./prompts/L-PERFORMANCE/specialist.md) · [Auditor](./prompts/L-PERFORMANCE/adversarial.md) |
| **L-CONCURRENCY** | Concurrency & Race Safety | D-MED | ROB · REL · RCV | [L-CONCURRENCY Rubric](./rubrics/L-CONCURRENCY.md) | [Specialist](./prompts/L-CONCURRENCY/specialist.md) · [Auditor](./prompts/L-CONCURRENCY/adversarial.md) |
| **L-DOCS-MODEL** | Documentation & Mental Model | D-LOW | RDB · OPS · CMP | [L-DOCS-MODEL Rubric](./rubrics/L-DOCS-MODEL.md) | [Specialist](./prompts/L-DOCS-MODEL/specialist.md) · [Auditor](./prompts/L-DOCS-MODEL/adversarial.md) |

> [!NOTE]
> **Density Split for `L-CODE-QUALITY`:** Discrete scans (such as hardcoded literals, secret leaks, and banned APIs) are executed at `D-HIGH` exhaustive density. Broad code style, formatting philosophies, and naming patterns are evaluated at `D-LOW` Top-N density.

---

## 2. Lens Catalog Governance & Principles

| Design Advantage | Operational Risk & Invariant |
| :--- | :--- |
| Comprehensive coverage of ISO/IEC 25010 and real-world production engineering disciplines. | Overlap between concurrency, reliability, and performance is arbitrated and deduped by the lead integrator in Wave 4. |
| Budget density classes prevent agents from producing unproductive, speculative essays. | Wave execution plans freeze density classes to prevent evaluators from arbitrarily downgrading thoroughness. |
| Decouples superficial code appearance from production operability, observability, and recoverability. | Developer usability (`L-USABILITY`) remains mandatory for systems with CLI, API, or configuration interfaces. |
