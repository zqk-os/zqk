# ADR: Hard Topic Budget and Documentation Quality Scrutiny Enforcement

## Status
Approved

## Context
As documentation grows across an evolving operating system and knowledge kernel, markdown files tend to accumulate unbounded verbosity, sprawling topic lengths, internal sprint retrospective narratives, and leaky project-specific kernel IDs. When comprehensive documentation packs 14 sub-domains or 50-row tables under a single high-level heading without structural boundaries, reader comprehension suffers and the public portal becomes overwhelming.

Furthermore, loose quality scrutinizers failed to catch topic overflow, invalid acronym expansions, and internal test normalization maps leaking into public portal builds.

## Decision
1. **Hard Character Budgets Per Topic**:
   - Enforce a strict ceiling of **4,000 characters** per high-level topic (`## <Heading>`) for concept, guide, tutorial, and manual documentation.
   - Enforce a ceiling of **8,000 characters** for formal architecture and specification documents (`docs/architecture/`, `docs/specs/`).
   - Escalate topic budget overflow from warnings to hard blocking errors via `fail_on_topic_overflow: true` in `config/gates.yaml`.

2. **Structural Pruning & Promotion Discipline**:
   - Decompose monolithic tables and long sub-sections by promoting `###` subsections into first-class `##` headings, creating clear, modular topics.
   - Curate index documents (e.g., `docs/guides/README.md`) into crisp thematic subsections rather than unbounded dumps of redundant catalog tables.

3. **Exclusion of Internal Engineering Reports**:
   - Exclude internal cycle sprint achievement narratives, defect remediation notes, and test normalization maps (`docs/reports/`) from public portal generation (`scripts/open-core/docs-portal/generate_docs_portal.py`).
   - Add `docs/reports/*` to default vetting exemptions so internal maps never contaminate public verification gates.

4. **Zero Nanosecond Kernel ID Leaks & Zero Contradictory Acronyms**:
   - Fail-close on any leaked raw nanosecond kernel IDs (`BLI-*`, `REQ-*`, `PRI-*`, `GOAL-*`, `TDE-*`, `MIL-*`) in public guides and tutorials.
   - Verify canonical expansions across core acronyms (ATK, CAS, VDS, CAP).

## Consequences
- **Positive**:
  - Documentation remains crisp, bounded, and easily scannable by humans and LLM agents alike.
  - Zero leakage of transient sprint internals or raw database identifiers to public audiences.
  - `zqk-vet --suite docs` runs deterministically in <600ms as an impermeable quality check-valve.
- **Negative**:
  - Authors must be disciplined about structural headings and character budgets when adding new documentation.
