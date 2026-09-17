# Vocabulary schemes and glossary term relations

**Last Verified:** 2026-08-31


## Purpose

First-class **graphs** over glossary content: multiple parallel taxonomies (inference, display, navigation, future external schemes) without overloading a single `parent` field.

## Object kinds

| Kind | ID prefix | Role |
|------|-----------|------|
| `vocabulary_scheme` | `VOC-*` | Names a graph: purpose (`inference`, `display`, `navigation`, `mixed`, `extension`), `context_scope`, optional `summary`, `machine_hints` for tooling contracts. |
| `glossary_term_relation` | `GTR-*` | Directed edge: `scheme_ref` → `VOC-*`, `source_term_ref` / `target_term_ref` / `predicate_ref` → `GLS-*`. Predicate semantics are defined by the referenced glossary term (use `category` + `semantic_tags` on that term, e.g. `predicate_definition`). |
| `glossary_term` (extended) | `GLS-*` | Optional `semantic_tags` list (e.g. `ontology`, `epistemology`, `inference`, `predicate_definition`) for filtering and rendering. |

## Usage sketch

1. Create a **scheme** (`vocabulary_scheme`) with the right `purpose` and `context_scope`.
2. Define **predicate terms** as `glossary_term` rows (tag with `predicate_definition` in `semantic_tags` / `category` as you standardize).
3. Create **edges** (`glossary_term_relation`) scoped to that scheme; `predicate_ref` points at the predicate’s `GLS-*`.

## Seeded navigation schemes (examples)

Parallel taxonomies can sit side by side: **operational** lenses vs **strategic** vision pillars.

| Scheme | `VOC-*` | Notes |
|--------|---------|--------|
| Decision discipline lenses (GAPE) | `VOC-1776420158230348000-c9312f40` | Grounding, Alignment, Provenance, Coherence, Economy under root **Decision discipline lenses**; `purpose: mixed`. |
| Strategic pillars (vision VIS-001) | `VOC-1776423114630317000-13d396bc` | Six strings from `VIS-001` `pillars` under root **Strategic pillars (vision)** (`GLS-1776423318425511000-790eca65`); `purpose: navigation`. Edges reuse the same SKOS-style **broader** predicate term `GLS-1776420207963611000-bdd209a5`. |
| Agent context cross-walk | `VOC-1776423589203738000-93395d65` | Bidirectional **related (SKOS)** (`GLS-1776423594015846000-64c6e28c`) between the strategic root and the GAPE root (`GLS-1776420209472499000-83004aa1`, same term the lens **broader** edges use); `purpose: navigation`. Same scheme also holds **heuristic pillar↔lens** pairs (see below). |

**Expected `glossary_term_relation` counts** (sanity check after edits): crosswalk **14**, strategic **6**, GAPE **5**. Run **`sh ./scripts/verify-vocabulary-gtr-counts.sh`** — exit **0** when all match; override expected values with **`EXPECT_CROSSWALK`**, **`EXPECT_STRATEGIC`**, **`EXPECT_GAPE`** if you extend the graph. Show the three **`vocabulary_scheme`** rows: **`sh ./scripts/show-seeded-vocabulary-schemes.sh`** (optional **`FORMAT=table`**).

### Pillar–lens heuristic links (cross-walk)

Within `VOC-1776423589203738000-93395d65`, each VIS-001 strategic pillar is linked with **related (SKOS)** to one GAPE lens (bidirectional edges for query symmetry). Five lenses cover six pillars: **Grounding** is used twice (distributed knowledge retrieval and self-bootstrapping platform extension). Mapping is a navigation aid, not a formal ontology axiom.

| Pillar (index) | GAPE lens | Rationale (short) |
|----------------|-----------|-------------------|
| 0 — Distributed Knowledge Kernel… | Grounding | Retrieval and graph-backed grounding. |
| 1 — Human-in-the-Loom Governance… | Alignment | Authority, decisions, and alignment. |
| 2 — Modular Agent Architecture… | Economy | Selective loading and resource economy. |
| 3 — IP Security & Provenance… | Provenance | Traceability lens. |
| 4 — Observable Collaboration… | Coherence | Reliable, transparent workflows. |
| 5 — Self-Bootstrapping System… | Grounding | Platform evolution grounded in system state. |

### Example: list cross-walk edges

From repo root (adjust `--format` as needed):

```bash
sh ./scripts/list-vocabulary-crosswalk.sh
# or: sh ./scripts/list-vocabulary-gtr.sh crosswalk
# same filter, explicit:
zqk object list glossary_term_relation \
  --filter scheme_ref=VOC-1776423589203738000-93395d65 \
  --format table
```

List strategic-pillar tree edges (broader/narrower under VIS-001):

```bash
sh ./scripts/list-vocabulary-gtr.sh strategic
zqk object list glossary_term_relation \
  --filter scheme_ref=VOC-1776423114630317000-13d396bc \
  --format table
```

List GAPE lens tree edges:

```bash
sh ./scripts/list-vocabulary-gtr.sh gape
```

Dump all three schemes in one run (section headers):

```bash
sh ./scripts/list-vocabulary-gtr.sh all
```

### Canonical roots (discipline)

Avoid two different `glossary_term` rows with the **same title** playing the same navigation role (for example duplicate “Decision discipline lenses” roots). Edges in one scheme (e.g. cross-walk **`skos:related`**) must reference the **same** `GLS-*` that the tree edges already use (e.g. GAPE **`skos:broader`** targets **`GLS-1776420209472499000-83004aa1`** for the lens root).

### Scripts (quick reference)

| Script | Role |
|--------|------|
| **`scripts/list-vocabulary-gtr.sh`** | List **`glossary_term_relation`** for **`crosswalk`**, **`strategic`**, **`gape`**, or **`all`**; **`counts`** runs the verifier below. |
| **`scripts/list-vocabulary-crosswalk.sh`** | Same as **`list-vocabulary-gtr.sh crosswalk`**. |
| **`scripts/verify-vocabulary-gtr-counts.sh`** | Assert **14 / 6 / 5** GTR totals (env overrides documented above). |
| **`scripts/show-seeded-vocabulary-schemes.sh`** | Print the three **`vocabulary_scheme`** objects. |

## Future-facing

- `purpose: extension` reserves schemes that may later map to SKOS, SHACL, or industry taxonomies without changing the edge shape.
- Query patterns: list relations `--filter scheme_ref=VOC-…`; list terms `--filter` on `semantic_tags` where supported.
- **Drift triage (git grep):** `docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md` — pattern **10** (`scheme_ref` / `vocabulary_scheme_ref` in CAS YAML).

## Process

New kinds are spec-defined under `.zqk/specs/objects/`. After spec edits, run `zqk system generate-field-keys` (PRUNED), `zqk system generate-instance-builders --overwrite`, and `zqk system generate-spec-index` (PRUNED). Use **`zqk system sync-glossary-from-specs`** (dry-run first) when aligning glossary entries with new ontology names.
