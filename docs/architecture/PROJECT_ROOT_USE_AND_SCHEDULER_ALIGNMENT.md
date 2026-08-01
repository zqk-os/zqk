# Project root: one per instance, persistent "use", and scheduler alignment

This doc extends [PROJECT_ROOT_ORIENTATION.md](./PROJECT_ROOT_ORIENTATION.md) with: (1) **one project root per binary instance**, (2) the **`zqk use [path]` command** and **persisted current root** (workspace-scoped), and (3) **scheduler alignment** when the root changes—stop the old instance, start a new one for the new root—plus **isolation** between scheduler instances and **no escape** from the defined use context (nested roots cannot reference data outside their root).

## One project root per binary instance

**Invariant:** For a single CLI process (one `zqk` invocation), every command and operation should use the **same** project root. That root is determined once at init (see resolution order in PROJECT_ROOT_ORIENTATION.md) and stored in the CLI context. No command in that process should resolve a different root unless we explicitly design a "switch" (e.g. `use`).

**Current behavior:** Root is resolved in the root command’s `PersistentPreRunE` via `ResolveProjectRoot(".")` (or env), and the result is stored in context. All subcommands that need a project root use that context (or, for CWD-oriented commands, `ResolveProjectRoot(".")` which should match when the user runs from the intended directory). So we already have "one root per process" for a typical single-command run.

**Goal:** Keep this invariant explicit and, if we add a `use` command, have it set the root for the current process (or persist it for the next process) so that all subsequent commands in scope use that root.

---

## Persistent "use" command (implemented)

**`zqk use [path]`** sets the project root for the **workspace** (the topmost directory containing `.zqk`; see workspace detection below). The choice is persisted so the next invocation uses it.

- **Workspace detection:** The workspace root is the topmost directory (walking up from CWD) that contains **`.zqk`**. We use `.zqk` only (not `go.mod`) so that non-Go projects (Dart, Java, Python, etc.) have a consistent, language-agnostic marker.
- **Storage:** Workspace's `.zqk/current_root` — one line, absolute path to the project root. The file lives in the **workspace** root, not in a nested scenario's `.zqk`.
- **Resolution order:** Env → **persisted** (read `FindWorkspaceRoot(".")/.zqk/current_root`, validated under workspace and valid project root) → CWD discovery.
- **Validation:** `path` must be a valid project root (contain `.zqk`) and **under the workspace** (no escape). Implemented: `IsValidProjectRoot`, `WritePersistedCurrentRoot`, `ReadPersistedCurrentRoot`; `paths.UnderProjectRoot` enforces no escape.
- **Scheduler:** On switch, the daemon for the **previous** persisted root is stopped (SIGTERM) before writing the new root; the scheduler for the **new** root is started automatically so the user does not need to run a separate command.
- **Listeners:** When the persisted current root is written, `OnCurrentRootSet` is invoked (see `internal/cli/context`). The root command wires this to emit a **project_root_changed** operational event via the global coordinator. Any `OperationalEventSubscriber` that subscribes to operational events can react (e.g. filter on `event.OperationType == coordination.OperationTypeProjectRootChanged`). Other threads or systems (MCP server, IDE integration, caches) can subscribe to be notified when the project root has been changed. **The scheduler daemon does not subscribe** to this event: it runs in a separate process, and the **use** command already stops the old daemon and starts the new one; no in-daemon listener is required.

**`--no-swap` (nested context, no swap):** Use when the intent is to run in a different root **without** changing the persisted root or touching the scheduler (e.g. tests, one-off commands, **test-scenarios**). `zqk use <path> --no-swap` validates the path only; `zqk use <path> --no-swap -- <command>` runs `<command>` with `ZQK_TEST_ROOT=<path>` so the child uses that root and never reads or writes `.zqk/current_root`. The primary scheduler and persisted root stay unchanged. **For nested roots under test-scenarios/, always use `--no-swap`** (e.g. `zqk use test-scenarios/onboarding-evaluation --no-swap -- zqk scheduler start`) so the workspace's `.zqk/current_root` is never overwritten and the main CLI/scheduler continue to use the workspace data. This makes the distinction explicit: **swap** (change active project) vs **no-swap** (nested/isolated run).

---

## Nested roots and no escape

**Goal:** Nested project roots (e.g. `test-scenarios/onboarding-evaluation` under the core repo) must never allow the orienting process or scheduler to reference data outside the **defined use context** (the chosen project root). This prevents data/process collision and confusion.

- **Persisted path must be under workspace:** `readPersistedCurrentRoot` and `WritePersistedCurrentRoot` require the project root path to be under the workspace (topmost `.zqk`). Paths that would escape (e.g. `../other-repo`) are rejected.
- **Strict path helpers:** Use `paths.ProjectPath(projectRoot, elem...)` to build paths under a project root; it returns `ErrPathEscapesProject` if the result would escape. Use `paths.UnderProjectRoot(projectRoot, path)` to validate that a path (e.g. job `WorkingDirectory`) is under the project root.
- **Scheduler and jobs:** Job definitions and `WorkingDirectory` are under the project root. When adding or validating jobs, ensure `WorkingDirectory` is under the scheduler’s project root so jobs never run in a different root’s tree unless explicitly intended.
- **Restructuring over time:** New code that builds project-scoped paths should use `paths.ProjectPath` or at least validate with `paths.UnderProjectRoot` so the guarantee holds as the codebase grows.

