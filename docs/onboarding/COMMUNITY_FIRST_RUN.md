# Community first-run (agent + human)

**Audience:** Strangers and open-core operators after `zqk system init` who have (or will use) a public IDE agent.  
**Headless / appliance:** [`EDGE_HEADLESS_FIRST_RUN.md`](./EDGE_HEADLESS_FIRST_RUN.md) (`--headless`).  
**Not this doc:** Studio-dense process ontology (PRI/BLI/CAP dogfood) — see [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md) (**studio pack**).  
**Product strategy:** [`../strategy/open-core/AGENT_ONBOARDING_SEQUENCE.md`](../strategy/open-core/AGENT_ONBOARDING_SEQUENCE.md) · [`../strategy/open-core/SKU_ONBOARDING_SURFACES.md`](../strategy/open-core/SKU_ONBOARDING_SURFACES.md).

## Installation (Homebrew Primary)

```bash
brew tap lanceman/zqk
brew install zqk
```

Or verify release tarballs with checksum integrity:
```bash
curl -fsSL https://github.com/lanceman/zqk/releases/latest/download/checksums.txt | sha256sum -c
```

## Goal

Finish first contact without cascading failures: missing seating, vendor markdown that drifts from the kernel, or guessing which IDE you are in.

## Fail-closed sequence

Run from the project root:

```bash
zqk system agent-onboard --format json
```

| Stage | What it does | If it fails |
|-------|----------------|-------------|
| **detect** | Find Cursor / Claude Code / Cline / Windsurf / Gemini / Antigravity / OpenClaw markers | Continue (Vector B = no public agent) |
| **auth** | Soft warn if no session profile | `zqk auth login` then re-run before seating |
| **seat** | Idempotent `PER-DEFAULT-*` seating (same as init) | `zqk system seed-default-agent-seating` |
| **prime_workspace** | Write regenerable kernel boot directives into **missing** detected vendor files (+ `.agents/AGENTS.md`). Existing files are **skipped** (use `--force` to overwrite). | Fix permissions; re-run |
| **prime_kernel** | Write `.zqk/config/agent_workspace_sync.json` (workspace→kernel lite registration) | Fix `.zqk/config` writes |
| **smoke** | Confirm directives present (written or intentionally skipped) and sync report write succeeded | Re-run without `--skip-prime` |

Useful flags:

```bash
zqk system agent-onboard --detect-only --format json   # scan only
zqk system agent-onboard --dry-run --format json       # plan, no writes
zqk system agent-onboard --all-vendors                 # target every known vendor path
zqk system agent-onboard --force                      # overwrite existing vendor directive files
```

## Greenfield & Polyglot Project Initialization

`zqk system init` initializes an embedded knowledge kernel in both empty directories and existing polyglot codebases:
```bash
zqk system init --project-name my-project
```
- **Language-Agnostic:** Works seamlessly in Python (`pyproject.toml`), TypeScript (`package.json`), Rust (`Cargo.toml`), Go, or standalone documentation workspaces.
- **Agent Context:** Pass `--context ai-agent` for non-interactive scripting.

## Fast Code Navigation (Native Code Search)

The kernel includes native in-process AST and trigram code search (`zqk grep` / `zgrep`):
```bash
zqk grep "MyStruct" pkg/
zqk grep --ast "func Test*" .
```

## After green

1. `zqk quickstart` or [`QUICKSTART.md`](./QUICKSTART.md) — 5-minute interactive onboarding walkthrough.  
2. `zqk mcp install` — automatically configure detected IDEs (Cursor, Claude Desktop, VS Code) to pair with the ZQK Knowledge Kernel.  
3. `zqk workflow whats-next --format json` — next work from the kernel (not chat memory).  
4. `zqk system start-here` — short Community tutorial.  
5. Studio / process dogfood only if that is your job: [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md).  

## Kernel vs pack

- **Kernel:** `agent-onboard`, seating seed, regenerable boot payload, sync report.  
- **Pack:** Dense Cursor rules forests, MMORCH, priority-plan culture — do not treat as default stranger seed ([`KERNEL_VS_PACK_INVENTORY.md`](../strategy/open-core/KERNEL_VS_PACK_INVENTORY.md)).

## New agent identity

Creating a named persona still uses [`NEW_AGENT_PROTOCOL.md`](./NEW_AGENT_PROTOCOL.md) (`zqk agent new`). Run **`agent-onboard` first** so the workspace is seated and directives are kernel-sourced.
