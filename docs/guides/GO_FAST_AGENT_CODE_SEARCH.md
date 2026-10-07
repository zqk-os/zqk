# Go Fast: Token-Budgeted Agent Code Search with zqk grep

> **Drop-in code search for Cursor, Claude Code, Cline, and Aider.**  
> Built into the single ZQK binary with sub-15ms trigram indexing, Go AST structural parsing, and strict token limits.

---

## 1. Overview

ZQK provides two primary paths for developers:
1. **Go Fast:** Immediate search speed and token conservation for coding agents with zero configuration overhead.
2. **Walk Through:** The full Knowledge Kernel (5-layer graph cascade, automated done-gates, and multi-agent swarm orchestration).

This guide covers **Go Fast**: configuring your daily coding assistant to use `zqk grep` (symlinked into your PATH as `zgrep`) to eliminate context window blowouts and search latency.

---

## 2. Why Agents Need Bounded Search

When autonomous coding agents search a repository, they routinely fall back on standard shell utilities like `grep -rn` or `find .`. On any non-trivial codebase, this creates two distinct failure modes:

1. **Context Flooding & Amnesia:** A single recursive search can dump tens of thousands of tokens of file paths, comments, and boilerplate into the active prompt window. Critical architectural instructions and system constraints get pushed out of the model's high-attention zone.
2. **Truncation Blindness:** To defend against flooding, many modern harnesses arbitrarily truncate shell output after a fixed line or byte limit. The model never sees the code symbols it actually needs, leading to hallucinated imports and broken edits.
3. **Compute and VRAM Strain:** When using fine-tuned Small Language Models (SLMs) on local workstations or edge devices (like a Raspberry Pi running Linux arm64), context window budgets are scarce. Dumping unstructured text exhausts VRAM and degrades reasoning speed.

---

## 3. The zqk grep Approach

Rather than relying on unparsed text dumps, `zqk grep` treats your codebase as structured data:

* **Token-Bounded Output (`--max-tokens 500 -f json`):** Returns compact, structured JSON containing file paths, line ranges, and matched symbols. The output never exceeds the configured token budget.
* **Go AST Structural Queries (`--ast`):** Finds exact definitions (structs, interfaces, function declarations, and method receivers) directly via AST parsing, eliminating string false positives.
* **Sub-15ms Trigram Indexing:** Pre-builds an in-tree trigram cache for fast multi-file lookups.
* **Single Zero-Dependency Binary:** Everything is compiled directly into the single `zqk` binary (which happens to be under 70MB), requiring no external databases, Python runtimes, or background vector servers.

---

## 4. Installation

Install ZQK and link `zgrep` to your PATH:

```bash
# Automated install (piped runs default to Go Fast)
curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh
```

Or install via Homebrew:

```bash
brew tap zqk-os/tap && brew install zqk
```

Or build from source:

```bash
go install github.com/zqk-os/zqk/cmd/zqk@latest
ln -sf $(which zqk) ~/.local/bin/zgrep
```

Verify your installation:

```bash
zgrep --help
```

---

## 5. Agent Configuration Drop-Ins

Add the relevant snippet below to your project's agent configuration file. Your coding agent will immediately adopt token-budgeted code search.

### A. Cursor (`.cursorrules` or `.cursor/rules/zgrep.mdc`)

```markdown
## Code Search Discipline (Token Conservation)
- NEVER run raw recursive grep, egrep, or find across the repository.
- ALWAYS use zgrep with token limits and JSON output:
    zgrep "<pattern>" --max-tokens 500 -f json
- For Go code, query structural AST symbols instead of strings:
    zgrep --ast --kind struct "<StructName>"
    zgrep --ast --kind func "<FunctionName>"
    zgrep --ast --recv "<ReceiverType>"
- Pre-index the repository on first run:
    zgrep --reindex
```

### B. Claude Code (`CLAUDE.md`)

```markdown
## Code Search Rules
- When searching code, use zgrep instead of raw grep or find:
  - Quick symbol lookup: zgrep <pattern> --max-tokens 500 -f json
  - AST structural search: zgrep --ast --kind struct|func|interface <symbol>
  - Method search on receiver: zgrep --ast --recv <Type>
- Always respect the --max-tokens flag to prevent context window blowouts.
```

### C. Cline / Roo Code (`.clinerules`)

```markdown
## Tooling & Search Guidelines
- DO NOT execute recursive grep commands that flood context.
- Use zgrep for all file and code searches:
  - zgrep "<query>" -p src/ --max-tokens 500 -f json
- For structural definitions:
  - zgrep --ast --kind struct <query>
```

### D. Aider (`.aider.conf.yml` or system prompt)

In `.aider.conf.yml`:
```yaml
lint-cmd: "zgrep --reindex"
```

In your custom instruction prompt:
```text
When searching for references or declarations, use `zgrep <symbol> --max-tokens 500 -f json` or `zgrep --ast --kind struct <symbol>`.
```

---

## 6. Command Reference & Examples

### 1. Token-Budgeted JSON Query (Recommended for Agents)
```bash
zgrep "HandleMessage" --max-tokens 500 -f json
```
Returns structured symbol coordinates capped strictly at 500 tokens.

### 2. AST Struct Declaration Search
```bash
zgrep --ast --kind struct MemoryStore
```
Finds struct definitions named `MemoryStore` across Go packages without false-positive string hits.

### 3. AST Method on Receiver
```bash
zgrep --ast --recv Engine
```
Finds all methods defined with receiver `(e *Engine)`.

### 4. Text or Regex Search
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

---

## 7. Ready for Path 2 (Walk Through)?

`zgrep` gives your agents immediate speed and token discipline. When you are ready to explore the rest of the kernel, launch the interactive walkthrough:

```bash
zqk system start-here
```

**Walk Through** activates the complete Knowledge Kernel:
* **Verifiable Decomposition Spine (VDS):** 5-layer graph cascade (Vision, Goals, Priority Plans, Requirements, Backlog Items) that eliminates vibe coding.
* **Fail-Closed Done Gates:** Automated AST verification and test execution before state transitions.
* **Content-Addressable Storage (CAS):** Hash-indexed immutable project state.
* **Multi-Agent Collision Arbitration:** Plan-scoped branch isolation and distributed agent lockfiles.

Learn more at [https://github.com/zqk-os/zqk](https://github.com/zqk-os/zqk) and [https://zqk.dev](https://zqk.dev).
