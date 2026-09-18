# Community first-run (agent + human)

**Audience:** People pressure-testing ZQK Community. The CLI is **`zcom`** until public launch (then it becomes `zqk`). Studio `zqk` is a different binary.  
**Kernel directory:** `.zqk/` — not `.zcom/`.  
**MCP pairing detail:** [`QUICKSTART.md`](./QUICKSTART.md).  
**Not this SKU:** Studio process dogfood — [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md) (pack).

## Install (this tree)

There is **no Homebrew formula and no public GitHub release** yet.

```bash
make                 # → ./bin/zcom
./bin/zcom --version
# equivalent: ./scripts/install.sh   (builds local zcom only; does not clone GitHub)
```

Use `./bin/zcom` from the project directory. **Do not** `export ZCOM_PROJECT_ROOT` or `ZQK_PROJECT_ROOT` in your shell profile.

## Fail-closed sequence

```bash
./bin/zcom system init --project-name my-project   # skip if .zqk/ already exists
./bin/zcom system agent-onboard --format json
./bin/zcom quickstart
./bin/zcom mcp install
./bin/zcom object list
./bin/zcom workflow whats-next --format json
```

| Stage | What it does | If it fails |
|-------|----------------|-------------|
| **detect** | Find Cursor / Claude Code / Cline / Windsurf / Gemini markers | Continue without an IDE agent |
| **auth** | Community seats a local system account. Leftover `~/.zqk/credentials` must not block an empty directory | `./bin/zcom system init` first. There is no `auth login` command |
| **seat** | Idempotent `PER-DEFAULT-*` seating (same as init) | `./bin/zcom system seed-default-agent-seating` |
| **prime_workspace** | Write regenerable vendor directives into **missing** files only (`--force` to overwrite) | Fix permissions; re-run |
| **prime_kernel** | Write `.zqk/config/agent_workspace_sync.json` | Fix `.zqk/config` writes |
| **smoke** | Confirm directives + sync report | Re-run without `--skip-prime` |

```bash
./bin/zcom system agent-onboard --detect-only --format json
./bin/zcom system agent-onboard --dry-run --format json
```

## Greenfield

```bash
mkdir my-project && cd my-project
/path/to/zqk-public-candidate/bin/zcom system init --project-name my-project
```

Init **seeds the starter Gantt in-process** (org → mission → vision → goal → workstream → plan) and writes slim retention/audit jobs. Optional flags `--with-onboarding-roadmap` / `--with-maintenance-jobs` are aliases for those outcomes — they do not fail.

Background loops (job ticks, retention, one-shots):

```bash
./bin/zcom scheduler start
./bin/zcom scheduler status
```

First-run CRUD, `object list`, and `whats-next` work without the daemon. Start it when you want the organism to keep running after you close the shell.

## Code search

```bash
./bin/zcom grep "MyStruct" pkg/
./bin/zcom grep --ast "func Test*" .
```

## After green

1. `./bin/zcom quickstart` — same text as `./bin/zcom system start-here`. See [`QUICKSTART.md`](./QUICKSTART.md).
2. `./bin/zcom mcp install` then optionally `./bin/zcom mcp ensure --tcp 127.0.0.1:8443`.
3. `./bin/zcom object list` — first-run scoreboard.
4. `./bin/zcom workflow whats-next --format json`.

There is no `zcom agent new`, `zcom auth`, or `zcom-admin`. Scheduler **is** shipped: `zcom scheduler start|stop|status`.
