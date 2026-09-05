# Kernel object-kind fitness rubric (7 lenses)

**Last Verified:** 2026-08-31


**Glossary:** `kernel_object_kind_fitness_rubric` (`GLS-1786687875188966000-0ed1e9a6`)  
**Plan:** `REDACTED`  
**Audience:** operators and agents auditing kinds under `docs/process/` (CAS) and elevated/internal kinds.

This rubric turns recurring inspection findings into a **repeatable scorecard** per object kind (and, when useful, per high-volume instance cohort). It applies to **visibility:public** and **visibility:internal** — RBAC may hide the internal bucket from default list/MCP; inspection still uses `zqk-admin` / `object … --internal` (or an entitled elevated session). Neglecting internal kinds is out of scope for “pristine kernel.”

## Dispositions (required outcome per kind)

| Disposition | Meaning |
|-------------|---------|
| `keep_enforce` | Kind stays; close validation/lifecycle gaps so the contract has teeth |
| `remediate` | Kind stays; fix instance population and/or code paths this sprint |
| `quarantine_fixtures` | Separate test/fixture IDs from production membrane (archive/delete/prefix policy) |
| `deprecate_remove` | Kind or cohort is noise — remove from registration membrane / archive instances |

Do **not** leave a kind as “interesting but unused” without a disposition.

## Lenses

### L1 — Lifecycle fitness

**Questions**

1. Do statuses form a coherent story (origin → execution → terminal) with non-empty `description` and `role`?
2. Are transitions real (manual/auto) or decorative?
3. For each non-terminal status: what **actionable validation** fires on enter or while held (preconditions, field gates, system-only statuses)?
4. Are halted/parking statuses (`error`, `escalated`, paused-like) machine-defined (who may bind, who may measure) or prose-only?

**Fail signals:** status with no system meaning; promote/demote with empty preconditions where hollow objects reach execution-facing roles; docs contradict code (e.g. calling a halted status “terminal”).

**Evidence:** lifecycle YAML + builder; promote verbose rejection reasons; CAP/tick/whats-next (or kind-specific) eligibility helpers.

### L2 — Utilization (meaning vs noise)

**Questions**

1. How many instances exist in kernel vs draft plane?
2. Which product/scheduler/CLI paths **read or write** this kind?
3. If underused: is the kind a deferred design, a failed migration, or accidental CAS litter?
4. Would removing the kind (or archiving the cohort) reduce agent confusion?
5. **Draft plane honesty:** is `status=draft` (or any `preliminary: true` origin) ever listable on CAS? That is a membrane lie — draft means `.zqk/object_drafts/`, not “draft-looking CAS.”

**Fail signals:** ontology claims “MUST” but runtime ignores; duplicate sources of truth (e.g. file DNA vs CSPEC); orphan approved instances with no callers; **`status=draft` in `object list` / Layer-0 integrity.**

**Evidence:** `object count`, code references (`Kind…`), generate/bootstrap paths, agent skills/rules.

### L3 — Test / fixture pollution

**Questions**

1. Are titles/IDs clearly fixtures (`Fixture:`, `ACC-TEST`, `GOAL-001`, …) living in production namespaces?
2. Do tests write into the operator CAS tree without cleanup?
3. Is there a quarantine or `ZQK_TEST_ROOT` boundary that actually holds?

**Fail signals:** fixture accounts/preps/skills in `zqk:kernel` list results; tests that create objects without delete; polluting `docs/process/` committed fixtures.

**Evidence:** title/id greps; crevice-sweep BLIs; test helpers; draft-plane inventory.

### L4 — Logic fields must have teeth (no theater)

**Questions**

1. Which fields drive branching (status, flags, roles, refs, thresholds, `delivery_mode`, …)?
2. Is each such field **validated on write** and **honored on read** by the code path that claims to care?
3. Can an agent set the field to a “good looking” value with no behavioral change?

**Fail signals:** status labels with no eligibility matrix; optional fields that docs treat as gates; promote succeeds while required narrative fields are empty under alternate names (`problem` vs `problem_statement`).

**Evidence:** validators, lifecycle preconditions, scheduler/CAP handlers, tests that flip the field and assert behavior.

### L5 — Required-field honesty (spec vs CAS)

**Questions**

1. Spec `validation.required: true` (and lifecycle preconditions) vs actual instance payloads.
2. Can create/promote succeed with missing `title` (or other required fields)?
3. Are “required at creation” placeholders still shipping?

