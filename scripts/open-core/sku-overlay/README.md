# ZQK Community

**Your AI agents are coding blind. ZQK gives them project awareness, memory, and guardrails.**

This tree is the **community** product. Command examples use the default executable token and are rewritten at install from `brand.executable_name`. Kernel data stays under **`.zqk/`**.

ZQK is an operating system for AI + human hybrid engineering teams. It standardizes project goals, architecture, documentation, and task orchestration so multiple agents can collaborate safely and autonomously without losing context or drifting from requirements.

## ⚡ Quickstart (5 minutes)

There is **no brew formula and no public GitHub release** yet. Build from this checkout.

👉 **[Community First-Run Guide (Human + Agent)](./docs/onboarding/COMMUNITY_FIRST_RUN.md)**

**1. Build the CLI**
```sh
make          # → ./bin/zqk  (or ./bin/<brand.executable_name>)
./bin/zqk --version
```

**2. Initialize your project (Polyglot: Python, TS, Rust, Go, Docs)**
```sh
# In this checkout (already initialized): skip init.
# Greenfield:
mkdir my-project && cd my-project
/path/to/this-repo/bin/zqk system init --project-name my-project
/path/to/this-repo/bin/zqk quickstart
```

Do **not** `export ZQK_PROJECT_ROOT` in your shell profile. It silently attaches later commands to that checkout instead of the directory you are in.

**3. Seat your AI agent**
```sh
./bin/zqk system agent-onboard --format json
./bin/zqk system start-here
```

**4. Connect via Model Context Protocol (MCP)**
```sh
./bin/zqk mcp install
./bin/zqk mcp ensure --tcp 127.0.0.1:8443
# Cursor stdio: ./bin/zqk mcp cursor-adapter
```

---

## 🎁 What You Get

- **AI Agent Guardrails:** Enforceable policies that agents verify *before* modifying code.
- **Autonomous Project Context:** Agents discover goals, requirements, and architectural decisions without manual prompting.
- **Native Code Search:** Fast in-process AST and trigram search via `./bin/zqk grep` (alias `zgrep`).
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
- **[Agents on this SKU](./docs/onboarding/AI_AGENT_ONBOARDING.md)** — This tree does not ship the studio process pack.
- **[Architecture (this SKU)](./docs/architecture/README.md)** — Pointer only. Studio architecture dump is not shipped.

**Onboarding as curriculum (system objects):** Templates live under **[scripts/onboarding_roadmap/README.md](./scripts/onboarding_roadmap/README.md)**. This SKU does **not** ship `make alpha-help` or `zqk-ts`. Scheduler **is** shipped: `./bin/zqk scheduler start|stop|status`.

### Getting started (clean machine golden path)

From an empty project directory, using the binary you built in this repo:

```bash
/path/to/this-repo/bin/zqk system init --project-name my-project
/path/to/this-repo/bin/zqk object list
/path/to/this-repo/bin/zqk workflow whats-next --format json
```

Expected outcomes:

- `system init` creates `.zqk/` and `.zqk/process/` scaffolding.
- `object list` succeeds (kinds with rows after a seeded init).
- `./bin/zqk scheduler start` is optional. First-run CRUD does not require it.

If you run init a second time in the same directory:

- default `zqk system init` returns a clear "already initialized" error with next steps.
- use `zqk system init --legacy --discover` to inspect/populate an existing project without destructive overwrite.
- use `zqk system init --force` only when you explicitly want overwrite behavior.

### Docs that exist in this tree

- [Community first-run](./docs/onboarding/COMMUNITY_FIRST_RUN.md)
- [Quickstart / MCP](./docs/onboarding/QUICKSTART.md)
- [First-run object tutorial](./docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)
- [Architecture](./docs/architecture/README.md)
- [Contributing](./CONTRIBUTING.md)

### License

Open-core / Community: [Apache License 2.0](LICENSE) (see `NOTICE`). Enterprise modules are not in this tree.