---

## Scheduler alignment with project root

**Invariant:** A scheduler daemon is bound to a **single** project root. Its PID file, keep-alive file, job definitions, and working directory for jobs live under that root’s `.zqk` and `docs/process`. There is no shared in-memory state between scheduler instances for different roots; each instance is a separate process.

**When the project root "switches" (e.g. via `use`):**

1. **Stop the scheduler for the previous root** (if one was running):
   - Send SIGTERM (graceful stop) so the daemon can finish the current job and shut down cleanly.
   - Optionally wait up to N seconds for the process to exit; then SIGKILL if necessary.
   - Do not start the new scheduler until the old process has exited (to avoid two daemons for the same root, or confusion about which process owns the PID file).
2. **Start a new scheduler for the new root:**
   - New process, new PID file under the new root’s `.zqk/scheduler/`.
   - Jobs for this instance load from the new root’s `docs/architecture/scheduler_jobs` and run with the new root’s `WorkingDirectory`.

**Where to trigger stop/start:**

- **If `use` is in-process only (Option A):** When the user runs `zqk use <new-root>` in a session, we don’t have a long-lived daemon in that process; the daemon is a separate process. So "use" would need to:
  - Call the same "stop scheduler for previous root" and "start scheduler for new root" logic that the CLI uses (e.g. stop via PID file path for previous root, start with `exec` for new root).
- **If `use` persists (Option B):** The next time the user runs any command (or explicitly `zqk scheduler start`), we resolve the root from persisted value; that root might differ from the one the currently running scheduler (if any) was started with. So we need a rule: **before starting a scheduler for root R, if a scheduler is already running for a different root R', stop it first.** That keeps one active scheduler aligned to the "current" root.

**Concrete behavior to implement (when we add `use` or persisted root):**

- On `zqk scheduler start` (or equivalent):
  - Resolve current project root (env, persisted, or CWD).
  - If a scheduler is already running for a **different** project root (detect by reading PID file under other roots we know about, or by a small registry of "last used roots"), signal it to stop and wait for exit.
  - Then start the scheduler for the **current** root.
- On `zqk use <path>` (implemented):
  - Validate `path` as project root.
  - If persisting: write to chosen store (e.g. `.zqk/current_root` for the repo we’re in, or a path derived from it).
  - If a scheduler is running for the **previous** current root, signal stop and wait (and tell the user "Stopped scheduler for previous project root.").
  - Start the scheduler for the **new** root automatically (so the user does not need to run `zqk scheduler start`).

---

## Isolation between scheduler instances

**Goal:** Logic that jobs run (e.g. run_wrapper, pre-commit, go test) must not interfere with other scheduler instances (other project roots).

**How we get isolation today:**

- **Separate processes:** Each scheduler daemon is one process. Different roots ⇒ different processes ⇒ no shared in-memory state.
- **Separate project data:** Each root has its own `.zqk` (PID file, issues, logs, caches, config) and its own `docs/process` (scheduler_jobs, backlog, etc.). Jobs run with `WorkingDirectory` set to that root, so they only see that root’s filesystem and state.
- **No global job queue:** Job definitions are loaded from the project’s `docs/architecture/scheduler_jobs`. There is no single global queue shared across roots.
- **Run-wrapper jobs:** Execute with `cmd.Dir = job.WorkingDirectory` (and resolve project root from that when needed). So a job for root A never touches root B’s `.zqk` unless it explicitly runs a command that targets another path.

**What to avoid:**

- **Shared PID file or lock:** Never use a single global path for the scheduler PID; always use a path under the project root (e.g. `.zqk/scheduler/scheduler.pid`). Same for any "single global scheduler" lock.
- **Cross-root job storage:** Do not store or load jobs from a path that could be shared across roots. Keep job storage under each root’s `docs/process`.
- **Cross-root triggers:** If we add a trigger mechanism that can target "any" scheduler, it must be keyed by project root so we only signal the daemon for that root.

**When adding `use` or multi-root support:** Ensure that any "current root" registry or list of known roots is used only for CLI-side decisions (e.g. "which scheduler to stop"), not for sharing state between running daemons.

---

## Summary

| Topic | Intended behavior |
|-------|-------------------|
| **One root per instance** | Each CLI process has one project root (resolved at init). All commands in that process use it (with CWD-oriented overrides where documented). |
| **`use` command** | Persists project root in workspace's `.zqk/current_root`; validates path under workspace; stops scheduler for previous root on switch. With `--no-swap`, does not persist or touch scheduler; optional trailing `-- <command>` runs that command with `ZQK_TEST_ROOT` set. |
| **Nested roots / no escape** | Persisted path under workspace only; `paths.ProjectPath` and `UnderProjectRoot` enforce no escape from the defined use context. |
| **Scheduler alignment** | One daemon per project root. On root switch: stop old daemon (SIGTERM, wait), then start new daemon for new root. |
| **Isolation** | Separate process and separate `.zqk` + `docs/process` per root; jobs run with that root’s WorkingDirectory; no shared in-memory or global job state. |

This design can be implemented in stages: first document and enforce "one root per process" and scheduler isolation as above; then add persisted "current root" and `use`; then add automatic scheduler stop/start on switch.
