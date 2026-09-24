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

## Layout

| Path | Role |
|------|------|
| `CONSTITUTION.md` | Shared law: scope, evidence grades, budget density rule, no-fix |
| `DIAMOND_SCALE.md` | Multi-axis “diamond” grading of overall quality |
| `LENSES.md` | Lens catalog + density class (exhaustive vs top-N) |
| `DIAGRAM_CONTRACT.md` | Required diagram types + anchoring rules |
| `WAVE_PLAN.md` | Ordered analysis waves |
| `OPERATOR.md` | One-page run checklist |
| `KICKOFF_PROMPT.md` | Reusable solo-operator paste prompt (A/B comparable) |
| `HANDOFF_SCHEMA.md` | Downstream objectification contract (tool-agnostic) |
| `EXTENSIONS.md` | How to add lenses, languages, project packs |
| `rubrics/` | Per-lens rubrics with citations, pros/cons |
| `prompts/` | Specialist + adversarial prompts per lens |
| `schemas/` | Machine-readable finding / diagram / scorecard schemas |
| `adapters/` | Optional language/project packs (e.g. Go) — **not** the core |
| `templates/` | Example `run_scope.yaml` and future run stubs |

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
