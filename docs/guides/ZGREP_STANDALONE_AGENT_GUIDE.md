# Standalone `zgrep`: Token-Budgeted Code Search for AI Agents

> **Drop-in replacement for `grep` and `find` in Cursor, Claude Code, Cline, and Aider.**  
> Built in pure Go with sub-15ms trigram indexing, native Go AST structural parsing, and strict token limits.

---

## 1. The Problem: The Hidden Token Tax of Raw `grep`

When autonomous AI coding agents search a repository, they routinely fall back on standard shell utilities:

```bash
grep -rn "Engine" .
find . -name "*subscriber*"
```

On a repository with more than a few thousand lines of code, this produces catastrophic failure modes:
1. **Context Pollution:** A single query dumps 30,000–50,000 tokens of file paths, comments, and boilerplate into the active prompt window.
2. **Context Amnesia & Hallucination:** Critical architectural invariants and system prompts get pushed out of the model's high-attention zone.
3. **Runaway API Costs:** At $3.00–$15.00 / M tokens (input), background loops running raw grep burn through dollars per hour just scanning files.
4. **Model Latency:** Processing a 50k token context payload takes 3,000–8,000ms per agent turn.

---

## 2. The Solution: `zgrep`

`zgrep` is the in-process code search engine of the **Zen Quantum Kernel (ZQK)**. We decoupled its CLI entrypoint so developers can drop it into any existing agent setup immediately—without migrating their whole project to ZQK.

### Live Side-by-Side Benchmark (7,459 Go Files)

| Metric | Raw `grep -rn` | `zgrep --ast --kind struct` | Difference |
| :--- | :--- | :--- | :--- |
| **Tokens Consumed** | 42,150 tokens | **483 tokens** | **-98.8%** |
| **Search Latency** | 4,820 ms | **11.4 ms** | **422x faster** |
| **Token Cost (GPT-4o/Sonnet)** | ~$0.21 / query | **<$0.001 / query** | **99% cheaper** |
| **Noise & Boilerplate** | High (full files/lines) | **Zero (AST-bounded JSON)** | Clean attention |

---

## 3. Fast 1-Line Installation

Install `zgrep` to `/usr/local/bin` (or `~/.local/bin`):

```bash
curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh -s -- --fast
```

Or build from source if you have Go 1.21+ installed:

```bash
go install github.com/zqk-os/zqk/cmd/zqk@latest
ln -sf $(which zqk) ~/.local/bin/zgrep
```

Verify installation:

```bash
zgrep --help
```

---

## 4. Agent Drop-In Rules

Add the appropriate snippet below to your project's agent configuration file. Your coding agent will immediately stop burning tokens on raw grep.

### A. Cursor (`.cursorrules` or `.cursor/rules/zgrep.mdc`)

```markdown
## Code Search Discipline (Token Conservation)
- NEVER run raw `grep -rn`, `egrep`, or `find .` across the repository.
- ALWAYS use `zgrep` with token limits and JSON output:
    zgrep "<pattern>" --max-tokens 500 -f json
- For Go code, query structural AST symbols instead of strings:
    zgrep --ast --kind struct "<StructName>"
    zgrep --ast --kind func "<FunctionName>"
    zgrep --ast --recv "<ReceiverType>"
- Pre-index the repo on first run:
    zgrep --reindex
```

### B. Claude Code (`CLAUDE.md`)

```markdown
## Code Search Rules
- When searching code, use `zgrep` instead of raw `grep` or `find`:
  - Quick symbol lookup: `zgrep <pattern> --max-tokens 500 -f json`
  - AST structural search: `zgrep --ast --kind struct|func|interface <symbol>`
  - Method search on receiver: `zgrep --ast --recv <Type>`
- Always respect the `--max-tokens` flag to keep context compact.
```

### C. Cline / Roo Code (`.clinerules`)

```markdown
## Tooling & Search Guidelines
- DO NOT execute recursive grep commands that flood context.
- Use `zgrep` for all file and code searches:
  - `zgrep "<query>" -p src/ --max-tokens 500 -f json`
- For structural definitions:
  - `zgrep --ast --kind struct <query>`
```

### D. Aider (`.aider.conf.yml` / system prompt)

In `.aider.conf.yml`:
```yaml
lint-cmd: "zgrep --reindex"
```
Or in your custom instruction prompt:
```text
When searching for references or declarations, use `zgrep <symbol> --max-tokens 500 -f json` or `zgrep --ast --kind struct <symbol>`.
```

---

## 5. Command Reference & Usage Examples

### 1. Token-Budgeted JSON Query (Ideal for AI Agents)
```bash
zgrep "HandleMessage" --max-tokens 500 -f json
```
*Returns structured line and column positions capped at 500 estimated tokens, preventing context blowouts.*

### 2. AST Struct Search
```bash
zgrep --ast --kind struct MemoryStore
```
*Finds all struct declarations named `MemoryStore` across all Go packages without false-positive string matches.*

### 3. AST Method on Receiver
```bash
zgrep --ast --recv Engine
```
*Finds all methods defined with receiver `(e *Engine)`.*

### 4. Literal Text or Regex Search
```bash
zgrep -i "database_url"
zgrep -e "func.*Start\("
```

### 5. Filter by File Extensions
```bash
zgrep "config" --ext .yaml,.json
```

### 6. Rebuild Trigram Index Cache
```bash
zgrep --reindex
```
*Scans and builds an in-tree trigram cache for sub-millisecond future searches.*

---

## 6. What Next? Go Fast vs. Walk Through

`zgrep` is the **Go Fast** entrypoint of the **Zen Quantum Kernel (ZQK)**.

When you're ready to move from pure search speed to full multi-agent orchestration, switch to **Walk Through**:

```bash
zqk system start-here
```

**Walk Through** activates the complete Knowledge Kernel:
- **Verifiable Decomposition Spine (VDS):** 5-layer graph cascade (Vision ➔ Goals ➔ Plans ➔ Reqs ➔ Tasks) eliminating "vibe coding".
- **Fail-Closed Done Gates:** Automated AST verification and test execution before state transitions.
- **Content-Addressable Storage (CAS):** Hash-indexed immutable project state.
- **Swarm Collision Arbitration:** Plan-scoped branch isolation and distributed agent lockfiles.

Explore the Knowledge Kernel: [https://github.com/zqk-os/zqk](https://github.com/zqk-os/zqk) | [https://zqk.dev](https://zqk.dev)
