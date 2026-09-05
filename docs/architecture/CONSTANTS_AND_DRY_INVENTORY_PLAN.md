# Constants and DRY Inventory Plan

**Last Verified:** 2026-08-31


**Purpose:** Replace magic strings and repeated logic with constants and shared patterns so updates are one-place and the codebase stays lean. See **PRE_CHANGE_CHECKLIST.md §12** for the checklist.

**Repetition-driven loop (R0–R3):** **`HARDCODED_LITERAL_REPETITION_PHASES.md`** — detect repeated literals, eliminate mechanically where possible, verify, move to the next bucket without ad hoc “slice” picks.

---

## 1. Existing constants (use these; don’t re‑introduce literals)

| Location | Contents |
|----------|----------|
| `pkg/paths/constants.go` | ProjectDataDir, CacheDir, file names (ValidationCacheFile, etc.), LockFileSuffix, TmpFileSuffix, extensions, **`DirPerm755` / `FilePerm600` / `FilePerm644`** for `MkdirAll` / `WriteFile` |
| `pkg/context/logging_context.go` | ProfileSystem, ProfileMCP, ProfileAIAgent, ProfileDebug, ProfileHuman |
| `pkg/validation/constants.go` | ValidationCacheVersion, NoSourceCodeAvailableChecksum, ValidationCodeChecksumFiles, LockOp* (state cache lock names) |
| `pkg/scheduler/constants.go` | DefaultMaxRuntimeSeconds, scheduler-related names |
| `pkg/metrics/constants.go` | Metrics-related keys |
| `pkg/mcp/constants.go` | MCP-specific strings |
| `pkg/kindnames/kinds.go` | Canonical object **kind** string literals (`kindnames.*`; add new kinds here first) |
| `pkg/objects/constants.go` | `objects.Kind*` aliases of `kindnames` (and schema version defaults) |
| `pkg/translation/constants.go` | Translation keys |

---

## 2. Systematic plan for .go and _test.go

### Phase A – Logger profile and lock names (high impact, low risk)

- **Logger profile:** Any `GetLoggerFromProfile("system")` or `GetLockLoggerFromProfile("system")` should use `string(pkgctx.ProfileSystem)`. **Done:** repo-wide (cmd, internal, pkg—including `pkg/validation` helpers such as `lockLoggerSystem`—and tests where those calls appeared). Prefer `pkgctx "github.com/lanceman/zqk/pkg/context"` for the import alias.
- **Lock / concurrency operation names:** If a package uses `RunInLockWithLogger` / `RunInRLockWithLogger` with string literals, move those strings to a package-level constants file (e.g. `lock_op_names.go` with `LockName*` string constants) and reference them. **Done in:** `pkg/validation` (`lock_op_names.go` for async validator, metrics, id/namespace/output queues, etc.; state cache still uses `LockOp*` in `constants.go`), `pkg/storage` (`pkg/storage/locknames` for storage + id_generation lock names, including `WithLockTimeout` ops in high-volume event cache and reverse reference index), `pkg/scheduler` (`constants.go` `LockName*` for scheduler core; `transceiver/lock_op_names.go` for transceiver), `pkg/mcp` (`lock_op_names.go`; `pkg/mcp/testing` imports `pkg/mcp` for shared `LockName*`), `pkg/coordination` (`lock_op_names.go` for coordinator mutex operations), `pkg/cli`, `pkg/config`, `pkg/featureflags`, `pkg/git`, `pkg/graph/memgraph` (including `WithLockLogger` / `WithRLockLogger` in the pool), `pkg/graph/provider`, `pkg/interactive`, `pkg/metrics`, `pkg/objects` (`lock_op_names.go`), `pkg/observer`, `pkg/rollback`, `pkg/runtime`, `cmd/zqk/system` (`lock_op_names.go` for `RunInLockWithLogger`, `WithLockTimeout` / `WithRLockTimeout`, and related helpers such as hash registry cache and fix executor), `cmd/zqk/callback` (`lock_op_names.go` for processor and queue), `cmd/zqk/utility` (`lock_op_names.go` for config file watcher), `internal/cli` (`lock_op_names.go` for format handler and MCP permission checker `WithLockTimeout` call sites). **Remaining:** `concurrency` package tests and other `_test.go` files may still use ad-hoc operation name strings for `WithLockTimeout`; optional to centralize in Phase E. **Done:** `cmd/zqk/system/spec_auto_fixer_test.go` — tests using `setupSpecAutoFixerTest` (sets `ZQK_TEST_ROOT`) no longer use `t.Parallel()` (process-global env races).

