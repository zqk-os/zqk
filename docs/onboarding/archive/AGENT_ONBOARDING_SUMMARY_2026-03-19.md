# Agent Onboarding Summary — 2026-03-19

**Generated:** 2026-03-19T18:41:00Z  
**Category:** onboarding · agent-summary  
**Purpose:** Periodic snapshot of project state captured at agent session start. As these accumulate, they form the basis for a unified knowledge library and glossary that compresses broad context into efficient abbreviated meaning.

---

## Project Overview

**ZQK (Zen Quantum Kernel)** is a Go-based CLI tool acting as an operating system for AI+human hybrid teams. It standardizes goals, documentation, automation, and workflow orchestration so multiple agents can collaborate safely.

**Key facts:**
- Single binary CLI (`cmd/zqk/main.go`) using Cobra
- Graph-backed object storage with CAS, WAL, stream segments
- Scheduler daemon for background jobs
- MCP server for AI assistant integration
- Object-first philosophy: all data lives in typed, versioned objects; managed exclusively via `zqk` CLI (never direct YAML edits)

---

## Session Context

| Field | Value |
|-------|-------|
| Date | 2026-03-19 |
| Branch | `feature/pri-218` |
| Active plan | PRI-218 — Development & Quality (1 week) |
| Scheduler | Running (PID 3013) |
| System status | Initialized |
| Retention at target | ❌ No |

---

## Architecture Snapshot

### CLI Command Groups
| Group | Purpose |
|-------|---------|
| `object/` | Full CRUD lifecycle, bulk ops, neighbors, templates, lifecycle, type conversion |
| `system/` | Health, validation, sync, autofix, metrics, migration, builder generation |
| `scheduler/` | Daemon management, job submission, scan-tests, history, diagnostics |
| `domain/` | Domain discovery and registration |
| `automation/` | Docman sync, lint-bypass |
| `callback/` | Scheduler job callback processing |
| `mcp/` | MCP server, coordinator setup, storage adapter |
| `observer/` | Event observer daemon |
| `reports/` | PCS, EDD, blockers, quick, improvement-report |
| `precommit/` | Pre-commit hooks (aggregate, integrity, policy, lint) |
| `quick/` | Fast-create for backlog-item, decision, question |
| `semantic/` | Semantic maturity assessment and recommendations |
| `spec/` | Spec listing |
| `rollback/` | Rollback operations |
| `utility/` | Version, migration, scenario-builder, config-file-watcher |
| `docman/` | Documentation management |
| `keystore/` | PKI/keystore operations |
| `ontology/` | Ontology management |
| `organizational/` | Org structure and change impact |

### Key Packages
| Package | Purpose |
|---------|---------|
| `pkg/storage` | File/graph/CAS backends, WAL, stream segments, path cache, object ID cache, list cache, retention, bulk ops |
| `pkg/specbuilder` | Spec-driven code generation — typed Go builders from YAML object specs |
| `pkg/validation` | Pluggable validators (Go, SHACL, custom); instance/tier/lifecycle/integrity |
| `pkg/scheduler` | Background job scheduler with daemon, transceiver, health metrics |
| `pkg/mcp` | Full MCP server implementation |
| `pkg/cli` | CLI framework, command builders |
| `pkg/coordination` | Central event coordination |
| `pkg/logging` | Structured logging with profiles (`GetLoggerFromProfile`) |
| `pkg/paths` | Path alias/constant resolution; all `.zqk/` subdirs via constants |
| `pkg/graph` | Graph backend interfaces (memgraph, provider) |
| `pkg/metrics` | Metric object management |
| `pkg/featureflags` | Feature flag support |

---

## Mandatory Agent Policies (Condensed)

| Policy | Rule |
|--------|------|
| **CLI-first** | Never edit `.zqk/process/` YAML directly — use `zqk object create/update/delete` |
| **Logging** | Never `fmt.Print*`/`os.Stdout`/`os.Stderr`; use `logging.GetLoggerFromProfile(ctx.Profile)` |
| **Output** | Use `cli.WriteOutput(cmd, data)` for structured results |
| **Generated files** | Never edit `pkg/specbuilder/bldr_instance_v1/*_instance_builder.go` by hand |
| **Tests** | Long suites via `scripts/test-runner.sh` or `zqk scheduler scan-tests`; always `-timeout` |
| **Builders** | Use generated instance builders; never hand-build `map[string]any` for spec-backed objects |
| **Binary** | Use `./bin/zqk` (built via `make build-all`); never `go run` for operational use |
| **Pre-change** | Consult `docs/architecture/PRE_CHANGE_CHECKLIST.md` before every code change |
| **Priority order** | Complete all items on current plan (P0→P1→P2→P3) before switching plans |

---

## Recent Git Activity (last 10 commits, as of session start)

