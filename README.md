# ZQK (Zen Quantum Kernel)

**Your AI agents are coding blind. ZQK gives them project awareness, memory, and guardrails.**

ZQK is an operating system for AI + human hybrid engineering teams. It standardizes project goals, architecture, documentation, and task orchestration so multiple agents can collaborate safely and autonomously without losing context or drifting from requirements.

## ⚡ Quickstart (5 minutes)

The easiest way to get started with ZQK:

👉 **[Community First-Run Guide (Human + Agent)](./docs/onboarding/COMMUNITY_FIRST_RUN.md)**

**1. Install ZQK**
```sh
brew tap lanceman/zqk
brew install zqk
# or (checksum-verified curl install)
curl -sSL https://raw.githubusercontent.com/lanceman/zqk/main/scripts/install.sh | sh
```

**2. Initialize your project (Polyglot: Python, TS, Rust, Go, Docs)**
```sh
mkdir my-project && cd my-project
zqk system init --project-name my-project
zqk quickstart
```

**3. Seat your AI agent**
```sh
zqk system agent-onboard --format json
zqk system start-here
```

**4. Connect via Model Context Protocol (MCP)**
```sh
zqk mcp ensure --tcp 127.0.0.1:8443
# Wire your IDE (Cursor, Claude Code, Windsurf, Gemini, etc.) to the MCP endpoint
```

---

## 🎁 What You Get

- **AI Agent Guardrails:** Enforceable policies that agents verify *before* modifying code.
- **Autonomous Project Context:** Agents discover goals, requirements, and architectural decisions without manual prompting.
- **Native Code Search:** Fast in-process AST and trigram search via `zqk grep` (alias `zgrep`).
- **Polyglot & Zero-Dependency:** Seamless greenfield initialization across Python, TypeScript, Rust, and Go.
- **Local-First & Offline-Ready:** Zero cloud dependency required—runs locally with Git and filesystem storage.
- **Full Traceability:** Every line of code and commit links directly back to project backlog items and requirements.

---

## ⚙️ How It Works

ZQK uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to expose your project's knowledge kernel to your AI tools. When you connect Cursor, Claude, or any MCP-compatible agent to ZQK, the agent uses structured tools to discover context, claim tasks, run tests, and verify completion criteria before shipping.

---

## 🏗️ Architecture & Development

- **Knowledge Kernel:** Distributed, spec-driven object store backed by content-addressable storage.
- **Workflow Engine:** Deterministic state machine managing tasks, plans, and peer-agent handoffs.
- **CLI & MCP Mesh:** Standardized interfaces for seamless human and agent pairing.

### 🚀 Getting Started Guides

