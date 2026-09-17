# Git-native drift search patterns

**Last Verified:** 2026-08-31


**Purpose:** Keep a **repeatable list of `git grep` recipes** so anyone can spot **style and DRY drift** when intuition strikes—without relying on memory or one-off searches. Git already indexes the tree; these commands are fast, work offline, and respect the worktree.

**Not a substitute for:** `golangci-lint`, tests, or `zqk system analyze-drift-hotspots` (PRUNED) (ontology and risk-tiered hotspots—see `docs/architecture/CONSTANTS_AND_DRY_INVENTORY_PLAN.md`). Use this doc for **human-triage** and **convergence-style** sweeps.

**Related vocabulary:** `docs/enforcement/ANTI_PATTERNS_BY_LANGUAGE.md` (Go sections G1–G3: `maps.Copy`, `strings.Cut`, `SplitSeq`; **G6** `pkg/appledouble` for `._*` sidecars).

---

## How to run

- From the **repository root**.
- **Expect false positives.** Many hits are legitimate; the value is **comparing over time** (counts, familiar files) and **spotting new hotspots**.
- To **scope** to packages you care about, append paths: e.g. `git grep … -- pkg/ internal/ cmd/`.
- To **exclude generated** instance builders (when hand-maintaining patterns):  
  `git grep … -- '*.go' ':(exclude)pkg/specbuilder/bldr_instance_v1/*_instance_builder.go'`

---

## Pattern list (copy-paste)

### 1. Whole-map merge intent (`maps.Copy` candidates)

**Signal:** `for k, v := range` over a map with `dst[k] = v` (or similar) where **`maps.Copy(dst, src)`** would state intent clearly.

```bash
git grep -n 'for k, v := range' -- '*.go' ':(exclude)vendor'
```

**Triage:** Skim for copy-into-another-map loops. Ignore ranges that only **read** or **delete** keys, or that merge with non-trivial logic (those are not mechanical `maps.Copy`).

---

### 2. Two-part string split (`strings.Cut` candidates)

**Signal:** `strings.SplitN(..., 2)` where **`left, right, ok := strings.Cut(...)`** is clearer.

```bash
git grep -n 'SplitN' -- '*.go' ':(exclude)vendor'
```

**Triage:** Filter for delimiter splits where exactly two parts are intended (see ANTI_PATTERNS **G2**).

---

### 3. Comma-separated iteration (`strings.SplitSeq` candidates)

**Signal:** `strings.Split` + `for range` over a slice when **`strings.SplitSeq`** would avoid the intermediate slice.

```bash
git grep -n 'strings\.Split(' -- '*.go' ':(exclude)vendor'
```

**Triage:** Only paths that immediately range over the result (see ANTI_PATTERNS **G3**).

---

### 4. User-facing stdout/stderr (logging policy)

**Signal:** Direct **`fmt.Print*`** or **`os.Stdout` / `os.Stderr`** in CLI and library code where **`logging`** / **`cli.WriteOutput`** should apply (`docs/enforcement/AGENT_GUIDELINES.md`, POL-CODE-007).

```bash
git grep -nE 'fmt\.(Print|Fprint|Panic)' cmd/zqk pkg/ internal/ -- '*.go'
git grep -n 'os\.Std(out|err)' cmd/zqk pkg/ internal/ -- '*.go'
```

**Triage:** Tests and deliberate tooling may legitimately use these; production CLI paths should follow project patterns.

---

### 5. Adoption baseline (`maps.Copy` usage)

**Signal:** How widely **`maps.Copy`** is already used (useful when judging remaining drift).

```bash
git grep -n 'maps\.Copy' -- '*.go' ':(exclude)vendor'
```

---

### 6. Machine-assisted hotspot scan (not `git grep`)

For **kind names, system field keys, and schema literals** with risk tiers, prefer the maintained command:

```bash
zqk system analyze-drift-hotspots (PRUNED)
```

Documented in **`docs/architecture/CONSTANTS_AND_DRY_INVENTORY_PLAN.md`**.

---

### 7. CLI RunE: `string_array` specs vs wrong pflag getter

**Signal:** A `RunE` handler calls **`GetStringSlice`** while the command YAML declares **`type: "string_array"`** for that flag. pflag then returns an empty slice (type mismatch), often with the error ignored.

```bash
git grep -n 'GetStringSlice' cmd/zqk -- '*.go'
```

**Triage:** For each hit, open the matching **`.zqk/cli/specs/...`** file. If the flag is **`stringSlice`**, **`GetStringSlice`** is correct. If it is **`string_array`**, use **`GetStringArray`**. Canonical note: **`pkg/cli/bldr_cli_cmd_v1/README.md`** (section *RunE handlers and pflag getters*).

