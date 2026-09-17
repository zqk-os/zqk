# Community first-run (agent + human)

**Audience:** Strangers pressure-testing ZQK Community. The CLI name is **`zcom`** until public launch (then it becomes `zqk`). Studio `zqk` is a different binary.  
**Kernel directory:** `.zqk/` — not `.zcom/`. A leftover `.zcom/credentials` file is not read.  
**Headless / appliance:** [`EDGE_HEADLESS_FIRST_RUN.md`](./EDGE_HEADLESS_FIRST_RUN.md) (`--headless`).  
**Not this doc:** Studio-dense process ontology (PRI/BLI/CAP dogfood) — see [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md) (**studio pack**).  
**MCP pairing detail:** [`QUICKSTART.md`](./QUICKSTART.md).

## Pressure-test install (this tree)

There is **no Homebrew formula and no public GitHub release** for this SKU yet. From a clone of this repo:

```bash
make          # builds ./bin/zcom from ./cmd/zqk-community
./bin/zcom --version
```

Use `./bin/zcom` from the project directory. **Do not** `export ZCOM_PROJECT_ROOT` (or `ZQK_PROJECT_ROOT`) in your shell profile — that silently attaches every later command to that checkout instead of the folder you `cd` into.

## Goal

Finish first contact without cascading failures: missing seating, docs that name commands the binary does not have, or guessing which IDE you are in.

## Fail-closed sequence

Run from the project root after `make`:

```bash
./bin/zcom system init --project-name my-project   # skip if .zqk/ already exists
./bin/zcom system agent-onboard --format json
./bin/zcom quickstart
./bin/zcom mcp install
```

| Stage | What it does | If it fails |
|-------|----------------|-------------|
| **detect** | Find Cursor / Claude Code / Cline / Windsurf / Gemini / Antigravity / OpenClaw markers | Continue (Vector B = no public agent) |
| **auth** | Community seats a local system account; leftover `~/.zqk/credentials` must not block an uninitialized directory | `./bin/zcom system init` first. There is no `auth login` command on this SKU |
| **seat** | Idempotent `PER-DEFAULT-*` seating (same as init) | `./bin/zcom system seed-default-agent-seating` |
| **prime_workspace** | Write regenerable kernel boot directives into **missing** detected vendor files (+ `.agents/AGENTS.md`). Existing files are **skipped** (use `--force` to overwrite). | Fix permissions; re-run |
| **prime_kernel** | Write `.zqk/config/agent_workspace_sync.json` (workspace→kernel lite registration) | Fix `.zqk/config` writes |
| **smoke** | Confirm directives present (written or intentionally skipped) and sync report write succeeded | Re-run without `--skip-prime` |

Useful flags:

```bash
./bin/zcom system agent-onboard --detect-only --format json   # scan only
./bin/zcom system agent-onboard --dry-run --format json       # plan, no writes
./bin/zcom system agent-onboard --all-vendors                 # target every known vendor path
./bin/zcom system agent-onboard --force                      # overwrite existing vendor directive files
```

## Greenfield & Polyglot Project Initialization

`zcom system init` initializes an embedded knowledge kernel in both empty directories and existing polyglot codebases:

```bash
mkdir my-project && cd my-project
/path/to/zqk-public-candidate/bin/zcom system init --project-name my-project
```

- **Language-Agnostic:** Works in Python (`pyproject.toml`), TypeScript (`package.json`), Rust (`Cargo.toml`), Go, or standalone documentation workspaces.
- **Agent Context:** Pass `--context ai-agent` for non-interactive scripting.
- **No scheduler on this SKU:** do not pass `--with-onboarding-roadmap` / `--with-maintenance-jobs` expecting a daemon that `zcom` does not ship. Init itself seeds the starter graph.

## Fast Code Navigation (Native Code Search)

The kernel includes native in-process AST and trigram code search (`zcom grep`):

```bash
./bin/zcom grep "MyStruct" pkg/
./bin/zcom grep --ast "func Test*" .
```

## After green

1. `./bin/zcom quickstart` or [`QUICKSTART.md`](./QUICKSTART.md) — 5-minute walkthrough (same text as `./bin/zcom system start-here`).
2. `./bin/zcom mcp install` — write detected IDE MCP config. Then `./bin/zcom mcp ensure --tcp 127.0.0.1:8443` if you want the TCP daemon.
3. `./bin/zcom object list` — first-run scoreboard (kinds you actually have).
4. `./bin/zcom workflow whats-next --format json` — next work from the kernel (not chat memory).
5. Studio / process dogfood only if that is your job: [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md).

## Kernel vs pack

- **Kernel:** `agent-onboard`, seating seed, regenerable boot payload, sync report.
- **Pack:** Dense Cursor rules forests, MMORCH, priority-plan culture — do not treat as default stranger seed.

## New agent identity

This SKU has no `agent new` command. Seat with `agent-onboard`, then follow [`NEW_AGENT_PROTOCOL.md`](./NEW_AGENT_PROTOCOL.md) using `./bin/zcom new object persona` when you need a named persona.
