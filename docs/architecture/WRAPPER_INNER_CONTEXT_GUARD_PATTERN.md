# Wrapper inner-context guard (pattern)

**Last Verified:** 2026-08-31


**Status:** Architecture (normative for CLI wrapper types)  
**Audience:** Anyone adding methods on types that embed another pointer (e.g. `*cli.Context` embedding `*context.Context`).

## Name

**Wrapper inner-context guard** — the combined practice of:

1. **Centralized nil guard** — A single helper (here: `withInnerContext` in `internal/cli/context.go`) that tests the wrapper and its embedded pointer once, then runs a short closure on the inner value.
2. **Mechanical enforcement** — `scripts/check-cli-inner-guard.sh` asserts the raw guard string `(c == nil || c.Context == nil)` appears **exactly once** in that file (inside the helper), so it cannot drift back into every method.
3. **Content-hash skip** — On success, SHA256 of the file is stored under `.zqk/pre-commit/cli-inner-guard.cache` so unchanged content skips re-running the grep in pre-commit.

**Shorthand:** *inner-context guard* (same meaning in backlog and reviews).

## Readability: declarative guards, not operator soup

Call sites that stack **`==`**, **`!=`**, **`&&`**, **`||`** (and the same idea in other languages) make the reader **decode symbols before intent**. That friction adds up in reviews and refactors. Prefer **named predicates** or **small functions** whose names read as English (`SpecResolvedFieldsMissing`, `decodeScheduledJobPayload`, `roleGuidanceDepsMissing`) so the **main path states what is true**, not how bits combine.

**Where operators may still live:** inside a **single** helper or behind a **mechanical check** (grep-count script)—one compression point per invariant, not repeated boolean noise across methods.

**Broader exploration:** backlog **`[REDACTED-ID]`** — shared declarative / combinator shapes for nil and presence (API TBD; must stay idiomatic Go and cheap where it matters).

## When to use

Apply this pattern when:

- A struct **embeds** a pointer to shared state (`*T`), and
- Many methods would otherwise repeat `x == nil || x.Inner == nil`, and
- You want **zero-value** behavior when the wrapper or inner is missing (typical for optional CLI context).

Do **not** copy the guard string into each method; add or extend the centralized helper and call it from methods.

## Anchors in this repo

| Piece | Location |
|-------|----------|
| Helper | `withInnerContext` in `internal/cli/context.go` |
| Pre-commit check | `scripts/check-cli-inner-guard.sh` (wired from `scripts/pre-commit-lint.sh`) |
| Script docs | `scripts/README.md` → `check-cli-inner-guard.sh` |
| Glossary | `glossary_term` **Wrapper inner-context guard** — `GLS-1775965558831066000-25c5083a` |

## Related sites (other embeddings)

These packages pair a node/wrapper with an embedded context pointer. They are **not** covered by `check-cli-inner-guard.sh` (different identifiers). Local helpers centralize the guard:

- `internal/cli/context/processor.go` — **`contextNodeMissing`** for `*ContextNode` + embedded `*Context`
- `pkg/context/chain.go` / `chain_helpers.go` — **`contextChainMissing`** for `*ContextChain` + embedded `ChainableContext`

If more call sites appear, keep the raw guard string only inside these helpers (or add a sibling grep script).

**Further examples (same idea, different types):**