### Phase B – Versions, suffixes, and config-like literals

- **Versions:** Any `"1.0.0"` or similar schema/cache version → constant in the owning package (e.g. ValidationCacheVersion in validation). Search for version-like strings in JSON structs and config. **Done in:** `pkg/storage` (version_constants.go: CASIndexFormatVersion, BlockingCheckConfigVersion; existing CompressedSnapshotFormatVersion, CompactedChangeJournalFormatVersion, SemanticSnapshotFormatVersion, highVolumeEventCacheVersion), `pkg/objects` (InitialFieldVersion in constants.go, used in field_versioning.go), `pkg/specbuilder/profile_builders` (`constants.go`: `DefaultProfileSchemaVersion`; `NewBaseProfileBuilder` and profile codegen compare against it; `bldr_profile_v1.SchemaVersionV1` aliases it), `pkg/specbuilder/bldr_profile_v1` / codegen (as above; `go run ./cmd/zqk system generate-profile-builders --overwrite`), `pkg/specbuilder/bldr_config_v1` / `config_builders` codegen (`ConfigYAMLVersionV1`; `go run ./cmd/zqk system generate-config-builders --overwrite` — default configs dir is `docs/process/_internal/configs`; `make build-all` uses the same), `pkg/specbuilder/bldr_v2` (`policy_constants.go`: `PolicyVersionFieldDefaultSemVer` for policy field checklist default), specbuilder builders/instance/constants codegen (`builders/codegen.go`, `instance_builders/codegen.go`, `constants_factory.go`: `fmt.Fprintf` for emitted Go). **Done (prefer `fmt.Fprintf` on builders):** repo-wide sweep replacing `WriteString(fmt.Sprintf(...))` with `fmt.Fprintf` for `strings.Builder` / `bytes.Buffer` text assembly (`cmd/zqk`, `pkg`, `internal`, `scripts`); helper script `scripts/refactor_write_string_sprintf_to_fprintf.py` (when `buf` is already `*strings.Builder` / `*bytes.Buffer`, use `fmt.Fprintf(buf, ...)` not `&buf`). **Remaining:** ad-hoc test literals outside bootstrap (Phase E). **`pkg/specbuilder/bootstrap`:** `BootstrapCaptureFormatVersion` (see above).
- **File suffixes:** `.lock`, `.tmp`, `.json` etc. already in `pkg/paths` where shared; package-local suffixes (e.g. under a single package) can live in that package’s constants.
- **Kind names / excluded lists:** Validation cache excluded kinds are in `pkg/validation/state_cache.go` as a map; consider moving the list to `pkg/validation/constants.go` as a slice and building the map at init if you want a single place to add/remove kinds.

### Phase C – Error and fallback messages

- **Recurring error/fallback strings** (e.g. “no-source-code-available”, “cache file is locked”) → constants in the same package so message changes and i18n (if added later) are centralized.

### Phase D – DRY and patterns

- **Repeated if/else or switch chains:** Consider small helpers, builder-style APIs, or table-driven logic so behavior is defined in one place.
- **Error handling:** Same error wrapping or logging pattern in multiple places → shared helper (e.g. “log and return”, “wrap with context”).
- **Callbacks / emitters:** If the same “do X then notify” pattern appears in several commands or packages, consider a small shared abstraction (e.g. runner + callback, or event emitter) instead of copying the pattern.

