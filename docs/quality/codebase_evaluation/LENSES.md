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
| `L-PREFLIGHT` | Mechanical Preflight & Inventory | `D-HIGH` | `CMP`, `MOD` | [`rubrics/L-PREFLIGHT.md`](./rubrics/L-PREFLIGHT.md) | Handled in Wave 0 Preflight |
| `L-ARCHITECTURE` | Architecture & Package Boundaries | `D-LOW` | `MNT`, `MOD`, `RDB` | [`rubrics/L-ARCHITECTURE.md`](./rubrics/L-ARCHITECTURE.md) | [`prompts/L-ARCHITECTURE/`](./prompts/L-ARCHITECTURE/) |
| `L-CODE-QUALITY` | Code Quality & Craftsmanship | `D-MED` / `D-HIGH`* | `RDB`, `MNT` | [`rubrics/L-CODE-QUALITY.md`](./rubrics/L-CODE-QUALITY.md) | [`prompts/L-CODE-QUALITY/`](./prompts/L-CODE-QUALITY/) |
| `L-SECURITY` | Security & Threat Modeling | `D-MED` | `SEC`, `ROB` | [`rubrics/L-SECURITY.md`](./rubrics/L-SECURITY.md) | [`prompts/L-SECURITY/`](./prompts/L-SECURITY/) |
| `L-RELIABILITY` | Reliability & Crash Recovery | `D-MED` | `REL`, `ROB`, `RCV` | [`rubrics/L-RELIABILITY.md`](./rubrics/L-RELIABILITY.md) | [`prompts/L-RELIABILITY/`](./prompts/L-RELIABILITY/) |
| `L-OBSERVABILITY` | Observability & Diagnostics | `D-MED` | `OBS`, `OPS`, `RCV` | [`rubrics/L-OBSERVABILITY.md`](./rubrics/L-OBSERVABILITY.md) | [`prompts/L-OBSERVABILITY/`](./prompts/L-OBSERVABILITY/) |
| `L-TESTING` | Test Strategy & Invariant Proofs | `D-MED` | `TST`, `REL` | [`rubrics/L-TESTING.md`](./rubrics/L-TESTING.md) | [`prompts/L-TESTING/`](./prompts/L-TESTING/) |
| `L-USABILITY` | Developer Ergonomics & UX | `D-LOW` | `OPS`, `RDB`, `CMP` | [`rubrics/L-USABILITY.md`](./rubrics/L-USABILITY.md) | [`prompts/L-USABILITY/`](./prompts/L-USABILITY/) |
| `L-SUPPLY-RELEASE` | Supply Chain & Packaging | `D-HIGH` | `SEC`, `OPS`, `RCV` | [`rubrics/L-SUPPLY-RELEASE.md`](./rubrics/L-SUPPLY-RELEASE.md) | [`prompts/L-SUPPLY-RELEASE/`](./prompts/L-SUPPLY-RELEASE/) |
| `L-PERFORMANCE` | Performance & Resource Bounds | `D-MED` | `REL`, `ROB` | [`rubrics/L-PERFORMANCE.md`](./rubrics/L-PERFORMANCE.md) | [`prompts/L-PERFORMANCE/`](./prompts/L-PERFORMANCE/) |
| `L-CONCURRENCY` | Concurrency & Race Safety | `D-MED` | `ROB`, `REL`, `RCV` | [`rubrics/L-CONCURRENCY.md`](./rubrics/L-CONCURRENCY.md) | [`prompts/L-CONCURRENCY/`](./prompts/L-CONCURRENCY/) |
| `L-DOCS-MODEL` | Documentation & Mental Model | `D-LOW` | `RDB`, `OPS`, `CMP` | [`rubrics/L-DOCS-MODEL.md`](./rubrics/L-DOCS-MODEL.md) | [`prompts/L-DOCS-MODEL/`](./prompts/L-DOCS-MODEL/) |

> [!NOTE]
> **Density Split for `L-CODE-QUALITY`:** Discrete scans (such as hardcoded literals, secret leaks, and banned APIs) are executed at `D-HIGH` exhaustive density. Broad code style, formatting philosophies, and naming patterns are evaluated at `D-LOW` Top-N density.

---

## 2. Architectural Analysis & Governance

| Evaluation Benefit | Operational Risk & Governance Invariant |
| :--- | :--- |
| Comprehensive coverage of ISO/IEC 25010 and real-world production engineering disciplines. | Overlap between concurrency, reliability, and performance is arbitrated and deduped by the lead integrator in Wave 4. |
| Budget density classes prevent agents from producing unproductive, speculative essays. | Wave execution plans freeze density classes to prevent evaluators from arbitrarily downgrading thoroughness. |
| Decouples superficial code appearance from production operability, observability, and recoverability. | Developer usability (`L-USABILITY`) remains mandatory for systems with CLI, API, or configuration interfaces. |
