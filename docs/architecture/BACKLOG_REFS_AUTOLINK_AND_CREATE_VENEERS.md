# Backlog document references, auto-linking, and CLI creation veneers

**Last Verified:** 2026-08-31


**Status:** Roadmap / design memory (not a shipped feature checklist)  
**Created:** 2026-04-13  
**`doc_entry`:** `DOC-1776208254618247000-0db09abe` — `zqk object get DOC-1776208254618247000-0db09abe`

## Why this exists

`backlog_item.document_refs` is allowed to hold **repo paths** and/or **doc identifiers** (see field contract in `../_internal/object_specs/backlog_item.yaml`). In practice, paths such as `docs/architecture/CLI_ALPHA_LAUNCH_PLAN.md` are easy to write but do not automatically become **stable, first-class links** unless something resolves them (doc index entry, document object, code reference object, etc.). We want automation to **close that gap when resolution is unambiguous**, and we want **higher-level CLI “veneers”** later so backlog + convergence_session + bundles are not an afterthought.

## YAML shape (human + tooling)

Keep `document_refs` as a **list** at the correct indentation. A common hand-edit mistake is to break the list so that a following top-level key (for example `id:`) looks like another list item. If you change instance data, do it through the **zqk CLI**, not by editing hash-named files under `.zqk/process/` (see [CLI_VS_DIRECT_YAML_ELICITATION.md](./CLI_VS_DIRECT_YAML_ELICITATION.md)).

## What already exists

| Area | Pointer |
|------|---------|
| Spec-based auto-fix roadmap (including reference fixes) | [SPEC_BASED_AUTO_FIXER_PLAN.md](./SPEC_BASED_AUTO_FIXER_PLAN.md) |
| CLI vs direct YAML for process objects | [CLI_VS_DIRECT_YAML_ELICITATION.md](./CLI_VS_DIRECT_YAML_ELICITATION.md) |
| Backlog field contract | `../_internal/object_specs/backlog_item.yaml` (`document_refs`) |

**Related backlog (broader doc/knowledge graph):** `[REDACTED-ID]` — phased automation beyond hand-maintained `document_refs` and markdown edges (complements this note; see [EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md](../../architecture/EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md)). **Deferred** while the data-cell program is the alpha-track focus. The backlog item’s **`related_object_refs`** include **`GLS-1776207925199440000-aa236125`** (*documentation graph*), **`DOC-1776207794402668000-767dcf59`** (assessment index), **`DOC-1776207931662056000-409a1147`** (CLI alpha plan), and this file’s **`DOC-1776208254618247000-0db09abe`** — `zqk object get [REDACTED-ID]` resolves them.

**Operational note:** CAS ID→hash mapping can drift after churn; repair uses `zqk system sync-cas-index --file (PRUNED) <path-to-hash.yaml>` when an on-disk blob is authoritative. That is orthogonal to document ref resolution but shows up in the same “things should line up” family.

## Intended auto-link / auto-fix behavior (future)

When tooling can **resolve** a location to a canonical identity:

1. Prefer the same identity the rest of the system uses (document object ID, stable doc id, validated path, or code-reference object where that is the right anchor).
2. Prefer **non-destructive** suggestions and dry-run first, consistent with the safety model in [SPEC_BASED_AUTO_FIXER_PLAN.md](./SPEC_BASED_AUTO_FIXER_PLAN.md).
3. Apply persisted fixes through **`zqk object update`** / check-driven autofix—not by editing CAS YAML by hand.

## Longer horizon: veneered create flows and persistence bundles

These are **directional** only; they are not committed implementation steps in this note.

- **Backlog + CVS in one story:** On `object create backlog_item`, accept a desired `convergence_session_profile` (or CVS template); after validation succeeds, optionally **create** the `convergence_session` and set `convergence_session_ref` in ordered, idempotent steps (or one transaction when the storage layer supports it).
- **Persistence bundle / template:** Emit a bundle or template that lists required fields, computes dates where deterministic, and lets operators override—so dependency creation is the **initial** desired state, not a follow-up patch.
- **Adjacent product exploration:** Composable CLI entry points (for example tray / named shortcuts over `zqk`) overlap this “veneer” idea; see backlog items that mention CLI veneer in `../backlog/README.md`.

## Example of a resolvable path

The path `docs/architecture/CLI_ALPHA_LAUNCH_PLAN.md` is a real file in this repo ([CLI_ALPHA_LAUNCH_PLAN.md](../../architecture/CLI_ALPHA_LAUNCH_PLAN.md)). Auto-linking work should decide whether persistence prefers **path**, **doc id**, or a **document object** once doc discovery and policy are aligned—then enforce that consistently in autofix and validation messaging.