- **[Community First-Run Guide](./docs/onboarding/COMMUNITY_FIRST_RUN.md)** — Recommended starting point for all new users and agents.
- **[First-Run Object Tutorial](./docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** — Step-by-step tutorial on creating and managing kernel objects.
- **[AI Agent Onboarding Guide](./docs/onboarding/AI_AGENT_ONBOARDING.md)** — Deep-dive guide for AI agents participating in ZQK development.
- **[Architecture Overview](./docs/architecture/README.md)** — Technical decisions, storage engine, and design specifications.

**Onboarding as curriculum (system objects):** Templates for a full onboarding track (priority plan, workstream, milestone, backlog items, optional one-shot seed job) live under **[scripts/onboarding_roadmap/README.md](./scripts/onboarding_roadmap/README.md)**. **`make alpha-help`** (repo root) lists alpha bundle/metrics targets and points to that curriculum and the certification design narrative. Isolated evaluation with **`zqk-ts`**: **[ONBOARDING_EVALUATION_SCENARIO.md](./docs/testing/ONBOARDING_EVALUATION_SCENARIO.md)**.

### Getting started (clean machine golden path)

From an empty project directory:

```bash
zqk system init --project-name my-project
zqk object list backlog_item --format table
zqk scheduler start
zqk system check   # authoritative kernel health (not --fast / not --allow-degraded)
```

Expected outcomes:

- `system init` creates `.zqk/` and `.zqk/process/` scaffolding.
- `object list backlog_item` succeeds (often zero rows right after a new init).
- Full `system check` is the health bar. `--fast` skips refs (see `docs/architecture/check-fast-mode.md`). `--allow-degraded` only means “partial/stale OK” — never treat it as proof the kernel is healthy (`docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md`).

Optional partial smoke (not health):

```bash
zqk system check --fast --allow-degraded
```

If you run init a second time in the same directory:

- default `zqk system init` returns a clear "already initialized" error with next steps.
- use `zqk system init --legacy --discover` to inspect/populate an existing project without destructive overwrite.
- use `zqk system init --force` only when you explicitly want overwrite behavior.

Troubleshooting during first hour:

- If a command says scheduler is not running, start it with `zqk scheduler start` (or recycle after promote via `./scripts/recycle-stable-daemons.sh`). Use `--allow-degraded` only when partial/stale output is intentionally acceptable.
- For package/iteration verification, prefer targeted scheduler runs like `zqk scheduler scan-tests --package ./cmd/zqk/system` over long foreground test runs.

### Security basics (alpha)

- **Keystore / trust material on disk:** credential-backed entries live under **`.zqk/process/keystore/`** (create the directory with restrictive permissions, e.g. `0700`, before placing sensitive files). Use **`zqk keystore`** subcommands for workflows that touch `keystore_entry` objects; do not hand-edit CAS YAML for those instances.
- **CLI output and logging:** user-facing output and logs follow **`docs/enforcement/AGENT_GUIDELINES.md`** (POL-CODE-007): use structured output and the logging profile—avoid echoing secrets, tokens, or raw credential fields in command output or copy-paste examples.

### 📚 Documentation

- **[Assessment & onboarding index](./docs/architecture/ASSESSMENT_AND_ONBOARDING_INDEX.md)** — central map for alpha/onboarding/architecture entry points; **`doc_entry`** anchors include **`DOC-1776207794402668000-767dcf59`** (this index), **`DOC-1776207931662056000-409a1147`** ([CLI alpha plan](./docs/architecture/CLI_ALPHA_LAUNCH_PLAN.md)), **`DOC-1776208254618247000-0db09abe`** ([backlog `document_refs` / auto-link design](./docs/architecture/BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md)); **`glossary_term`** **`GLS-1776207925199440000-aa236125`** (*documentation graph*); platform backlog **`[REDACTED-ID]`** tracks automation of that graph (optional long-horizon work)
- **[Architecture Documentation](./docs/architecture/README.md)** - Architecture decisions, patterns, and design documents
- **[Coding Best Practices](./docs/best-practices/coding/README.md)** - Coding standards and best practices
- **[Observability Best Practices](./docs/best-practices/observability/README.md)** - Logging, metrics, and monitoring
- **[Refactoring Documentation](./docs/refactoring/README.md)** - Refactoring plans and status
- **[Codebase Summary](./docs/reports/CODEBASE_SUMMARY.md)** - Comprehensive codebase statistics and analysis

### Why It Exists
- **Goal traceability** – every artifact (docs, scripts, runs) links back to project goals.
- **Path freedom** – default path aliases merge with your `zqk-settings.yaml` overrides (no full replace); relocate `.zqk` subtrees without hardcoding strings—see [PATH_ALIAS_RESOLUTION.md](./docs/architecture/PATH_ALIAS_RESOLUTION.md) and [DATA_CELL_RUNTIME_ORGANISM.md](./docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md). Use `zqk system path-cache --show-paths` to print resolved datacell paths.
- **Predictable documentation** – doc index + CLI make it effortless to discover the
  right guidance or script from the terminal.
- **Template packs** – new projects (Flutter, Node, Rust, etc.) start with sensible,
  opinionated defaults while remaining customizable.
- **Release intelligence** – history snapshots and tagging deliver rollups that explain
  why goals were (or were not) met.
- **Distributed Knowledge Kernel** – git-based distribution with PKI authority verification
  enables collaborative operating system model across multiple agents.

### Early Roadmap
1. **Graph Backend Foundation**
   - Pluggable graph backend interface
   - MemGraph implementation
   - System ontology definition
   - Knowledge kernel separation design
2. **Distributed Kernel**
   - PKI infrastructure
   - Git-based distribution
   - Authority verification
3. **CLI & MCP Integration**
   - Fresh CLI implementation
   - MCP server with graph traversal
   - Progressive disclosure patterns

### License

ZQK uses an **open-core model**:
- **Open-Core / Community**: [Apache License 2.0](LICENSE) (see also `NOTICE`)
- **Enterprise / Pro modules**: Commercial / proprietary (not part of the Apache grant; see LICENSE Appendix A and `docs/architecture/OPEN_CORE_PROPRIETARY_SPLIT.md`)



## Verification Matrix
All agent skills and user documentation updates are orchestrated utilizing our new verification matrix to ensure features traceability.
telemetry update for r24