---

### 8. Hardcoded map keys, env names, and common literal sinks (exhaustive baseline)

**Signal:** A single broad `grep -E` over `./cmd/**/*.go` (and peers) mixes **map/JSON field names**, **`os.Getenv("ZQK_…")`**, test helpers, and unrelated matches. That is useful as a **spot check** but is **not** equivalent to “all literals eliminated” and is **not** what the codebase vetting matrix’s **`fully_refactored_dry`** column claims by itself—pair matrix gates with **targeted scans** and **`zqk system analyze-drift-hotspots` (PRUNED)** (pattern **6**).

**Repeatable script (recommended):** runs several **`git grep`** passes over **`cmd/`**, **`cmdv2/`**, **`pkg/`**, **`internal/`**, excludes generated **`pkg/specbuilder/bldr_instance_v1/*_instance_builder.go`**, optional **`--no-tests`** (drops `*_test.go` and `*_test_helper.go`).

```bash
chmod +x ./scripts/scan-hardcoded-go-literals.sh
./scripts/scan-hardcoded-go-literals.sh
./scripts/scan-hardcoded-go-literals.sh --no-tests --out .zqk/logs/drift/go-literals-scan.txt
```

**Sections inside the script (summary):** Pathspec also **excludes `pkg/errfmt/**`** (intentional std `fmt`/`errors` shims; scanning there duplicates pattern G/F noise).

| Section | What it approximates |
|--------|------------------------|
| A | `foo["field"] =` (CLI output maps, object shapes, audit metadata) |
| B | `os.Getenv` / … with a string literal (**anchored** so `fooos.Getenv` substrings are not counted) |
| C | `context.WithValue(` call sites (**anchored** so only real `context` selector) |
| D | `http.Header`-style `.Set("Name",` / `.Add` / `.Del` |
| E | Prometheus-style `WithLabelValues("` (when used) |
| F | `errors.New(` (**anchored** — excludes `xerrors.New`-style substring false positives) |
| G | **Std** `fmt.Errorf(` only (regex excludes `errfmt.Errorf`); remaining volume is real `fmt` migration debt |
| H–J | **Legacy octal `0755` / `0644` / `0600`** on `MkdirAll` / `WriteFile` / `OpenFile` / `Chmod` — prefer **`paths.DirPerm755`**, **`paths.FilePerm600`**, **`paths.FilePerm644`** (`pkg/paths/constants.go`). **`gocritic` `octalLiteral`** stays disabled in `.golangci.yml` (would force thousands of `0o` edits without naming); this section is the mechanical sweep. |

**Triage:** Many hits are **correct as literals** (stable JSON keys, env contract names). **`docs/architecture/CONSTANTS_AND_DRY_INVENTORY_PLAN.md` §3** lists what **not** to centralize (e.g. `json:"…"` tags). Prefer **`analyze-drift-hotspots`** for **ontology / system field** risk tiers.

**Per-file ranking (optional):** If you maintain a **`search-terms.txt`**-style vocabulary from extraction tools, use **`scripts/drift_quoted_literal_triage.py`** to intersect with **kind + FieldKey** literals and count **`"term"`** occurrences per file (see **`scripts/README.md`**). That trims false positives from the raw word list and surfaces **high-offender files** for convergence tranches.

**Legacy one-liner (high overlap with A + B, extra noise):** the regex is fine for a coarse signal, but **`git grep` pathspecs were wrong** in the original loop: with multiple path arguments, Git treats them as **OR**, so `"$p"` together with `'*.go'` matches **any** `*.go` in the repo — not “`*.go` under `$p`”. That made `{cmd,cmdv2,pkg,internal}_results.txt` nearly the same size and **not** per-package scoped.

**Correct per-root scope** (still only `*.go` under each top-level tree):

```bash
for p in cmd cmdv2 pkg internal; do
  git grep -nE '(\[[[:space:]]*"[a-z_A-Z]+"[[:space:]]*\][[:space:]]*=)|\([[:space:]]*"[A-Z_]*"[[:space:]]*\)' \
    -- ":(glob)${p}/**/*.go" > "${p}_results.txt" || true
done
```

**Automation:** `scripts/drift-search-baseline.sh` uses the **`:(glob)…` form by default** and can **`stats` / `compare` / `compare-all` / `compare-dirs` / `compare-to-latest` / `compare-last-two` (`trend`) / `record` / `sync-matrix` / `list` / `prune-misscoped`**. **`record`** updates **`.zqk/logs/drift/search-baseline/latest`**; **`sync-matrix`** (or **`record --sync-matrix`**) refreshes **`docs/quality/DRIFT_SEARCH_BASELINE_MATRIX.csv`** for **`zqk matrix report --name drift_search_baseline`**. Use **`capture --legacy-pathspec`** only when reproducing old, mis-scoped snapshots for comparison. See **`scripts/README.md`** and **`docs/quality/DRIFT_SEARCH_BASELINE_RUBRIC.md`**.

