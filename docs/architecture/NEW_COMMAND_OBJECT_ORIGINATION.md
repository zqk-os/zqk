# `zqk new` — standard object origination pipeline

**Last Verified:** 2026-08-31


**Status:** Implemented (CLI). Process traceability: requirement + doc_entry + criteria in process data (via `zqk object create`).

## Problem

Creating spec-backed objects and scenario bundles today requires knowing field shapes, valid enums, and the right create/apply flows. That friction slows humans and agents.

## Pattern

### Instance mint (`new object`)

1. **`zqk new object <kind> --title "…"`** — persists kind+title onto the **object draft plane** (`.zqk/object_drafts/<kind>/<shard>/<id>.yaml`), no CAS.
2. **Stay on draft plane by default** — fill fields via update; promote when ready (`zqk object promote` or mint with `--promote`).
3. **Opt-in background promote** — `--promote` enqueues `SCH-mint-promote-<id>`. On success, materialize into CAS → list/count/filter.
4. **Get-by-id** dual-reads the draft plane before promote; normal list omits drafts.

See [CAS_CREATE_MEMBRANE_FLOW.md](./CAS_CREATE_MEMBRANE_FLOW.md).

**Why promote is opt-in:** mint only guarantees title+kind (and origin status). Promote tries the next lifecycle status and **re-validates** the object. Missing required fields / checklist / refs for that next status cause promote to **stick** (fail closed on draft plane). Auto-enqueue would mostly schedule noise.

### YAML scaffolds (not mint)

- **`zqk object template <kind>`** — printable YAML from specs (edit offline; then create/mint as appropriate).
- **`zqk new object-spec` / `new bundle`** — write under **`.zqk/drafts/`** (scaffolds / bundles, not instance draft-plane mint).
- Elevated apply after scaffold: **`zqk object create <kind> --internal --file …`** (Enterprise license or `zqk-admin`). Legacy `zqk new internal` removed (REDACTED).

## Commands

| Command | Output |
|--------|--------|
| `zqk new object <kind> --title "…"` | Mint onto draft plane (`{id}.yaml`); stays there until promote (opt-in `--promote`). |
| `zqk object template <kind>` | Printable YAML scaffold (not persisted as an object). |
| `zqk new object-spec <ontology>` | Draft **kind definition** YAML for `object_specs/`. |
| `zqk new bundle [--name <name>]` | Minimal `scenario_bundle` under `.zqk/drafts/`. |
| `zqk object create … --internal` | Elevated create for built-in / internal kinds (license or zqk-admin). |

**Implementation:** RunE wiring in `cmd/zqk/new/new.go`; builders in `pkg/cli/bldr_cli_cmd_v1/new_*_command_builder.go`.

## Non-goals

- Interactive TUI or embedded editor (future).
- Dual instance draft areas (`new object` no longer writes instance YAML under `.zqk/drafts/`).

## Tests

- `go test ./cmd/zqk/new/...` — mint requires title; unknown kind; internal/object-spec/bundle scaffolds + last-draft for bundles.
- Draft-plane Create/promote: `pkg/storage` `TestObjectDraftPlane_*`.

## New kind specification (registration membrane)

Scaffolding an `object_spec` (or `zqk new object-spec`) is **not** enough for instance create.
Crossing into the **instance-capable** ontology is a **Kind Registration Membrane** (same *idea* as
draft-plane leave-preliminary, different plane):

1. Apply **atomically** (not hand-edited rituals): `kind_mappings` bidirectional, `id_prefixes`, namespace membership, builders/index revision.
2. Ensure `GetDirectoryFromKind` is non-empty (`docs/process/<dir>` exists **or** kind is `on_demand`).
3. **Fail closed:** do not advertise the kind as creatable (`list-kinds` / discovery) until create cannot return `unknown object kind`.
4. Prefer `net_new_kind_spec_pipeline` / `SPEC_ORIGIN_*` including **`SPEC_ORIGIN_CROSS_MEMBRANE`** over one-off YAML edits.

Kernel vocabulary: `GLS-1786412212882748000-cace1a24` (Kind Registration Membrane),
`GLS-1786411698289189000-0f37e5cd` (Kind Mapping Membrane).

## Formalization snag catalog (absorb into the process)

While formalizing system objects we keep hitting the same classes of incoherence. They are
catalogued as **`GLS-1786413953213934000-1c0fb3ff`** (*System Object Formalization Snag Catalog*)
and **must** be folded into the registration / origination process (`REDACTED`,
`REDACTED`, `REDACTED`) — not left as chat residue.

| ID | Snag | Process implication |
|----|------|---------------------|
| S01 | Spec/`list-kinds` without storage membrane → `unknown object kind` | CrossMembrane mandatory before advertise |
| S02 | Advertise before creatable | Creatable catalog ≠ raw spec index |
| S03 | Manual `id_prefixes` edits | Forbidden ritual; CrossMembrane owns apply |
| S04 | Naive `kind+"s"` directory | Irregular plural / explicit map required |
| S05 | Draft-plane ghosts / CAS rename lag | Dual-read honesty; ghost refs must not brick updates |
| S06 | Stuck draft-field updates | Draft update path must accept required fields |
| S07 | Workstream `planned` preliminary + auto-only →`active` | Research lanes need operable activate or CAS-safe planned |
| S08 | No agent `*→archived` path | Dedicated archive CLI; promote must not dead-end hygiene |
| S09 | Placeholder workstream spam | Hygiene + archive; few named lanes |
| S10 | `kinds_count` = cache occupancy | Label catalog provenance in check progress |
| S11 | Hollow Validated 0/0 | Discovery/validate membrane honesty |
| S12 | Demote ↔ `object_drafts` entanglement | Lifecycle plane contracts |
| S13 | PRI shockwave miss | Lock invariants |
| S14 | Discovery-lane allowlist ≠ storage membrane | Keep lanes aligned or fail closed |
| S15 | CLI `--field` list coercion bugs | Typed create/update |
| S16 | Objectify claims to ghost IDs | Claim verify + CAS plane proof |
| S17 | Dump many BLIs on one PRI | Workstream research lane + sprint-sized PRIs |
| S18 | No per-kind promote/demote/archive TDD | **Required outcome:** lifecycle edge matrix under test so S07/S08/S12/S13-class disparities are uncommon (`GLS-1786414163511129000-9a389a59`, `REDACTED`) |

**Required outcome — lifecycle TDD:** If promote/demote (and archive / leave-preliminary) were tested per object kind against its lifecycle YAML, many of the disparities above would fail closed in CI instead of in ops. Net-new / Kind Registration Membrane should require a matrix row (or generated tests) before a kind is treated as formalized/creatable.

When a **new** snag appears in ops, extend the catalog GLS (`machine_hints`) and link a TDE/CRIT — then close via the membrane/origination BLIs above.

## References

- [CAS_CREATE_MEMBRANE_FLOW.md](./CAS_CREATE_MEMBRANE_FLOW.md)
- [OBJECT_TEMPLATE_SYSTEM.md](./OBJECT_TEMPLATE_SYSTEM.md)
- [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) — `net_new_kind_spec_pipeline`
- Kernel: `GLS-1786413953213934000-1c0fb3ff`, `GLS-1786412212882748000-cace1a24`
