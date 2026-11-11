# CLI Kind Alias Parity Guide

## Executive Summary

The **CLI Kind Alias Parity Specification** defines the canonical kind shorthand aliases supported across the ZQK CLI, specifically unifying `zqk new <kind>`, `zqk new object <kind>`, and `zqk object new <kind>` (an alias of `zqk object create`).

Under **CRIT-001** (POL-DOC-001 Documentation Invariant for CLI Kind Aliases), this document formalizes the canonical shorthand alias mappings to ensure seamless developer and autonomous agent ergonomics across all entrypoints.

---

## Supported Shorthand Kind Aliases

The table below delineates the canonical kinds and their standard shorthand aliases across CLI entrypoints:

| Canonical Kind | Shorthand Aliases | Domain / Ontology Role | CLI Invocations |
| :--- | :--- | :--- | :--- |
| `test_case` | `tst`, `tc` | Unified Verification Group | `zqk new tst`, `zqk new tc`, `zqk new object tst`, `zqk object new tc` |
| `criteria` | `crit` | Atomic Verification Proof | `zqk new crit`, `zqk new object crit`, `zqk object new crit` |
| `requirement` | `req` | Timeless Feature Contract | `zqk new req`, `zqk new object req`, `zqk object new req` |
| `priority_plan` | `pri`, `plan` | Time-Bounded Execution Cycle | `zqk new pri`, `zqk new plan`, `zqk new object pri`, `zqk object new pri` |
| `backlog_item` | `bli` | Atomic Unit of Effort | `zqk new bli`, `zqk new object bli`, `zqk object new bli` |
| `milestone` | `mil` | Chronological Boundary Anchor | `zqk new mil`, `zqk new object mil`, `zqk object new mil` |
| `goal` | `goal` | Timeless Strategic Compass | `zqk new goal`, `zqk new object goal`, `zqk object new goal` |

---

## Architectural Mechanics & Resolution Pipeline

Kind alias resolution operates through three synchronized layers:

1. **Config & Default Synonyms Layer (`pkg/validation/id_prefixes_config.go` & `.zqk/specs/configs/id_prefixes_config.yaml`):**
   - Declares explicit `kind_to_synonyms` mappings.
   - Externalized configuration enables standardized alias resolution without hardcoding in command parsers.
   - Feeds the `SynonymProviderAdapter` into `objects.GetGlobalSynonymResolver()`.

2. **Semantic Synonyms Engine (`pkg/kindsynonyms` & `pkg/objects`):**
   - Provides deterministic mapping from shorthand aliases (e.g., `tst`, `tc`, `pri`, `req`, `crit`, `bli`, `mil`) to canonical kind identifiers (`test_case`, `priority_plan`, etc.).
   - `objects.GetCanonicalKind(raw)` resolves any alias or canonical name to its canonical identifier.

3. **CLI Command Routing Layer (`cmd/zqk/new/` & `cmd/zqk/object/`):**
   - **Direct Convenience Commands (`zqk new <alias>`):** Subcommands under `zqk new` configured with alias arrays (e.g. `test_case` with `Aliases: []string{"tst", "tc"}`; `priority_plan` with `Aliases: []string{"plan", "pri"}`).
   - **Positional Validation (`zqk new object <kind>` / `zqk object new <kind>`):** Uses `cli.ValidateAnnotatedKind` and `objects.ResolveAndValidateKindForProject` to resolve aliases prior to object construction and validation against the project spec index.

---

## Negative Adversarial Boundary

When an unrecognized kind or alias is supplied (such as `zqk new object unknown_alias` or `objects.ResolveAndValidateKindForProject(root, "invalid_kind")`), the system fails closed with an informative error message:

```text
unknown kind "invalid_kind": not in this project's object spec index (update specs or use a registered kind name; valid canonical kinds include: goal, requirement, test_case, criteria, priority_plan, backlog_item, milestone; valid aliases include: req, tst, tc, crit, pri, bli, mil, goal)
```

This error guides operators directly to valid canonical kinds and supported shorthand aliases.
