# Community First-Run Guide (Agent + Human)

**Audience:** Developers and operators getting started with ZQK.  
**CLI:** Examples use the canonical executable `zqk`.  
**Kernel Directory:** `.zqk/` stores local kernel state and runtime artifacts; project configuration resides in `config/`.

## Install

### Homebrew (macOS & Linux)
```bash
brew tap zqk-os/tap
brew install zqk
```

### GitHub Release Binaries
Pre-compiled release archives and OpenVEX attestations are available at:
https://github.com/zqk-os/zqk/releases/latest

### Build from Source
```bash
make                 # → ./bin/zqk  (or ./bin/<brand.executable_name>)
./bin/zqk --version
# equivalent: ./scripts/install.sh   (builds the branded binary locally; does not clone GitHub)
```

Use `zqk` (or `./bin/zqk` from this tree). **Do not** `export ZQK_PROJECT_ROOT` in your shell profile.

### 1-Click Cloud Sandbox (GitHub Codespaces)
[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/zqk-os/zqk)

Launch a zero-touch development environment in under 60 seconds with zero local dependencies:
1. Click the **Open in GitHub Codespaces** badge above.
2. The container automatically compiles `./bin/zqk`, configures MCP daemon bindings, and seeds the agent host seat.
3. Start exploring right away:
   ```bash
   ./bin/zqk ui -w     # Launch Web Studio
   ./bin/zqk do        # Run autonomous swarm loop
   ```

## Quickstart (2–3 Commands)

**New Project (Greenfield):**
```bash
zqk system init --with-onboarding-roadmap # zqk init is a top-level alias for zqk system init
zqk ui -w
zqk do
```

**Existing Project:**
```bash
zqk ui -w
zqk do
```

**Air-Gapped / Sovereign Offline Execution (Ollama):**
Run completely offline with zero WAN connectivity on Apple Silicon (M1–M4) or local GPU:
```bash
# Ensure local Ollama is running (127.0.0.1:11434) with e.g. qwen2.5-coder:32b
zqk system init --with-onboarding-roadmap
zqk system agent-onboard      # Detects Ollama, primes offline directives & seats personas
zqk do                        # Executes autonomous loop locally with deterministic AST gating
```
See [Sovereign Air-Gapped AI Engineering Specification](../architecture/SOVEREIGN_AIRGAPPED_AI_ENGINEERING.md).

## Fail-closed sequence

```bash
zqk init                                          # Greenfield setup (skip if .zqk/ exists)
zqk system agent-onboard                          # Detect IDE/Ollama, prime rules, & seat agent
zqk quickstart                                    # Walkthrough (alias: zqk system start-here)
zqk workflow whats-next                           # Discover active plan and shovel-ready tasks
zqk do                                            # Autonomously claim and execute work
zqk ui -w                                         # Launch visual Web Studio (timeline & DAG)
zqk state stream --dashboard                      # Real-time ANSI visual seismograph
```

| Stage | What it does | If it fails |
|-------|----------------|-------------|
| **detect** | Find Cursor / Claude Code / Cline / Windsurf / Gemini / Ollama markers | Continue without an IDE/local agent |
| **auth** | Local system account. Leftover `~/.zqk/credentials` must not block an empty directory | `./bin/zqk system init` first, or run `./bin/zqk auth login` to create or reuse a session |
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
zqk init
```

Init **seeds the starter Gantt in-process** (org → mission → vision → goal → workstream → plan) and writes slim retention/audit jobs. Optional flags `--with-onboarding-roadmap` / `--with-maintenance-jobs` are aliases for those outcomes — they do not fail.

Background loops (job ticks, retention, one-shots):

```bash
zqk scheduler start
zqk scheduler status
```

First-run CRUD, `object list`, and `whats-next` work without the daemon. Start it when you want the organism to keep running after you close the shell.

Init's maintenance jobs are **kernel survival** (retention, object validation, caches). They are not a prompt to configure linting. Lint, policy, and integrity timers are an optional source-code pack — see [Scheduler and maintenance](../howto/SCHEDULER_AND_MAINTENANCE.md).

## Intent Discovery & The Holonic Consciousness Architecture

ZQK models multi-agent engineering not as transient LLM chat prompts, but as a persistent, self-evolving **holonic consciousness**:
- **The Intent Seed:** Every initiative begins with human operator intent objectified as persistent Vision (`VIS-*`) and Mission (`MIS-*`) objects.
- **The Nervous System & Spinal Cord:** The 5-layer cascade (`Vision/Mission` ➔ `Goals` ➔ `Priority Plans` ➔ `Requirements` ➔ `Backlog Items`) anchors high-level strategic compass points down to AST-verified code mutations.
- **The Middle-Brain Holons (Amygdala):** Ambient routines, filesystem watchers, repository branch hygiene monitors, and automated information processors operate autonomously to maintain their specific subsystem ecosystems without human micro-management.

When greenfield onboarding (`PRI-STARTER-COMMUNITY-001`) begins, the pairing agent walks the operator through intent capture before locking execution:
```bash
# Objectify the high-level intent seed
zqk new vision --title "<Your High-Level Vision>"
zqk new mission --title "<Your Active Mission>"
zqk new goal --title "<Timeless Strategic Compass>"
zqk new plan --title "<1-Cycle Bounded Batch>"

