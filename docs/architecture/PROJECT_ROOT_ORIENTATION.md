# Project root orientation

Commands must operate on the correct project: **tests** (isolated), **test-scenarios with live data** (e.g. `test-scenarios/onboarding-evaluation`), or the **core project root** (main repo). This doc defines how project root is resolved and how to keep commands unambiguous.

**One root per binary instance:** For a single CLI process, every command uses the same project root (resolved once at init). Optional **`use` command** and **scheduler alignment** when switching roots are described in [PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md](./PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md).

## Canonical resolution (single source of truth)

Project root is resolved in exactly this order (see `internal/cli/context/context.go` → `ResolveProjectRoot`):

1. **`ZQK_PROJECT_ROOT`** — Explicit production root (scripting, multi-repo).
2. **`ZQK_TEST_ROOT`** — Explicit test root (test isolation; when set, CWD is ignored for that process).
3. **Persisted current root** — Workspace's `.zqk/current_root` (see [PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md](./PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md)). The **workspace root** is the topmost directory containing `.zqk` (we use `.zqk` only, not `go.mod`, so non-Go projects are supported). The file must contain a path under the workspace (no escape).
4. **CWD-based discovery** — `findProjectRoot(".")` walks up from the current working directory and returns the topmost directory that contains `.zqk`.

**Config must not override.** Project root is **not** read from `.zqk/config/config.yaml` or user config. The context’s `ProjectRoot` is set at CLI init from the above resolution (with `"."` as start path). This avoids ambiguity where a config file in one project could point at another (e.g. a scenario).

## Three orientations

| Orientation | How to get it | Typical use |
|-------------|----------------|-------------|
| **Tests** | Set `ZQK_TEST_ROOT` to a temp or scenario dir; root is isolated from CWD. | `go test`, test scripts, CI. |
| **Test-scenario with live data** | `cd` into the scenario (e.g. `test-scenarios/onboarding-evaluation`); no env set. CWD-based discovery finds that directory’s `.zqk`. | Running commands against that scenario’s data. |
| **Core project root** | Run from repo root (or the dir that has the repo’s `.zqk`) with no env set; or set `ZQK_PROJECT_ROOT` to the repo. | Normal development, scheduler daemon, pre-commit. |

## CWD-oriented vs context-oriented commands

- **CWD-oriented** (prefer `ResolveProjectRoot(".")` first): Commands that read/write **local state** under the project (scheduler PID/issues/jobs, pre-commit results, go test output). They should act on the project that **contains the current working directory** so that “run from this directory” always means “this project.”  
  Example: `zqk scheduler clear-issues`, `zqk scheduler status`, `zqk scheduler start`, `zqk pre-commit status`.

- **Context-oriented** (use `ctx.ProjectRoot` with fallback to `ResolveProjectRoot(".")`): Commands that use the project root from CLI init (same resolution at startup). Fine when the process is started from the intended directory; fallback handles missing context.

**Rule:** For any command that touches `.zqk` state (scheduler, pre-commit, go test), use **CWD-first** resolution so the project root is always the one for the directory from which the user ran the command. That avoids acting on a different project when context was set from another path (e.g. a previous run or a different working directory in the same process).

## Ensuring orientation in code

- **CWD-oriented:**  
  `projectRoot := cli.ResolveProjectRoot(".")`  
  then if needed: `if projectRoot == "" && ctx != nil { projectRoot = ctx.ProjectRoot }`

- **Context-oriented (with fallback):**  
  `projectRoot := ctx.ProjectRoot`  
  `if projectRoot == "" { projectRoot = cli.ResolveProjectRoot(".") }`

- **Tests:** Set `ZQK_TEST_ROOT` (or `ZQK_PROJECT_ROOT`) so resolution does not depend on test CWD.

## References

- `internal/cli/context/context.go`: `ResolveProjectRoot`, `findProjectRoot`
- [PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md](./PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md): one root per instance, optional `use` command, scheduler stop/start on switch, isolation
- `docs/architecture/README.md`: scheduler project root
- `docs/architecture/README.md`: `ZQK_TEST_ROOT` for scenarios
