# Assessment, onboarding, and alpha launch — document index

**Tags (for search/filter):** `assessment`, `quality`, `code`, `architecture`, `non-functional`, `onboarding`, `greenfield`, `alpha`, `CLI`, `security`, `standards`, `convergence`

**`doc_entry` (documentation index):** `DOC-EXAMPLE` — `zqk object get DOC-EXAMPLE` (path = this file).

**`glossary_term` (documentation graph):** `GLS-EXAMPLE` — `zqk object get GLS-EXAMPLE`.

**Local CI:** Verification is **local-first** until a hosted CI is attached to the canonical repo; see **`scripts/README.md`** section *Local verification (no hosted CI yet)* for policy + spec-cell commands.

**Process links (CAS):**

| Object | Role |
|--------|------|
| `PRI-EXAMPLE` | Priority plan — **CLI alpha launch readiness** (includes **data-cell program** / persistence continuity — see **note** on the object) |
| `CVS-EXAMPLE` | Convergence session — **expertise docs, greenfield plan, workspace hygiene** |
| `BLI-EXAMPLE` | Backlog — **data cell program** umbrella (**in progress** on the same PRI; child **`BLI-177608*`** ) |
| `BLI-EXAMPLE` | Backlog — **documentation graph** (manual cross-links → ontology-backed linking) — **Deferred**; **`related_object_refs`:** PRI (plan), **`GLS-EXAMPLE`**, **`DOC-EXAMPLE`**, **`DOC-EXAMPLE`**, **`DOC-EXAMPLE`** |
| `DOC-EXAMPLE` | `doc_entry` — this index file (**[ASSESSMENT_AND_ONBOARDING_INDEX.md](./ASSESSMENT_AND_ONBOARDING_INDEX.md)**) |
| `DOC-EXAMPLE` | `doc_entry` — **[CLI_ALPHA_LAUNCH_PLAN.md](./CLI_ALPHA_LAUNCH_PLAN.md)** (technical alpha plan) |
| `DOC-EXAMPLE` | `doc_entry` — **[BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md](../process/architecture/BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md)** (backlog `document_refs` / auto-link design) |
| `GLS-EXAMPLE` | `glossary_term` — **documentation graph** (navigational edges across markdown and CAS) |

**Repository documents (this folder):**

| Document | Purpose |
|----------|---------|
| [EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md](./EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md) | Good / Bad / Ugly across engineering dimensions; gap roadmap; **recommended** process objects (create via CLI; some rows still aspirational) |
| [GREENFIELD_TEAM_OPERATING_PLAN.md](./GREENFIELD_TEAM_OPERATING_PLAN.md) | Seed standards, gates, diagrams — new team building a zqk-like system from scratch; optional doc-graph backlog `BLI-EXAMPLE` (**deferred**) |
| [CLI_ALPHA_LAUNCH_PLAN.md](./CLI_ALPHA_LAUNCH_PLAN.md) | Technical plan to ship a usable CLI alpha; ties to `PRI-EXAMPLE`; **`doc_entry`** `DOC-EXAMPLE` |
| [ONBOARDING_ROADMAP_AND_CERTIFICATION.md](../process/architecture/ONBOARDING_ROADMAP_AND_CERTIFICATION.md) | Onboarding as objects; per-account completion; certification direction |
| [ONBOARDING_EVALUATION_SCENARIO.md](../process/testing/ONBOARDING_EVALUATION_SCENARIO.md) | Isolated **zqk-ts** + **ZQK_TS_TEST_ROOT** eval runbook (P1 alpha gate); links seed job + templates |
| [scripts/onboarding_roadmap/README.md](../../scripts/onboarding_roadmap/README.md) | Template YAMLs, CLI creation order, **reference pattern for advanced tutorials** |
| [FIRST_RUN_OBJECT_TUTORIAL.md](../onboarding/FIRST_RUN_OBJECT_TUTORIAL.md) | First-run object flow (`template` → `create` → `get` → `update`) with troubleshooting |
| [METRICS_TREASURE_MAP_AND_ALPHA_TIMELINE.md](./METRICS_TREASURE_MAP_AND_ALPHA_TIMELINE.md) | **Treasure map** of system/project/audit metrics; ASCII timeline; **tentative alpha date** (metrics-informed); **§8 operator quick health checklist** (alpha) |
| [MEASUREMENT_OUTCOME_TAXONOMY.md](./MEASUREMENT_OUTCOME_TAXONOMY.md) | **Normative four-outcome model** for validators/vettors (convergence / divergence / halt_error / ambiguous); interim mapping from rollup and `delta_assessment`; implementation **`BLI-EXAMPLE`** |
| [PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md) | Path alias cache, `prefix:` resolution, **`zqk-settings.yaml` merge** over defaults |
| [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) | **Normative** data cell: logical unit + operational envelope; storage profiles (light file, CAS, stream); multi-kind cells |
| [BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md](../process/architecture/BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md) | `document_refs` resolution and future auto-link behavior; **related backlog** `BLI-EXAMPLE` (**deferred**); **`doc_entry`** `DOC-EXAMPLE` |
| [ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md](../process/decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md) | **Accepted ADR:** every data cell requires a spec (declares storage profile); repeatable CLI pipeline incl. field vetting, per-field deploy gate, glossary |
| [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md) | **Implemented slice:** runtime operator config (feature flags, CLI hooks, tray): **single layout + path aliases** (`pkg/datacell`); **`BLI-EXAMPLE`** |

**Related canonical architecture (existing):**

- [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) — spec plane, caches, glossary
- [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) — change discipline (top callout + **§13** post-verify for **semantic density**)
- [DRY_PATTERN_EXTRACTION.md](../best-practices/coding/DRY_PATTERN_EXTRACTION.md) — semantic density, rule of two / three, post-verify narrative
- [AGENT_CONTEXT_REFRESH.md](../process/enforcement/AGENT_CONTEXT_REFRESH.md) — generated compliance, **before implementing** checklist (incl. glossary + **navigation vocabulary graph** pointers), + **after logic is verified** reminder (`scripts/README.md`)
- [VOCABULARY_GRAPH.md](./VOCABULARY_GRAPH.md) — seeded **`vocabulary_scheme`** / **`glossary_term_relation`** (GAPE lenses, VIS-001 pillars, cross-walk); **`list-vocabulary-gtr.sh`**, **`verify-vocabulary-gtr-counts.sh`**
- [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) — CLI vocabulary
- [AGENT_GUIDELINES.md](../process/enforcement/AGENT_GUIDELINES.md) — logging, output, timestamps

**Archive snapshot:** `docs/archive/snapshots/` may contain a zip of `docs/reports/` (see `docs/archive/README.md`).