**Fail signals:** empty titles on CAP-minted objects; required lists empty; status advanced without contract fields (CVS without hypothesis, etc.).

**Evidence:** object spec + instance sample; create/promote dry-runs; system check / hygiene scan rules.

### L6 — Duplicative IDs (identity integrity)

**Questions**

1. Does any object ID map to **more than one** CAS blob / path (dual-CAS / hash-duplicate)?
2. Do system-check / Layer-1 / CacheLag findings **list the same ID more than once** in a single issue row (aggregation noise vs real multiplicity)?
3. Are there **semantic duplicates** (same kind + near-identical title/purpose, distinct IDs) that confuse agents?
4. After CAS mutation, does object-id-cache lag produce “exists in storage but missing from cache” for IDs that are already known — and does refresh clear it without hiding true dual-CAS?

**Fail signals:** repeated IDs inside one CacheLag / blocking-excluded list (e.g. same `BLI-…` three times); Tier-1 dual-CAS inventory hits; two hash files for one `id:`; CacheLag that survives `--refresh-cache` / `EnsureObjectIDCacheReady`.

**Evidence:** `zqk system check` Layer 1; dual-CAS inventory / quarantine helpers (POL-CODE-004 family); `rg`/CAS walk by `id:`; object-id-cache rebuild; compare unique vs raw ID counts in check payloads.

**Note:** Deduping *display* of IDs in check output is necessary but not sufficient — always distinguish **report duplication** from **storage duplication**. Nested YAML `id:` keys (e.g. `domains[].id`) must never be treated as the object id — CAS peek is document-level only.

### L7 — Status vocabulary fitness (kind-native words)

**Questions**

1. Do status labels match what the kind *is*? (Goals/requirements = commitments → `active`/`authorized`, not “planned work”; risks = open/mitigating, not “approved document”; criteria = verification state, not `not_started` chore.)
2. Are lifecycle-preliminary objects exclusively on the object draft plane? A preliminary status on listable CAS is Layer-0 integrity, not a soft Layer-1 bucket.
3. When linked neighbors are already execution-facing, does the child/parent stay preliminary without a fail-closed promote path?
4. For `agent_task`: when does `proposed` → `approved` fire, and what fields are validated?
5. Does Layer-3 “complete” mean terminal success for that kind, or is it colliding with other vocabularies?

**Fail signals:** planned goals/REQs that are live program targets; all risks `approved` with no open/mitigating state; CRIT `not_started` for acceptance gates; ATKs forever `proposed` with no approve gate; “complete” on kinds where operators expect `implemented`/`resolved`; any preliminary status returned by normal `object list`.

**Evidence:** lifecycle YAML per kind; Layer-1 preliminary table from `system check`; promote verbose; population status histograms.

## Scorecard template (per kind)

```text
kind: <ontology>
visibility: public|internal
instance_count_kernel: N
lenses:
  L1_lifecycle: pass|fail|ambiguous — note
  L2_utilization: pass|fail|ambiguous — note
  L3_test_pollution: pass|fail|ambiguous — note
  L4_logic_teeth: pass|fail|ambiguous — note
  L5_required_fields: pass|fail|ambiguous — note
  L6_duplicative_ids: pass|fail|ambiguous — note
  L7_status_vocabulary: pass|fail|ambiguous — note
disposition: keep_enforce|remediate|quarantine_fixtures|deprecate_remove
follow_ups: [BLI-… / TRACK]
evidence: [paths, commands, object ids]
```

## Inspection paths

| Bucket | How |
|--------|-----|
| Public / default membrane | `zqk object list\|count <kind>`, dumps, system check |
| Internal / elevated | `zqk-admin` or `zqk object … --internal` (entitlement); do not skip because MCP RBAC blocked the human dump |
| Spec plane | `docs/process/_internal/object_specs/`, lifecycles, `generate-instance-builders` |
| Identity | Layer-1 / CacheLag rows; dual-CAS inventory; unique vs multiset ID counts in check JSON |

## Related work

- Crevice sweep: `REDACTED`
- Lifecycle TDD matrix: `REDACTED`
- CVS status matrix Option A: `REDACTED`
- CLI DNA / command_spec dual-source: `REDACTED`
- Dual-CAS fail-closed / quarantine: `REDACTED` (and POL-CODE-004 family)
- Draft-on-CAS eradication (status=draft must not be listable): `REDACTED`
- `zqk system object-hygiene-scan` — operational prefix/regex rules (complement, not a substitute for this rubric)
- Preliminary-on-CAS invariant and L7 population migration: `REDACTED`