# Generate complete Three-Fold Proof traceability with one command
zqk workflow gen-trace-pipeline <REQ-ID>

# Immediately launch concurrent, token-budgeted execution
zqk do
```

## Mint Intent & Features (`zqk new <kind>`)

ZQK organizes work into an authoritative 5-layer cascade: **Vision/Mission ➔ Goals ➔ Priority Plans ➔ Requirements ➔ Backlog Items**.
Mint new objects onto the safe Draft Plane using `zqk new <kind>` (never edit `.zqk/process/` YAML directly):

```bash
# 1. High-level strategic compass (timeless intent)
zqk new goal --title "Support Dark Mode Theme"

# 2. Time-bounded 1-cycle execution batch (aimed toward a milestone)
zqk new plan --title "Dark Mode Sprint 1"             # Aliases: priority_plan, pri

# 3. Whole feature functional requirement (declares criteria)
zqk new req --title "Render dark CSS color variables" # Aliases: requirement

# 4. Atomic work unit (satisfies 1–3 criteria)
zqk new bli --title "Implement CSS token mapping"     # Aliases: backlog_item

# 5. Non-functional technical debt or test fixes
zqk new td --title "Fix theme switch test flake"      # Aliases: technical_debt
```

Link relationships across the knowledge graph with `zqk object ref add`:
```bash
zqk object ref add BLI-001 REQ-001                    # Link atomic task to parent requirement
zqk object ref add REQ-001 CRIT-001 CRIT-002          # Link acceptance criteria to requirement
```

Promote verified items from draft to authoritative CAS with `zqk object promote <id>`, or run `zqk do` to claim and execute them autonomously.
See [First-Run Object Tutorial](./FIRST_RUN_OBJECT_TUTORIAL.md) for full relational and lifecycle details.

## In-Process Code Search (`zqk grep`)

Use `zqk grep` (alias `zgrep`) instead of external `grep` or `find`. It runs sub-15ms in-process searches using persistent trigram indexing and Go AST parsing, preventing token blowup in AI agent context windows:

```bash
zqk grep "MyStruct" pkg/                       # Trigram indexed search
zqk grep --ast --kind struct .                 # AST structural query (find all structs)
zqk grep --ast --recv Engine .                 # AST method query (find methods on receiver)
zqk grep "error" --max-tokens 2000 -f json     # Token-budgeted JSON output for AI agents
```


## After green

1. `zqk quickstart` — same text as `zqk system start-here`. See [`QUICKSTART.md`](./QUICKSTART.md).
2. `zqk new <kind>` — mint goals, priority plans, requirements, or atomic backlog items (`zqk new goal`, `zqk new plan`, `zqk new req`, `zqk new bli`).
3. `zqk ci init` — scaffold zero-config GitHub Actions CI/CD and Homebrew formula.
4. `zqk swarm dispatch` — one-click autonomous worker swarm dispatch with token budgeting and AST verification.
5. `zqk mcp install` then optionally `zqk mcp ensure --tcp 127.0.0.1:8443`.
6. `zqk run <url|file>` — run a portable agent swarm package.
7. `zqk state stream --dashboard` — stream live change journal mutations and CPCP status with the visual seismograph.
8. `zqk state tree` — inspect the live knowledge graph hierarchy.
9. `zqk object list` — first-run scoreboard.
10. `zqk object inspect` — interactive object inspector, lineage radar, and live Policy Studio.
11. `zqk system dashboard` (or `zqk ui`) — Mission Control Console with dedicated Audit Tab (`--tab audit`).
12. `zqk test dashboard --check-dod` — test_case ↔ criteria lineage and 100% Definition of Done verification.
13. `zqk workflow whats-next --format json`.

ZQK Core provides the complete system kernel: object lifecycle, spec origination, command codegen, scheduler daemons (`zqk scheduler start|stop|status`), and Model Context Protocol (MCP) integration are fully native and offline-capable out of the box.