| Hash | Date | Message |
|------|------|---------|
| `b54a6f0` | 2026-03-19 | data updates |
| `44a551c` | 2026-03-19 | Fix UE-state cascade: add exit watchdog + O_NONBLOCK after command output |
| `0cd5290` | 2026-03-19 | Add backlog items for improvement report gaps and P1 consolidation tracker |
| `250746c` | 2026-03-19 | fix(scheduler): suppress false-positive trigger_failed for persistent maintenance jobs |
| `3daad86` | 2026-03-19 | feat(cache-first): pre-warm Tier 4 validation scan and deterministic freshness check |
| `cb1bfb3` | 2026-03-19 | data updates |
| `f7c2fe28` | 2026-03-19 | fix(scheduler): align timeout goroutines and suppress stale bundle noise |
| `d93aa9869` | prior | data updates |
| `ebe8c43` | prior | fix(system): enforce spec-valid system generation and trace autofix persistence |
| `042b83506` | prior | fix(system): persist cumulative autofixes and extend audit status metrics |

---

## Priority Plan PRI-218 — Remaining Work

### In Progress (P1)
| ID | Title | Notes |
|----|-------|-------|
| BLI-010 | CLI/MCP Comprehensive Test Coverage | Stale title ("New Title"); has `non_existent_field` — cleanup candidate |
| BLI-761 | Implement Cypher import | No description in object |
| BLI-762 | Implement JSON Schema import | No description in object |
| BLI-763 | Implement OpenAPI import | No description in object |

### Planned — Critical
| ID | Title |
|----|-------|
| BLI-1772465538… | Document and support manual reset (shut down, bulk-delete, data sacrifice) |

### Planned — High Priority
| ID | Title |
|----|-------|
| BLI-17739164… | **Reduce `system object-count-report` latency** (55–108s → <10s target) |
| BLI-17739164… | **Fix `system auto-fix-process-pending`** high latency (60s avg) and 33% error rate |
| BLI-17724655… | Bulk delete + batch index updates for retention |
| BLI-17724655… | Decouple ordered structure build from aggregation job |
| BLI-17724655… | Storage primitives for count and oldest N (object maintenance redesign) |

### Planned — Medium/Untiered
| ID | Title |
|----|-------|
| BLI-17724655… | Retention status in main feedback loop and bootstrap |
| BLI-17739164… | Consolidation tracker — done when all dependent BLIs resolve |
| BLI-17737684… | Align misplaced/unmanaged process files with CAS/index |

---

## Live Improvement Report (2026-03-19T18:41:20Z)

| Severity | Finding | Action |
|----------|---------|--------|
| **P1** | 11 commands with high failure rate; 7 timeout-prone | Run `zqk system audit-report --format json` (PRUNED) |
| **P2** | 5 slow commands | Run `zqk system audit-report` (PRUNED) for details |
| **P3** | 1 churn indicator | Improve docs/UX for frequent-failure patterns |

**Top failing commands:**
| Command | Failure Rate | Invocations |
|---------|-------------|-------------|
| `zqk scheduler scan-tests --load-bundles pkg-storage` | 90.9% | 11 |
| `zqk scheduler scan-tests --load-bundles cmd-zqk-system` | 85.7% | 7 |
| `zqk system check all --auto-fix --fast --format json ...` | 80.0% | 5 |
| `zqk scheduler scan-tests --load-bundles cmd-zqk-object` | 80.0% | 5 |
| `zqk system check --auto-fix` | 100.0% | 3 |
| `zqk system check all --format json --auto-fix --timeout 1200s` | 33.3% | 3 |
| `zqk-stable system auto-fix-process-pending` | 33.3% avg 32s | 3 |
| `zqk object get <arg>` | 64.3% | 14 |

---

## Recommended Next Action (at session start)

**Most efficient: `system object-count-report` latency fix**

The backlog item has a clear spec:
1. Read counts from the `count_by_kind` map in the object ID cache — no live storage scan
2. Make `--emit-events` fire-and-forget (async)
3. Optionally cache the last result with a TTL

Reduces 55–108s command to <10s; directly resolves one P1 improvement-report item.

**Close second:** Investigate `zqk system check --auto-fix` 100% failure (3 consecutive).

---

## Glossary Seeds (for future compression)

| Term | Abbreviated Meaning |
|------|---------------------|
| **PRI-N** | Priority plan — bounded 1-week focus area with ordered backlog |
| **BLI-N** | Backlog item — atomic unit of work tied to a priority plan and tier |
| **DOC-N** | Doc entry — registered documentation object with group/category/path |
| **CAS** | Content-addressed storage — immutable hash-keyed object store |
| **WAL** | Write-ahead log — durability layer before final persistence |
| **OHTV** | Observe → Hypothesize → Test → Verify — mandatory investigation framework |
| **POL-CODE-007** | Logging policy — no fmt.Print*; use GetLoggerFromProfile |
| **bldr_instance_v1** | Generated typed builders from YAML specs — never hand-edit |
| **audit_event** | High-volume event record; managed by retention + aggregation jobs |
| **path-cache** | Alias resolver for all `.zqk/` subdirs; use `paths.ResolvePathStrict` |
| **improvement-report** | Live system health snapshot: failure rates, timeouts, slow commands |
| **scan-tests** | Scheduler-based test bundle discovery and execution |
| **data updates** | Commit label for CLI-only process object mutations (not code changes) |
