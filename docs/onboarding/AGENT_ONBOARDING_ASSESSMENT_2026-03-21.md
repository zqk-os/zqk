# Agent Onboarding Assessment — 2026-03-21

**Generated:** 2026-03-21 (session)  
**Category:** onboarding · assessment · workflow · glossary  
**Purpose:** Consolidated review of onboarding docs, CLI operating model, glossary objects, recent git activity, and actionable improvements. Filed alongside prior agent summaries in this directory.

---

## 1. Executive summary

The project operates as a **single Go CLI (`zqk`)** backed by **content-addressed object storage**, **scheduler jobs**, **MCP exposure of CLI commands**, and **policy/process objects** under `.zqk/process/`. Work is **object-first**: plans, backlog, policies, and glossary terms are first-class objects; instance data must be changed via the **CLI**, not by editing YAML on disk.

**Current engineering focus** (from git history and live objects): **convergence / test-failure remediation** (`convergence_session`, glossary for remedies), **coordination pipeline hardening**, **scheduler job state layout** (nested dirs, ≤100 top-level entries rule), **validation and policy path fixes**, and ongoing **process/data** commits labeled `data updates`.

**Branch context:** `feature/pri-221-product-performance` aligns with priority plan **PRI-221** — *Product & Performance (1 week)* — status **`in_progress`**. PRI-219 (*Process & System*) is **`complete`**.

---

## 2. Onboarding documentation review

### Primary sources

| Document | Role |
|----------|------|
| [README.md](./README.md) | Entry point; points to AI Agent Onboarding and quick commands |
| [AI_AGENT_ONBOARDING.md](./AI_AGENT_ONBOARDING.md) | Canonical agent guide: strategic plan, CLI routines, process integrity, git workflow, OHTV, policies, architecture patterns |
| [AGENT_ONBOARDING_SUMMARY_2026-03-19.md](./archive/AGENT_ONBOARDING_SUMMARY_2026-03-19.md) | Snapshot of CLI groups, policies, sample git log, PRI-218 backlog notes, glossary seeds |
| [../process/ai-assistant/CLI_FIRST_COMMITMENT.md](../process/ai-assistant/CLI_FIRST_COMMITMENT.md) | CLI-first, no `cd` in automation, background long runs |

### Strengths

- Clear **non-negotiables**: CLI/MCP for object YAML, OHTV, Lessons Learned, policy discovery via `zqk object list policy`.
- **Pre-change** expectations are centralized in `docs/architecture/PRE_CHANGE_CHECKLIST.md` (with Cursor rule).
- **Maintenance bundle** (`ensure-retention-jobs`) and **test runner** patterns are documented for healthy long-running use.

### Gaps / inconsistencies to fix in docs (no process YAML edits required)

1. **“Prototype CLI” vs “production-ready”** — `AI_AGENT_ONBOARDING.md` still says “prototype CLI” in places while `README.md` says production-ready. Pick one canonical term and align to reduce confusion.
2. **Last updated** — `README.md` and `AI_AGENT_ONBOARDING.md` show *Last Updated: 2025-12-29* while the product and glossary have moved on (e.g. convergence, filesystem layout cap). Bump dates when content is refreshed.
3. **Self-referential closing** — The onboarding doc ends with “recreate this prompt” for agents; consider replacing with a pointer to **live** context commands (`priority_plan`, `backlog_item`, `glossary_term`) so the doc stays valid without manual duplication.
4. **Filesystem layout** — Recent work added `FILESYSTEM_DATA_LAYOUT.md` and checklist §16. Onboarding should **link** once so new agents know the ≤100 top-level entry rule and scheduler_job storage expectations.

---

## 3. CLI and operating model (how the project runs)

### Normative operations

- **Objects:** `zqk object create|get|list|update|delete|template|bulk …` — all instance data paths go through here.
- **System health / generation:** `zqk system check`, `zqk system …-report`, `zqk system generate-instance-builders`, etc.
- **Scheduler:** daemon, `scan-tests`, job history/activity — long work and tests are pushed here or via `scripts/test-runner.sh`.
- **Policies & docs:** `zqk object list policy`, `zqk object list doc_entry`, and `zqk object list glossary_term` for vocabulary and standards.

### Agent enforcement (Cursor + docs)

- **Logging/output:** `GetLoggerFromProfile` / `cli.WriteOutput` — not raw `fmt.Print*` (POL-CODE-007 / AGENT_GUIDELINES).
- **Tests:** `go test` with `-timeout`; large suites via test-runner or scheduler (workspace rules).
- **Process data:** CLI only — matches glossary term `process data` and policy POL-CODE-002.

This is a coherent model: **the CLI is the API** for persisted process state; **MCP** mirrors it for assistants.

---

## 4. Glossary exploration

### Mechanism

- Kind **`glossary_term`** with fields: `title`, `definition`, `category`, `context_scope`, **`agent_prompts`**, **`machine_hints`**, optional `alias_refs` (see `.zqk/specs/objects/glossary_term.yaml`).
- **Live count:** 16 terms returned by `zqk object list glossary_term` (matches stored shards under `.zqk/process/glossary_terms/`).

