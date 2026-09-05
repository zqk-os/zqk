# Semantic density and pattern extraction

**Status:** Active  
**Primary goal:** **Semantic density** — code where the interesting lines are *what the feature does* (rules, boundaries, integration), not copy-pasted scaffolding. This doc supports the **§13 post-verify** pass in **`docs/architecture/PRE_CHANGE_CHECKLIST.md`**.

Secondary topics: when to extract helpers, why we skip “% unique code” KPIs, and where shared pieces live.

---

## Semantic density (the bar)

- **Dense:** Readers see domain logic, named invariants, and clear control flow; repeated shapes are **data** (tables, constants) or **one small helper**.
- **Thin:** The same guard, error string, or branch body appears multiple times — readers must diff mentally to see what differs.

**Post-verify (after logic is verified):** Scan for duplication and literals; tighten until the file “reads like the problem.” See checklist §13 and the pass list [below](#post-verify-pass-after-logic-is-verified).

---

## When to extract (rule of two, cap at three)

| Copies | Action |
|--------|--------|
| **Second** identical or near-identical structure | **Ideal time** to extract — if the helper name and parameters are obvious, do it now. |
| **Third** | **Extract or table-drive** before merging — three copies without a helper is debt. |

If two blocks differ in subtle ways, a **table** (slice of structs) or **shared prefix + small variation** often beats a vague mega-helper.

---

## Why not a percentage of “unique” vs “repeated” code?

Line-count ratios are a poor KPI: tests, generated code, and intentional verbosity distort the number. **Semantic density** is qualitative — use judgment, not a target percentage.

---

## Practical signals

| Signal | Action |
|--------|--------|
| Same **error shape** (“usage: …”, “unknown X %q”) | One helper + constants for stable parts (e.g. `cmd/zqk/system/cli_hooks.go`: `requireCliHooksArgs`, usage constants aligned with `.zqk/cli/specs/system/cli_hooks_command.yaml`). |
| Pattern may spread | **Comment** pointing at spec YAML or canonical doc; promote to `internal/…` or `pkg/…` when it crosses packages. |
| Unsure whether to abstract | **Clarity at call site** wins over premature abstraction — but the **second** copy is the prompt to decide. |

If removing a helper would force pasting the same block in **three** places, the helper is earning its keep.

---

## Static alias tables (domain-specific)

When user-facing strings must accept **synonyms** but code should branch on **one canonical** value:

1. **Data** — `map[string]string` from **alias (or synonym) → canonical** for that domain only (same spirit as `objects` status alias maps). Map **values** should be named constants (e.g. `FieldOpCreate`), not fresh string literals.
2. **Normalize** — `aliases.ResolveStatic(input, yourTable)` from **`pkg/aliases`** so trim/lower/lookup is consistent everywhere.
3. **Dispatch** — `switch` on the canonical constants only (handlers and metrics stay stable; e.g. `cmd/zqk/system/spec_field_op.go` **`FieldOp*`** and **`ResolveSpecFieldOperation`** so CLI and `SpecWriter` share one surface).

**Do not** reuse **`pkg/cli.AliasRegistry`** default *operation* aliases for unrelated domains: that table is tuned for **CRUD-style** verbs (e.g. `modify` → `update`), which can **collide** with another feature’s vocabulary (e.g. spec field lifecycle **modify** must stay `modify`).

**When to add `pkg/aliases`:** second or third place that needs the same “trim + lower + map + pass-through” shape.

---

## Post-verify pass (after logic is verified)

Before considering a change “done,” scan the diff (or file) for:

1. **DRYability** — duplicated guards, repeated `errfmt.Errorf("usage:…")`, parallel branches.
2. **Hardcoded literals** — strings that are really **configuration or policy** → constants or spec-linked constants.
3. **Readability** — names, order of operations, and whether a **table** beats repeated `case` bodies.
4. **Anti-patterns** — **`docs/process/enforcement/ANTI_PATTERNS_BY_LANGUAGE.md`**, **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** §13.

This pass is **not** “minimize LOC”; it is **raise semantic density** and **anchor intentional literals**.

---

## Where reusable pieces live

- **`internal/`** — CLI wiring and helpers tied to this binary.
- **`pkg/`** — Stable libraries shared across commands or modules.
- **`.zqk/cli/specs/`** — Canonical help/examples; align hand-written usage errors (comment in code → YAML path).

---

## Related

- **`pkg/aliases`** — `ResolveStatic` for shared alias-table normalization.
- **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** §13 — Constants, DRY, post-verify checklist items.
- **`docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md`** — Periodic sweeps when doing broader consistency work.
- **`docs/process/enforcement/AGENT_GUIDELINES.md`** — Output, logging, common mistakes.
