## Quick Start (< 2 min)

**Install** (choose one):

```sh
# Option A — Binary release (once repo is public, no token required):
curl -sSL https://raw.githubusercontent.com/lanceman/zqk/main/scripts/install.sh | sh

# Option B — go install (Go 1.21+):
go install github.com/lanceman/zqk/cmd/zqk@latest

# Option C — Build from source:
git clone --depth=1 https://github.com/lanceman/zqk.git && cd zqk
make zqk   # → bin/zqk (public product name; enterprise tools use distinct names)
```

**First agent** (< 60 sec after install):

```sh
mkdir my-project && cd my-project
zqk system init --project-name my-project   # scaffold .zqk/ + docs/architecture/
zqk workflow whats-next                     # discover mission + next tasks
zqk mcp proxy --tcp 0.0.0.0:7777           # expose MCP tools to your AI tool
```

> **Capability-secured, deterministic, local-first.** ZQK is a host-level FSM for AI + human teams — not a prompt wrapper. [Architecture »](./docs/architecture/README.md)

---

## ZQK (Zen Quantum Kernel)


ZQK (Zen Quantum Kernel) is an operating system for AI + human hybrid teams, built on a distributed knowledge kernel architecture. It standardizes project goals, documentation, automation, and workflow orchestration so multiple agents can collaborate safely without losing context.

### 🚀 For AI Agents: Start Here

**⚠️ If you are an AI agent joining this project, you MUST read the onboarding documentation first:**

👉 **[AI Agent Onboarding Guide](./docs/onboarding/AI_AGENT_ONBOARDING.md)**

This guide covers essential routines, safety protocols, development workflows, and project conventions that are critical for effective collaboration. For **current plan/convergence pointers**, see **[docs/onboarding/AGENT_ONBOARDING_SNAPSHOT.md](./docs/onboarding/AGENT_ONBOARDING_SNAPSHOT.md)** after the main guide.

**Code discipline:** Before substantive edits, **[PRE_CHANGE_CHECKLIST.md](./docs/architecture/PRE_CHANGE_CHECKLIST.md)**. After behavior is correct (before merge), run the **§13** post-verify for **semantic density** — **[DRY_PATTERN_EXTRACTION.md](./docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md)**; session reminder file **`docs/architecture/README.md`** (compliance + **glossary** reminders; regenerate with **`scripts/go test ./....sh`**; **`scripts/README.md`**).

### Getting started (first object)

For **humans** after **`zqk init`** in a project directory: follow **[First-run object tutorial](./docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** (`object template` → `create` → `get` → `update` with the small **`question`** kind). This matches **Journey B** in **[CLI alpha launch plan](./docs/architecture/CLI_ALPHA_LAUNCH_PLAN.md)**. The same tutorial is also listed under **[docs/onboarding/README.md](./docs/onboarding/README.md)**.

**Onboarding as curriculum (system objects):** Templates for a full onboarding track (priority plan, workstream, milestone, backlog items, optional one-shot seed job) live under **[scripts/onboarding_roadmap/README.md](./scripts/onboarding_roadmap/README.md)**. **`make alpha-help`** (repo root) lists alpha bundle/metrics targets and points to that curriculum and the certification design narrative. Isolated evaluation with **`zqk-ts`**: **[ONBOARDING_EVALUATION_SCENARIO.md](./docs/architecture/README.md)**.

### Getting started (clean machine golden path)

From an empty project directory:

```bash
zqk system init --project-name my-project
zqk object list backlog_item --format table
zqk system check --fast --allow-degraded
```

Expected outcomes:

- `system init` creates `.zqk/` and `docs/architecture/` scaffolding.
- `object list backlog_item` succeeds (often zero rows right after a new init).
- `system check --fast` completes without blocking violations.

If you run init a second time in the same directory:

- default `zqk system init` returns a clear "already initialized" error with next steps.
- use `zqk system init --legacy --discover` to inspect/populate an existing project without destructive overwrite.
- use `zqk system init --force` only when you explicitly want overwrite behavior.

Troubleshooting during first hour:


### Security basics (alpha)

- **CLI output and logging:** user-facing output and logs follow **`docs/architecture/README.md`** (POL-CODE-007): use structured output and the logging profile—avoid echoing secrets, tokens, or raw credential fields in command output or copy-paste examples.

### 📚 Documentation

- **[Assessment & onboarding index](./docs/architecture/ASSESSMENT_AND_ONBOARDING_INDEX.md)** — central map for alpha/onboarding/architecture entry points; **`doc_entry`** anchors include **`DOC-EXAMPLE`** (this index), **`DOC-EXAMPLE`** ([CLI alpha plan](./docs/architecture/CLI_ALPHA_LAUNCH_PLAN.md)), **`DOC-EXAMPLE`** ([backlog `document_refs` / auto-link design](./docs/architecture/README.md)); **`glossary_term`** **`GLS-EXAMPLE`** (*documentation graph*); platform backlog **`BLI-EXAMPLE`** tracks automation of that graph (optional long-horizon work)
- **[Architecture Documentation](./docs/architecture/README.md)** - Architecture decisions, patterns, and design documents
- **[Coding Best Practices](./docs/best-practices/coding/README.md)** - Coding standards and best practices
- **[Observability Best Practices](./docs/best-practices/observability/README.md)** - Logging, metrics, and monitoring
- **[Refactoring Documentation](./docs/architecture/README.md)** - Refactoring plans and status
- **[Codebase Summary](./docs/reports/CODEBASE_SUMMARY.md)** - Comprehensive codebase statistics and analysis

### Why It Exists
- **Goal traceability** – every artifact (docs, scripts, runs) links back to project goals.
- **Predictable documentation** – doc index + CLI make it effortless to discover the
  right guidance or script from the terminal.
- **Template packs** – new projects (Flutter, Node, Rust, etc.) start with sensible,
  opinionated defaults while remaining customizable.
- **Release intelligence** – history snapshots and tagging deliver rollups that explain
  why goals were (or were not) met.
- **Distributed Knowledge Kernel** – git-based distribution with PKI authority verification
  enables collaborative operating system model across multiple agents.

### Strategic Pivot

ZQK represents a strategic pivot from the zqk prototype, incorporating:
- **Graph-based backend architecture** with pluggable graph database support (MemGraph, Neo4j, RDF stores)
- **Comprehensive system ontology** formalizing all objects and relationships
- **Distributed knowledge kernel** with PKI-based authority verification
- **CLI ontology** designed for both AI-agent and human interpretation

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
