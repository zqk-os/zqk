# Framework Extensions & Adapter Architecture

> **Purpose:** Formal specifications for extending the Codebase Evaluation Framework with language-specific adapters, project-specific compliance packs, custom evaluation lenses, and backward-compatible schema evolutions.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Extensibility Model) |
| **Target Roles** | Framework Maintainers, Language Specialists, Security Auditors |
| **Extension Isolation** | Core framework remains strictly runtime- and product-agnostic |

---

## 1. Language Adapter Packs

Language adapters specialize the framework for a specific runtime ecosystem without polluting the core specification. Adapters reside in `adapters/<lang>.md` alongside optional rules:

- `adapters/<lang>/banned_apis.txt` — Known dangerous or deprecated library calls.
- `adapters/<lang>/secrets_patterns.txt` — Ecosystem-specific credential patterns.

### Adapter Content Requirements

Each language adapter specification must declare:

1. **Authoritative Idiom Citations** — Official style guides, language specifications, and canonical texts (such as *Effective Go* or *Effective Java*).
2. **Recommended Automated Tooling** — Preferred static analyzers, linters, AST query tools, and race detectors.
3. **Concurrency & Memory Model Notes** — Language-specific execution semantics, thread synchronization primitives, and memory safety invariants.
4. **Mechanical Scan Mappings** — Concrete configuration mapping `D-HIGH` criteria to AST queries or linter rules.

> [!TIP]
> Refer to [`adapters/go.md`](./adapters/go.md) as a reference implementation of an idiomatic language adapter.

---

## 2. Project & Organizational Packs

Organizations may author project-specific overlay packs to enforce internal policies, compliance frameworks, or launch criteria:

- **Launch Filters:** Triage findings against release readiness thresholds.
- **Compliance Overlays:** Map findings to regulatory regimes (such as SOC 2, HIPAA, or ISO 27001).
- **Monorepo Policies:** Custom rules governing internal boundary checks.

> [!IMPORTANT]
> **Grade Immutability Invariant:** Project packs may filter, prioritize, or annotate findings, but they must never silently overwrite or inflate Diamond Scale axis grades. Organizations must publish derived documents (such as `launch_triage.md`) rather than mutating the baseline truth map.

---

## 3. Authoring Custom Evaluation Lenses

To introduce a new evaluation lens into the framework:

1. **Author Authoritative Rubric:** Create `rubrics/L-<NAME>.md` specifying purpose, criteria, citations, and density classification.
2. **Update Lens Directory:** Add the lens definition and primary diamond axis mappings to [`LENSES.md`](./LENSES.md).
3. **Equip Agent Prompts:** Provide paired prompt templates in `prompts/L-<NAME>/specialist.md` and `prompts/L-<NAME>/adversarial.md`.
4. **Schedule Wave Execution:** Assign the lens to an appropriate wave in [`WAVE_PLAN.md`](./WAVE_PLAN.md).
5. **Version Increment:** Increment the framework minor version.

---

## 4. Schema Evolution & Compatibility

The framework maintains semantic versioning across its data schemas:

| Schema Change Type | Version Impact | Governance Requirement |
| :--- | :---: | :--- |
| Additive fields to finding or scorecard schemas | Minor (`0.x.0`) | Fully backward compatible; existing consumers continue operating without modification. |
| Deprecation or addition of optional quality axes | Minor (`0.x.0`) | Documented in release notes with migration guidance. |
| Renaming or removing core diamond axes | Major (`1.0.0`) | Breaking change requiring consensus approval from framework maintainers. |
| Altering severity or resolution enum values | Major (`1.0.0`) | Breaking change requiring schema validator updates across all tooling. |