**Historical note (mis-scoped):**

```bash
# Do not use for new baselines — matches essentially all tracked *.go, not per-root.
for p in cmd cmdv2 pkg internal; do
  git grep -nE '(\[[[:space:]]*"[a-z_A-Z]+"[[:space:]]*\][[:space:]]*=)|\([[:space:]]*"[A-Z_]*"[[:space:]]*\)' -- "$p" -- '*.go' > "${p}_results.txt" || true
done
```

---

### 9. macOS AppleDouble sidecars (`._*`) — migrate to `pkg/appledouble`

**Signal:** Ad hoc **`strings.HasPrefix(name, "._")`** / **`strings.HasPrefix(filepath.Base(path), "._")`** when walking directories of YAML specs, process data, or bootstrap trees. These files are **not valid YAML**; they break parsers and **`zqk system generate-command-builders`** if picked up as specs.

**Prefer:** **`github.com/lanceman/zqk/pkg/appledouble`** — **`IsSidecarFileName`** for a single basename; **`PathHasSidecarSegment`** for full paths (e.g. tar/archive entries). Bootstrap extract, CLI spec codegen, and several scripts already use this package; **remaining call sites** are migration debt—run a **pre-alpha** sweep and trim stragglers.

**Find stragglers (copy-paste):**

```bash
# Ad hoc basename checks (migrate to pkg/appledouble)
git grep -n 'HasPrefix.*"\._"' -- '*.go' ':(exclude)vendor'

# Adoption check: call sites already using the package
git grep -n '"github.com/lanceman/zqk/pkg/appledouble"' -- '*.go' ':(exclude)vendor'
```

**Triage:** Legitimate **non-file** uses are rare; most hits should migrate to **`appledouble`** for one vocabulary (see **`docs/enforcement/ANTI_PATTERNS_BY_LANGUAGE.md`** **G6**). **Operational:** **`scripts/build-bootstrap-archive.sh`** uses **`COPYFILE_DISABLE=1`** and **`find … -name '._*' -delete`** before **`tar`**; **`field-key-literal-scan.mdc`** remains the canonical FieldKey gate—do not conflate.

---

### 10. Navigation vocabulary graph (`scheme_ref` / anchored `vocabulary_scheme`)

**Signal:** Edits that add or reshuffle **`glossary_term_relation`** edges or glossary terms keyed to **`vocabulary_scheme_ref`** should stay aligned with **`docs/architecture/VOCABULARY_GRAPH.md`** (canonical GAPE root **`GLS-1776420209472499000-83004aa1`**, expected GTR totals **14 / 6 / 5**).

**Spot cross-links in CAS YAML (copy-paste):**

```bash
git grep -n 'scheme_ref: VOC-' .zqk/process/glossary_term_relations -- '*.yaml'
git grep -n 'vocabulary_scheme_ref:' .zqk/process/glossary_terms -- '*.yaml'
```

**Triage:** Prefer **`zqk object list glossary_term_relation --filter scheme_ref=… --count`** or **`sh ./scripts/list-vocabulary-gtr.sh counts`** over manual counting; avoid duplicate **`glossary_term`** roots with the same human-facing title for the same navigation role.

---

## When to run

- **Before a large refactor** in a package (establish a baseline hit list).
- **After merges** that touch many files (spot accidental style regression).
- **During convergence / cleanup** sessions when **`desired_end_state`** mentions DRY, `maps.Copy`, FieldKey-style consistency, or **literal / env centralization** (pattern **8** + **`analyze-drift-hotspots`**).
- **Before alpha launch** (or any wide release): optional **pattern 9** sweep + migrate remaining **`._`** **`HasPrefix`** call sites to **`pkg/appledouble`**.
- **After changing** seeded **`vocabulary_scheme`** / cross-walk **`glossary_term_relation`** rows (pattern **10** + **`list-vocabulary-gtr.sh counts`**).
- **After changing** command specs or `RunE` handlers for flags with **`string_array`** / **`stringSlice`** (see pattern **7**).
- **Whenever it feels wrong**—that is enough.

---

## Maintaining this list

When you discover a **high-value grep** that catches real drift repeatedly, add a short subsection here (command + triage note). Prefer **stable** patterns over brittle regex. If a check becomes mandatory at CI time, move it to **linters or scripts** and leave a pointer here for history.
