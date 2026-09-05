# Log Naming Conventions and Patterns

**Last Verified:** 2026-08-31


**Purpose:** Keep log file names consistent so sibling logs (same process pattern) use the same naming scheme. When adding a new log destination, follow these patterns so users know where to look and what each file contains.

**Canonical path constants:** `pkg/paths/constants.go` (e.g. `LogEventsPrefix`, `LogsDir`, `SchedulerJobLogsSubdir`, `MCPTraceLogPrefix`, `CallbackDir`, `ComponentsLogDir`).

---

## Conventions

### 1. Structured event logs (JSON/JSONL)

Use a **scope** and an extension that matches the payload:
- **`.json`** for single JSON documents
- **`.jsonl`** for append-only JSON Lines streams (preferred for event streams)

| Pattern | Meaning | Example |
|--------|---------|--------|
| `log-events-<profile>.json` | CLI logger output by profile (JSON) | `log-events-human.json`, `log-events-system.json` |
| `<scope>-events.json` | Single daemon/component event stream | `validation-events.json` |
| `<scope>-events.jsonl` | Single daemon/component JSONL event stream | `diagnostics.jsonl` |
| `<scope>/events.jsonl` | Append-only coordination stream | `.zqk/scheduler/events/coordination-bus.jsonl` |
| `<id>.events.json` | Per-entity event stream (ID in directory name) | `.zqk/logs/scheduler/<job-id>/<job-id>.events.json` |

Rule: **Same process pattern → same naming pattern.** For line-delimited event streams, prefer `.jsonl`; for text logs, use `.log`. Avoid naming JSONL streams with `.log`.

### 2. Human-readable logs (plain text)

Use the same **scope** as the JSON variant with extension **`.log`**.

| Pattern | Meaning | Example |
|--------|---------|--------|
| `log-events-<profile>.log` | CLI logger output by profile (text) | `log-events-human.log` |
| `<scope>-events.log` | Optional text counterpart to a JSON event stream | (use when both JSON and text are written) |

### 3. Stream capture (stdout/stderr)

Use **`<scope>.stdout`** and **`<scope>.stderr`** when capturing process streams. The scope is usually the same as the directory or entity (e.g. job ID).

| Pattern | Meaning | Example |
|--------|---------|--------|
| `<id>.stdout` / `<id>.stderr` | Per-entity stream capture | `.zqk/logs/scheduler/<job-id>/<job-id>.stdout` |

Do not introduce new ad-hoc names (e.g. `out.log`, `err.log`) for the same concept; use `.stdout` / `.stderr` so they are clearly siblings.

### 4. Combined raw stream (daemon)

When a process redirects both stdout and stderr into one file, use a fixed name that reflects “stdio” rather than “events” (events go to a separate structured file).

| Pattern | Meaning | Example |
|--------|---------|--------|
| `daemon.stdio` | Combined stdout+stderr for the daemon process | `.zqk/scheduler/daemon.stdio` |

No extension: raw stream, not JSON. Keeps it distinct from `diagnostics.jsonl`.

### 5. Trace / callback / one-off logs

| Pattern | Meaning | Example |
|--------|---------|--------|
| `<prefix><scope>.log` | Trace or callback log; scope can be client ID or “system” | `mcp-trace.json` (MCP profile), `mcp-trace-<client>.log` (per client), `system.log` (callback default) |
| Callback default | Single default file when no `--log-file` is set | `.zqk/callback-logs/system.log` |

For **test runs** (scheduler run_wrapper with a log file path): keep a consistent pattern under `.zqk/logs/tests/`, e.g. `bundle-<bundle-id>-<timestamp>.log` or similar, so test output logs are clearly siblings.

---

## Where logs live (by scope)

| Scope | Base path | Files |
|-------|-----------|--------|
| **CLI (profile)** | `.zqk/logs/` | `log-events-<profile>.json`, `log-events-<profile>.log` |
| **Scheduler daemon** | `.zqk/scheduler/` | `diagnostics.jsonl`, `events/coordination-bus.jsonl`, `daemon.stdio` |
| **Scheduler job** | `.zqk/logs/scheduler/<job-id>/` | `<job-id>.events.json`, `<job-id>.stdout`, `<job-id>.stderr` |
| **Components** | `.zqk/logs/components/` | `<component>-events.json` (e.g. `validation-events.json`) |
| **MCP** | `.zqk/mcp/logs/` | `mcp-trace.json`, `mcp-trace-<client>.log` |
| **Callback** | `.zqk/callback-logs/` | Default: `system.log`; custom via `--log-file` |
| **Tests (runner)** | `.zqk/logs/tests/` | User/scheduler-supplied name, e.g. `bundle-*.log` |
| **Reports** | `.zqk/logs/reports/` | Report outputs (e.g. `object-count-report-<timestamp>.txt`); not “logs” in the event sense but colocated under logs |

Paths use `paths.ProjectDataDir` (`.zqk`) and the constants above; see `pkg/paths/constants.go`.

---

## Which file do I get? (profile → formatter → filename)

The **context profile** decides the log **formatter**, which decides whether a `.json` or `.log` file is written (see `cmd/zqk/root.go` and `pkg/logging/router_init.go`):

| Profile      | Formatter   | File written                    |
|-------------|-------------|----------------------------------|
| **human**   | Text        | `log-events-human.log` only     |
| **debug**   | Compact     | `log-events-debug.log` only     |
| **mcp**     | JSON        | `mcp-trace.json` (under `.zqk/mcp/logs/`) |
| **system**  | JSON        | `log-events-system.json`        |
| **ai-agent**| JSON        | `log-events-ai-agent.json`      |

So if you run the CLI with the default (human) profile, you only get **`log-events-human.log`** — no `log-events-human.json`. The old “event.json” style exists only when the profile uses the JSON formatter (system, ai-agent, or mcp). To get structured JSON logs for interactive use, run with `--context system` or `--context ai-agent`.

---

## Visibility into CLI commands invoked

Logger output (what ran, errors, etc.) goes to the profile log above. For a **summary of which CLI commands were invoked** (counts, timing, failures), we capture **command metrics** in `.zqk/metrics/command_metrics.json`. To surface that in a human-readable way (similar to `healthchk run`):

- **`zqk system metrics`** — list per-command metrics (invocation count, success/failure/timeout, durations).
- **`zqk system metrics --summary`** — detailed analysis report (markdown or JSON/YAML).
- **`zqk system audit-report` (PRUNED)** — analysis of command metrics (failure rates, slow commands, churn); optional `--output <path>`.

Use these when you need visibility into recent CLI activity without tailing the event log.

---

## Do not

- Mix **events** and **stream** in the same filename without a clear suffix (e.g. avoid `job.log` for “event log” when we already use `<job-id>.events.json` and `<job-id>.stdout`).
- Introduce a new pattern for “the same kind of thing” (e.g. a second style of per-job log with a different extension).
- Use ad-hoc names (e.g. `out.log`, `error.log`) for stdout/stderr capture when the project standard is `.stdout` / `.stderr`.

---

## References

- **Paths:** `pkg/paths/constants.go`
- **Layout:** `docs/architecture/BASELINE_DATA_INVENTORY.md` (§ Canonical layout, §4 Logs)
- **Scheduler logs:** `docs/process/system-health/SCHEDULER_AND_CHILD_LOG_LOCATIONS.md`, `docs/process/system-health/SCHEDULER_EVENTS_AND_METRICS.md`
