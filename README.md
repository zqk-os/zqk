# ZQK Community (pressure-test SKU: `zcom`)

**Your AI agents are coding blind. ZQK gives them project awareness, memory, and guardrails.**

This tree is the **community pressure-test** product. The binary is **`zcom`** so it cannot be confused with studio `zqk`. Public launch will rename it to `zqk`. Kernel data stays under **`.zqk/`**.

ZQK is an operating system for AI + human hybrid engineering teams. It standardizes project goals, architecture, documentation, and task orchestration so multiple agents can collaborate safely and autonomously without losing context or drifting from requirements.

## ⚡ Quickstart (5 minutes)

There is **no brew formula and no public GitHub release** yet. Build from this checkout.

👉 **[Community First-Run Guide (Human + Agent)](./docs/onboarding/COMMUNITY_FIRST_RUN.md)**

**1. Build `zcom`**
```sh
make          # → ./bin/zcom
./bin/zcom --version
```

**2. Initialize your project (Polyglot: Python, TS, Rust, Go, Docs)**
```sh
# In this checkout (already initialized): skip init.
# Greenfield:
mkdir my-project && cd my-project
/path/to/this-repo/bin/zcom system init --project-name my-project
./path/to/this-repo/bin/zcom quickstart
```

Do **not** `export ZCOM_PROJECT_ROOT` in your shell profile. It silently attaches later commands to that checkout instead of the directory you are in.

**3. Seat your AI agent**
```sh
./bin/zcom system agent-onboard --format json
./bin/zcom system start-here
```

**4. Connect via Model Context Protocol (MCP)**
```sh
./bin/zcom mcp install
./bin/zcom mcp ensure --tcp 127.0.0.1:8443
# Cursor stdio: ./bin/zcom mcp cursor-adapter
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

**Onboarding as curriculum (system objects):** Templates for a full onboarding track live under **[scripts/onboarding_roadmap/README.md](./scripts/onboarding_roadmap/README.md)**. This SKU does **not** ship `make alpha-help`, `zqk-ts`, or a scheduler daemon — do not treat those as first-run steps.

### Getting started (clean machine golden path)

From an empty project directory, using the `zcom` you built in this repo:

```bash
/path/to/zqk-public-candidate/bin/zcom system init --project-name my-project
/path/to/zqk-public-candidate/bin/zcom object list
./path/to/zqk-public-candidate/bin/zcom workflow whats-next --format json
```

Expected outcomes:

- `system init` creates `.zqk/` and `.zqk/process/` scaffolding.
- `object list` succeeds (kinds with rows after a seeded init).
- There is no `zcom scheduler`. Do not follow studio docs that say `zqk scheduler start`.

If you run init a second time in the same directory:

- default `zcom system init` returns a clear "already initialized" error with next steps.
- use `zcom system init --legacy --discover` to inspect/populate an existing project without destructive overwrite.
- use `zcom system init --force` only when you explicitly want overwrite behavior.

### Security basics (alpha)

- **Keystore / trust material on disk:** credential-backed entries live under **`.zqk/process/keystore/`** (create the directory with restrictive permissions, e.g. `0700`, before placing sensitive files). Use **`zqk keystore`** subcommands for workflows that touch `keystore_entry` objects; do not hand-edit CAS YAML for those instances.
- **CLI output and logging:** user-facing output and logs follow structured logging (POL-CODE-007)—avoid echoing secrets, tokens, or raw credential fields in command output or copy-paste examples.

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
