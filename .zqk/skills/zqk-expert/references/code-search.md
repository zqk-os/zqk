# Native Code Search & AST Inspection (`zqk grep` / `zgrep`)

**Last updated:** 2026-09-17  
**Canonical Command:** `zqk grep [query] [path] [flags]` (alias: `zgrep`)

## 1. Overview & Architecture

ZQK provides a fast, in-process, pure-Go code search engine located at `pkg/search` and exposed via the `zqk grep` CLI command. It eliminates reliance on external shell tools (`ripgrep`, `grep`, `find`) and provides sub-15ms code searches directly within the Go runtime.

### Key Capabilities
- **Trigram Index Acceleration:** Accelerated sub-string and literal searches without disk thrashing.
- **Go AST Structural Queries:** Parse and inspect Go Abstract Syntax Trees in-memory to find functions, methods, structs, and interfaces without regex brittle-pattern failures.
- **Token-Budgeted Output:** Built-in token bounding (`--max-tokens <int>`) with structured JSON output (`-f json`) designed specifically for AI agent context windows.
- **Fail-Closed Workspace Bounding:** Automatically respects `.gitignore`, `.zqk-state`, and vendor boundaries unless explicitly overridden.

---

## 2. CLI Usage & Query Patterns

### Literal & Regex Search
```bash
# Literal string search
zqk grep "MaterializedView"

# Case-insensitive search
zqk grep -i "wal_subscriber"

# Regular expression query
zqk grep -e "func.*Start\("

# Bounded by path and file extension
zqk grep "Execute" pkg/cli --ext .go,.yaml

# Context lines
zqk grep "NewEngine" -C 3
```

### Go AST Structural Queries
Use `--ast` to activate structural AST search. AST search parses `.go` files into AST nodes and matches declarations deterministically:

```bash
# Discover all struct definitions
zqk grep --ast --kind struct

# Discover all interface definitions
zqk grep --ast --kind interface

# Discover all top-level functions
zqk grep --ast --kind func

# Discover all methods bound to a specific receiver type
zqk grep --ast --recv Engine

# Combine kind and receiver filter
zqk grep --ast --kind method --recv StateMachine
```

Supported `--kind` filters: `func`, `method`, `struct`, `interface`, `type`, `var`, `const`.

### Agent Context & Token Budgeting
When an AI agent performs code discovery, raw grep output can blow context windows. Always use token-budgeted JSON formatting:

```bash
# Limit output to 2000 tokens in JSON format
zqk grep "error" --max-tokens 2000 -f json

# Target a specific subsystem with token budget
zqk grep "RegisterKind" pkg/storage --max-tokens 1500 -f json
```

---

## 3. Best Practices for Swarm Agents

1. **Avoid External Grep/Find:** Never use external sub-shell tools (`find . | xargs grep`) or rely on OS-specific grep flags. Always use `zqk grep`.
2. **Explore Before Modifying:** When analyzing type hierarchies or receiver methods during refactoring, use `zqk grep --ast --recv <Type>` before writing custom inspection scripts.
3. **Keep Reference in Skills:** Align with `go-ast-expert` and `go-architect` skills when making multi-file structural modifications.