### Phase E – Tests

- **Test-only literals:** Large inline JSON, repeated file paths, or magic timeouts in `_test.go` can be moved to test helpers or constants in the test file/package (e.g. `testDataDir`, `defaultTestTimeout`) so tests stay readable and changes are one-place.

---

## 3. What not to constant-ize

- **JSON struct tags** (e.g. `json:"object_id"`): Keep as literals in struct definitions; they are part of the type and changing them is a schema change. Document schema in one place (e.g. README or schema doc) instead.
- **One-off error messages** that appear only once and are highly specific; optional to move to a constant if it improves clarity.
- **Third-party API strings** (e.g. HTTP methods, well-known headers) unless the project wraps them in its own API.

---

## 4. Verification

- After refactors: `go build ./...` and relevant tests.
- **Broad literal / map-key baseline (cmd, cmdv2, pkg, internal):** `./scripts/scan-hardcoded-go-literals.sh` (see **`docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md`** pattern **8**); use `--no-tests` for a production-skewed list. Expect noise; compare baselines over time.
- Grep for remaining high-value literals: e.g. `"system"` in logging, `".lock"`, `"1.0.0"` in caches/config.
- **Drift hotspot scan (kind names, system field keys, schema version literals):** `zqk system analyze-drift-hotspots` (PRUNED) walks Go sources under the project root (default), loads **ontology** names from `docs/process/_internal/object_specs/*.yaml`, and reports findings with risk tiers:
  - **critical / high:** string literals equal to a spec ontology name when used in `==` / `!=` or `case` (tests downgraded one tier).
  - **medium / low:** map index keys that match **system object** field names (`id`, `kind`, `schema_version`, …; aligned with instance builder `systemFieldOrder` in `pkg/specbuilder/instance_builders/codegen.go`).
  - **high / medium:** bare `"2.0.0"` schema version literals outside `pkg/objects/constants.go` (prefer `objects.DefaultSchemaVersion`).
  Use `--min-risk` (`critical` … `low`), `--include-tests`, `--specs-dir`, and `--extra-kinds` as needed. JSON: `--format json`. Generated Go and `vendor/` are skipped.
  - **Text output defaults:** totals, by risk/category, and **Top files** (see `--top`); line-level hits require **`--details`** (large). **`--summary-only`** prints only rollups.
  - **Constant barrels:** `pkg/kindnames/kinds.go` and `pkg/objects/constants.go` are **skipped** for **ontology kind** string literals (those files define the canonical spellings). By default, `lock_op_names.go` and `pkg/specbuilder/bldr_v2/*_constants.go` are also **skipped** (literals there are intentional). Use **`--include-constant-barrels`** for the latter group. JSON reports include **`file_summaries`** (per-file counts).
- **Per-file inventory column 4 (CSV):** snapshots under `.zqk/logs/cvs/` may mark **Yes** for files that only centralize strings (lock op names, generated/bldr_v2 `*_constants.go`). To align the gate with *remaining* ad hoc work, run **`scripts/cvs-reconcile-col4-inventory.py`** (input CSV → new CSV with those paths set to **No**). Re-run when you regenerate the inventory.
- Checklist: Before new code, confirm **§12** (constants, DRY, lean code) is considered.

## 5. End of each phase

- **Stage** only the code and docs for that phase (exclude `.zqk/` runtime and process instance data).
- **Commit** with a clear message (e.g. `refactor(phase-a): ...` or `refactor(phase-b): ...`).
- **Push** to the feature branch (e.g. `feature/pri-219`).
- If pre-commit blocks (e.g. policy/lint from background), fix violations or run `zqk pre-commit aggregate` after background checks pass; use `--no-verify` only when necessary and note it in the commit message.