### Strengths

- Definitions are **operational** (e.g. “hot path”, “instance builder”, “object spec”) with **agent_prompts** and **machine_hints** pointing to rules and docs — good for automation and humans.

### Recommendations

1. **Rename or fix placeholder titles** — At least one term still has title **“New system object”** while carrying a real definition. Use `zqk object update <id> --field title=…` so list/search results read cleanly.
2. **Discoverability** — Add to `docs/onboarding/README.md` **Quick Reference**:
   - `zqk object list glossary_term --format json`
   - Optional: `--filter category=…` / `title~…` once agents know the filters.
3. **Cross-linking** — Use **`alias_refs`** (per spec) to point to **policy** or **doc_entry** IDs where a term is the short form of a longer policy (e.g. POL-CODE-002 ↔ “process data”).
4. **Convergence vocabulary** — Recent commit `c319b25d2d` added **convergence_session** and **remedy-oriented** glossary terms. Ensure onboarding or a single **architecture note** explains *when* to read `glossary_term` vs `convergence_session` vs `scheduler test-failures` (one paragraph avoids duplication).
5. **Periodic export (optional)** — If markdown-only consumers need a glossary, consider a **generated** doc from `zqk object list glossary_term` in a script (similar to other reports) — avoid hand-maintaining two sources of truth.
6. **Overlap with AGENT_ONBOARDING_SUMMARY “Glossary Seeds”** — The table in `archive/AGENT_ONBOARDING_SUMMARY_2026-03-19.md` overlaps with formal `glossary_term` objects. Either **migrate** remaining rows into objects or **link** the summary to the CLI list so one source wins.

---

## 5. Workflow recommendations

1. **Single “current plan” signal** — Multiple `priority_plan` rows can have `status=active` (e.g. PRI-218, PRI-222, PRI-223). **PRI-221** is `in_progress` and matches the branch — document that **`in_progress`** is the execution focus when present, and **`active_order`** orders within the active set.
2. **After PRI-219 completion** — Workspace rules or docs that still say “current plan PRI-219” should be updated to **PRI-221** or to the **object-driven** rule: “prefer `priority_plan` with `status=in_progress`, else lowest `active_order` among active.”
3. **“Data updates” commits** — Frequent commits that only refresh process objects via CLI are normal; onboarding should **one line** explain that this is expected and not necessarily “noise” in `git log`.
4. **Improvement / audit reports** — `zqk system` audit/report commands are the feedback loop for **slow or failing commands**; tie into onboarding “Essential routines” so new agents run them early in a session.

---

## 6. Recent git history and current focus

### Sample of substantive commits (Mar 2026, non–data-only)

| Theme | Examples |
|-------|----------|
| **Convergence & glossary** | `c319b25d2d` — `convergence_session` kind, remedy glossary, coordination fixes, spec index/bootstrap |
| **Scheduler / FS layout** | `c07ee01d6e` — nested job state under `.zqk/scheduler/state/`, `FILESYSTEM_DATA_LAYOUT.md`, checklist §16 |
| **Test failures / ops** | `44c30f2eda` — convergence measure snapshot from `health.jsonl` |
| **Coordination** | `abd34f4340`, `336cb40a68`, `8fd5e1fb7d` — coordinator pipeline, async router, emit stages |
| **Validation / paths** | `19d5ccf122`, `b1774b90d8`, `d8b55e6f72` — policy directory mapping, nil-safe registry, path/hash CAS tests |

### Interpretation

Work is concentrated on **reliability of background execution** (scheduler state, coordinator), **observability of test/convergence** (sessions, health snapshots), and **correctness** of validation and storage paths — consistent with **PRI-221** themes (product, performance, reliability) and with **object-first** process hygiene (`data updates`).

---

## 7. Checklist applied (this assessment)

- Consulted **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** (sections 1, 5, 7, 8 conceptually) — no new hot-path code; this file is documentation only.
- **No edits** to instance YAML under `.zqk/process/` — all glossary/process observations use CLI read paths or existing committed files.
- **Agent guidelines** — structured output via this markdown file; no CLI user-facing changes.

---

## 8. Suggested next actions (for humans / agents)

1. Refresh **one short paragraph** in onboarding linking **glossary_term**, **convergence_session**, and **scheduler test-failures** after the convergence work.
2. **CLI:** fix any **placeholder glossary titles** and expand **`alias_refs`** where terms duplicate policies.
3. Align **Cursor/workspace rules** that reference PRI-219 with **PRI-221** or the object-based rule.
4. Optionally add **`docs/reports/README.md`** or onboarding cross-link listing **this**, **[`AGENT_ONBOARDING_SNAPSHOT.md`](./AGENT_ONBOARDING_SNAPSHOT.md)**, **[`AGENT_ONBOARDING_SUMMARIES_DIGEST.md`](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md)**, and **[`archive/`](./archive/)** dated summaries as the agent assessment series.

---

*End of assessment. Previous related file: [AGENT_ONBOARDING_SUMMARY_2026-03-19.md](./archive/AGENT_ONBOARDING_SUMMARY_2026-03-19.md).*
