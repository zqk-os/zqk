# Lint and code-quality rule inventory

**Last Verified:** 2026-08-31


**Purpose:** Map **policy, lessons, and agent guidance** to **how they are enforced** (machine vs script vs process-only). Use this when onboarding agents or exporting a “knowledge module”: the **lint profile** (`.golangci.yml`, hooks) is the enforceable contract; **process objects** (`docs/process/`) carry intent and traceability.

**Primary config:** `.golangci.yml` (v2). **Hooks / background:** `tools/git-hooks/pre-commit`, `scripts/pre-commit-lint.sh`, `zqk pre-commit aggregate` — see `docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md`.

---

## Machine-enforced (Go / static analysis)

| Source | Rule / intent | Mechanism | Notes |
|--------|----------------|-----------|--------|
| POL-CODE-007, AGENT_GUIDELINES | No `fmt.Print` / `Printf` / `Println` to implicit stdout in production code | **golangci-lint `forbidigo`** (`.golangci.yml`) | `fmt.Fprintf(w, ...)` and structured output paths are allowed. **`*_test.go` excluded** (examples may print). |
| POL-CODE-007, logging policy | Broader logging/output patterns | **`scripts/check-logging-compliance.sh`**, review | Complements forbidigo; not every pattern is AST-trivial. |
| Performance / complexity / security | Cyclomatic complexity, gosec, gocritic, unparam, unused, staticcheck | **golangci-lint** | Many **path/text exclusions** document accepted debt; burn down incrementally. |
| Format / imports | gofmt, goimports | **golangci-lint formatters** | `scripts/` skipped per config. |

## Script- and tool-enforced (repo-specific)

| Source | Intent | Mechanism |
|--------|--------|-----------|
| Constants / magic literals | Reduce hardcoded strings and permissions | `scripts/scan-hardcoded-go-literals.sh`, `pkg/paths` constants — see `GIT_DRIFT_SEARCH_PATTERNS.md` |
| Architecture / layering | Layer and dependency rules | `scripts/check-architecture-compliance.sh` |
| Drift / hotspots | High-churn patterns | `zqk system analyze-drift-hotspots` (PRUNED), `pkg/drifthotspots` |
| Spec plane | Spec-derived caches and invalidation | `PRE_CHANGE_CHECKLIST.md` §3a, `SPEC_ORIGIN_PLANE.md` — enforcement is process + review |

## Process and documentation only (not golangci)

These are **essential** but **not** replaced by linters; they belong in **process data** (`zqk object …`) and docs:

| Source | Why not auto-lint |
|--------|-------------------|
| LESSONS_LEARNED § CLI / YAML | “Never hand-edit `docs/process/` YAML” — requires **workflow** and **CLI** discipline |
| LESSONS_LEARNED § E2E verification | Behavioral; checked by **convergence / human verify** |
| Priority plans, milestones, goals | **Traceability** — `backlog_item` lifecycle and refs |
| POL objects | **Normative** text; selective automation via scripts above |

---

## Exporting a “knowledge module” for another repo

Bundle **without** copying the whole tree:

1. **`.golangci.yml`** (and document **Go + golangci-lint versions**, e.g. `go.mod` / CI `GO_VERSION`).
2. **Hook contract:** `tools/git-hooks/pre-commit` + `scripts/sync-git-hooks.sh` (or your variant).
3. **Policy index pointer:** `docs/process/enforcement/POLICY_ENFORCEMENT_INDEX.md` (or a slim export of POL IDs you care about).
4. **Agent runbook:** `AGENT_GUIDELINES.md`, `AGENT_CONTEXT_REFRESH.md` (incl. glossary / `glossary_term` pointers), `PRE_CHANGE_CHECKLIST.md`, `scripts/README.md` (local verification + context refresh script) (by reference).
5. **Optional process pack:** create objects via **`zqk object create`** from templates — never hand-author CAS YAML.

---

## Changelog

- **2026-04-04:** Document inventory; enable **`forbidigo`** for `fmt.Print*` (POL-CODE-007 surface), with `*_test.go` excluded.
