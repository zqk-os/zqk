# Expertise in architecture and code quality (the non-functional side of the house)

**Last Verified:** 2026-08-31


**Status:** Assessment (living document)  
**Tags:** `assessment`, `quality`, `code`, `architecture`, `non-functional`, `security`, `observability`  
**Index:** [ASSESSMENT_AND_ONBOARDING_INDEX.md](./ASSESSMENT_AND_ONBOARDING_INDEX.md)  
**Glossary:** `GLS-1776207925199440000-aa236125` — *documentation graph* (`zqk object get GLS-1776207925199440000-aa236125`)

This note is an expert-level **Good / Bad / Ugly** read of the zqk codebase and product concept, plus a **gap roadmap**, **priorities**, and **recommended process objects** (IDs to create later via `zqk` CLI — not created by this document).

---

## Good

| Area | Evidence / observation |
|------|-------------------------|
| **Spec plane** | Object specs, lifecycles, generated builders, and `spec_index` give a single declarative “genome” for kinds and validation. |
| **Storage layering** | CAS for instances, streams for high-volume paths, clear separation in architecture docs (`SPEC_ORIGIN_PLANE`, stream pilot). |
| **CLI discipline** | Command specs + codegen, `FormatOutput` direction, policy docs (`AGENT_GUIDELINES`) reduce ad hoc UX. |
| **Scheduler & observability** | Test bundles, `health.jsonl`, coordinator pipeline — convergence can be measured, not guessed. |
| **Security direction** | Security contexts, keystore patterns, field-level access metadata in specs (materialization still uneven). |
| **Interoperability** | MCP, translation, ontology import — multiple surfaces share the same object model. |

---

## Bad

| Area | Why it hurts |
|------|----------------|
| **Surface area vs. team size** | Compiler, storage, graph, CLI, scheduler, process CAS — integration tax is high for newcomers. |
| **Lifecycle vs. reality** | Plans and backlog can show “complete” while tests or synonyms still drift (e.g. kind alias collisions). |
| **Heavy packages** | Full `cmd/zqk/system` / `pkg/storage` tests require background runners; easy to skip real gates. |
| **Literal / env hygiene** | Ongoing mechanical work (`field_keys`, `zqk_env`, hardcoded strings) competes with feature work. |
| **Documentation sprawl** | Many overlapping reports under `docs/reports/`; discovery cost for “current truth.” |

---

## Ugly

| Symptom | Root risk |
|---------|-----------|
| **Same failure class recurring** | Convergence aborted or not tied to persisted evidence → thrash. |
| **Process YAML edited by hand** | Breaks CAS/content addressing and policy. |
| **Foreground long tests** | Wastes agent/human time; masks scheduler as source of truth for bundles. |
| **Implicit synonyms** | Default kind aliases can collide when new kinds appear (`roadmap` vs `priority_plan` class of bug). |

---

## Additional critical assessments (5+)

1. **Feasibility:** Achievable with disciplined scope slices; danger is **simultaneous** convergence on spec, storage, CLI, and graph without package-level gates.  
2. **Robustness:** Strong in validation and storage invariants; weakest under **partial restarts**, **multi-writer** stream paths, and **cache invalidation** edges.  
3. **Readability:** Go is generally clear; **generated** and **spec-derived** code can obscure control flow — need generated-file headers and “do not edit” discipline.  
4. **Consistency:** Good where codegen and linters enforce; **manual** CLI paths and one-off scripts remain drift vectors.  
5. **Abstraction:** `pkg/objects`, `pkg/specbuilder`, coordinators — healthy; risk of **leaky** shortcuts on hot paths (`LoadFields` on create, etc.).  
6. **Efficiency:** File CAS and indexes are tuned for correctness first; **large-repo** grep/scan cost is a recurring theme.  
7. **DRYness:** Improving via literal convergence and shared helpers; **tests** still duplicate fixtures.  
8. **Observability:** Metrics + audit + scheduler events are strong; **user-facing** progress lines need continued alignment with `AGENT_GUIDELINES`.

---

## High-level roadmap to shore up gaps (priority)

| Priority | Gap | Direction |
|----------|-----|-----------|
| **P0** | **Green test bundles** on touched packages | `zqk scheduler scan-tests --package ./…`; fix compile/vet first. |
| **P0** | **Kind/CLI synonym safety** | Central review of `pkg/kindsynonyms` + storage-loaded synonyms; tests per kind. |
| **P1** | **Alpha CLI** | Single happy path: install → init → first object → list/get (see [CLI_ALPHA_LAUNCH_PLAN.md](./CLI_ALPHA_LAUNCH_PLAN.md)). |
| **P1** | **Doc index** | One index per major concern (this tree + `SPEC_ORIGIN_PLANE`); archive snapshots under `docs/archive/`. |
| **P2** | **Privilege materialization** | Map spec `access` to enforced gates consistently (architecture already calls this out). |
| **P2** | **Documentation graph / automated linking** | Phased move from manual indices and cross-links to ontology-backed suggestions; backlog `[REDACTED-ID]` (**deferred** — capacity on data-cell alpha program). |
| **P2** | **Graph + spec** | Keep orthogonality; no second ontology in stream cells. |
| **P3** | **Vetting matrix / DRY sweeps** | Batch refactors with `go-safe-replace` and small tranches. |

---

## Recommended system objects (document only — create via CLI later)

| Kind | Suggested title / purpose |
|------|---------------------------|
| `requirement` | REQ: CLI alpha — first-run success and safe defaults |
| `criteria` | CRIT: Alpha CLI — documented init path and smoke test green |
| `backlog_item` | BLI: Link to `[REDACTED-ID]` for each alpha workstream (docs, init, security) |
| `backlog_item` | `[REDACTED-ID]` — documentation graph (manual cross-links → automated linking); **deferred** on same priority plan |
| `glossary_term` | GLS: “CLI alpha readiness” definition for humans/agents |
| `decision` | DEC: Deprecation policy for archived reports vs. live docs |

Use **`zqk object create`** / **`update`** with traceability to **`[REDACTED-ID]`** and **`[REDACTED-ID]`** when you create them.

---

## Revision

Revisit after each **major** test-bundle green milestone or when **alpha** scope changes.