- **`pkg/objects/spec_index.go`** — **`specIndexKindsMissing`** for `*SpecIndex` + `Kinds` (also used from `field_keys_generate.go`).
- **`pkg/scheduler/scheduled_job_guard.go`** — **`scheduledJobEnvMissing`** for `*ScheduledJob` + `EnvironmentVariables` (handlers that read job env).
- **`pkg/mcp/role_aware_prompts.go`** — **`roleGuidanceDepsMissing`** on **`RoleAwarePromptGenerator`** (guidance generator + security context).
- **`pkg/mcp/role_prompt_renderer.go`** — **`rolePromptRendererDepsMissing`** on **`RolePromptRenderer`** (guidance generator + security context).
- **`pkg/pipeline/metrics_config.go`** — **`metricsConfigSinkMissing`** / **`metricsConfigStrategyMissing`** for **`MetricsConfig`**.
- **`pkg/objects/spec_loader.go`** — **`SpecResolvedFieldsMissing`** for **`Spec`** + **`ResolvedFields`** (used from **`display_length`**, validation, storage stream-delta).
- **`pkg/quality/test_bundle_matrix_pipeline.go`** — **`testBundleMatrixPayloadFrom`** for pipeline stage payload assertions.
- **`pkg/pipeline/payload_decode.go`** — **`DecodeNonNilPayload`** delegates to **`pkg/nildecode`** (same function; **pkg/objects** and other low-level packages import **nildecode** directly to avoid an import cycle with **pkg/pipeline**).
- **`pkg/nildecode`** — canonical implementation of **`DecodeNonNilPayload`** for any package that must not depend on **pkg/pipeline**.
- **`pkg/scheduler/pipeline_scheduled_job_payload.go`** — **`decodeScheduledJobPayload`** for `*JobPayload` + `*ScheduledJob` pipeline stages; **`schedulerEventsAggPayloadForCommit`** (in **`events_aggregation_pipeline.go`**) for COMMIT when **`Summary`** must be set.
- **`pkg/storage/list_cache.go`** — **`listCacheMiss`** for map lookup + entry + **`Result`** presence.
- **`pkg/storage/object_storage_file_list_query.go`** — local **`cacheCountDecisionWithCache`** for COMMIT when **`cache`** must be set (uses **`pipeline.DecodeNonNilPayload`**).
- **`pkg/storage/content_addressable_storage_types.go`** — **`contentAddressableStorageOrIndexMissing`** for CAS get + `index` (used from **`object_storage_file_discovery.go`**).
- **`cmd/zqk/system/aggregate_audit_pipeline.go`** — **`aggregateAuditPayloadFrom`** with optional **`requireAggCtx`** for aggregate-audit stages.
- **`pkg/clihooks/profile.go`** — `map[string]*Hook`: **`Has`** uses **`_, ok := p.hooks[id]`** (membership only); **`Get`** / **`IsEnabled`** / mutators use **`pipeline.DecodeNonNilPayload[*Hook](p.hooks[id])`** for “present and non-nil”; **`List`** skips nil entries when iterating. Same three shapes repeat across the repo—see **DRY_ANALYSIS.md §6** and backlog **`[REDACTED-ID]`** (template **`scripts/templates/backlog_map_pointer_lookup_dry.yaml`**).

**Optional static analysis (tracked work):** backlog **`[REDACTED-ID]`** — `go/analysis` (or golangci plugin) to *detect* duplicate wrapper nil-guards; template **`scripts/templates/backlog_go_analysis_nil_guard_linter.yaml`**.

**Declarative / combinator nil checks (readability):** backlog **`[REDACTED-ID]`** — explore generic predicates (e.g. any/all nil or non-nil) and fluent-style flows so intent reads clearly without a forest of `== nil`; API is intentionally unspecified in the item—spike and design note; template **`scripts/templates/backlog_declarative_nil_check_api.yaml`**.

**Map `*T` lookup DRY (membership vs non-nil get):** backlog **`[REDACTED-ID]`** — optional helpers for **`map[K]*T`** so **`Has`**-style vs **`Get`**-style checks are not re-implemented at every call site; template **`scripts/templates/backlog_map_pointer_lookup_dry.yaml`**; complements §6 in **`docs/archive/continuous_improvement/DRY_ANALYSIS.md`**.

## Generalizing (“epidemic”)

The same **shape** applies elsewhere:

- **Code:** Introduce `withInner[W, T](w *W, fn func(*Inner) T, zero T)` (or equivalent) so one site owns the guard.
- **CI guard:** A small shell script counts occurrences of the **canonical** guard string for that type/file pair.
- **Cache:** Reuse the SHA256-on-success pattern under `.zqk/pre-commit/<name>.cache` to keep pre-commit fast.

Copy the conventions from `check-cli-inner-guard.sh` (env toggles for staged-only and no-cache) when adding a sibling check.

## Mechanical migration and “auto-processor” expectations

**Recommendation:** Do **not** invest in a dedicated AST migrator that invents helpers and rewrites arbitrary `a == nil || a.B == nil` across the repo. Receiver names differ, some guards return errors vs zero values, and helper placement is a package-level decision.

**What works well instead (same spirit as literal-replacement tooling):**

1. **Human** adds a single helper (or `withInnerContext`-style closure) and keeps **one** raw guard string inside it.
2. **Mechanical replace:** `scripts/go-safe-replace.sh` with `--from` set to the **exact duplicated guard** (as it appears in source) and `--to` set to the helper call (e.g. `scheduledJobEnvMissing(job)`). Narrow with `--path` to the package. Dry-run first; then `--apply`, `gofmt`, and `go test` / `go build` for touched packages.
3. **Verification:** A **grep-count** script (like `check-cli-inner-guard.sh`) or review ensures the canonical string does not creep back.

**When a new unified “auto-processor” might pay off:** If we add **many** single-file guard checks, a **manifest-driven** runner (one loop over `file + expected count + fixed string`) could reduce copy-paste between shell scripts—still **verification**, not rewrite. A **custom `go/analysis` checker** or golangci-lint plugin would be the right place for **detecting** duplicate guard *patterns* across identifiers; that is a larger investment than `go-safe-replace` + targeted checks.

## Related

- [PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md) — `*cli.Context.PathResolver()` uses the guard.
- [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) — glossary cross-links for CLI vocabulary.
- [DRY_ANALYSIS.md](../process/continuous_improvement/DRY_ANALYSIS.md) — duplication inventory, including **§6 Map-of-pointer lookups**.
