# Community First-Run Guide (Agent + Human)

**Audience:** Developers and operators getting started with ZQK.  
**CLI:** Examples use the canonical executable `zqk`.  
**Kernel Directory:** `.zqk/` stores local kernel state and runtime artifacts; project configuration resides in `config/`.

## Install (this tree)

There is **no Homebrew formula and no public GitHub release** yet.

```bash
make                 # → ./bin/zqk  (or ./bin/<brand.executable_name>)
./bin/zqk --version
# equivalent: ./scripts/install.sh   (builds the branded binary locally; does not clone GitHub)
```

Use `./bin/zqk` from the project directory. **Do not** `export ZQK_PROJECT_ROOT` in your shell profile.

## Fail-closed sequence

```bash
./bin/zqk system init --project-name my-project   # skip if .zqk/ already exists
./bin/zqk system agent-onboard --format json
./bin/zqk quickstart
./bin/zqk mcp install
./bin/zqk object list
./bin/zqk workflow whats-next --format json
```

| Stage | What it does | If it fails |
|-------|----------------|-------------|
| **detect** | Find Cursor / Claude Code / Cline / Windsurf / Gemini markers | Continue without an IDE agent |
| **auth** | Local system account. Leftover `~/.zqk/credentials` must not block an empty directory | `./bin/zqk system init` first. There is no `auth login` command |
| **seat** | Idempotent `PER-DEFAULT-*` seating (same as init) | `./bin/zqk system seed-default-agent-seating` |
| **prime_workspace** | Write regenerable vendor directives into **missing** files only (`--force` to overwrite) | Fix permissions; re-run |
| **prime_kernel** | Write `.zqk/agent-runtime/agent_workspace_sync.json` | Fix `.zqk/agent-runtime` writes |
| **smoke** | Confirm directives + sync report | Re-run without `--skip-prime` |

```bash
./bin/zqk system agent-onboard --detect-only --format json
./bin/zqk system agent-onboard --dry-run --format json
```

## Greenfield

```bash
mkdir my-project && cd my-project
/path/to/this-repo/bin/zqk system init --project-name my-project
```

Init **seeds the starter Gantt in-process** (org → mission → vision → goal → workstream → plan) and writes slim retention/audit jobs. Optional flags `--with-onboarding-roadmap` / `--with-maintenance-jobs` are aliases for those outcomes — they do not fail.

Background loops (job ticks, retention, one-shots):

```bash
./bin/zqk scheduler start
./bin/zqk scheduler status
```

First-run CRUD, `object list`, and `whats-next` work without the daemon. Start it when you want the organism to keep running after you close the shell.

Init's maintenance jobs are **kernel survival** (retention, object validation, caches). They are not a prompt to configure linting. Lint, policy, and integrity timers are an optional source-code pack — see [Scheduler and maintenance](../howto/SCHEDULER_AND_MAINTENANCE.md).

## Code search

```bash
./bin/zqk grep "MyStruct" pkg/
./bin/zqk grep --ast "func Test*" .
```

## After green

1. `./bin/zqk quickstart` — same text as `./bin/zqk system start-here`. See [`QUICKSTART.md`](./QUICKSTART.md).
2. `./bin/zqk mcp install` then optionally `./bin/zqk mcp ensure --tcp 127.0.0.1:8443`.
3. `./bin/zqk object list` — first-run scoreboard.
4. `./bin/zqk system dashboard` — kernel pulse (plan + counts).
5. `./bin/zqk test dashboard` — test_case ↔ criteria lineage.
6. `./bin/zqk workflow whats-next --format json`.

ZQK Core provides the complete system kernel: object lifecycle, spec origination, command codegen, scheduler daemons (`zqk scheduler start|stop|status`), and Model Context Protocol (MCP) integration are fully native and offline-capable out of the box.
