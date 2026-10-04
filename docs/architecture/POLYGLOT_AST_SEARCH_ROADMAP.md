# Polyglot AST Code Search Architecture & Roadmap (`zqk grep`)

## Executive Summary

Autonomous AI coding agents spend over 70% of their token budgets on codebase discovery and symbol retrieval. Traditional developer tools force a false dichotomy:
1. **Raw Text `grep` / `ripgrep`**: Fast, but syntax-blind. Dumps thousands of tokens of comments, strings, and irrelevant matches into agent context windows, inducing hallucinations and rapid context window exhaustion.
2. **Heavy Language Servers (LSP)**: Semantically rich, but heavyweight, stateful, and slow to initialize across large, multi-language enterprise repositories.

The **Zen Quantum Kernel (ZQK)** solves this via `zqk grep` (alias `zgrep`): an in-process, pure-Go, trigram-accelerated code search engine with structural AST queries and strict token budgeting (`--max-tokens 2000 -f json`).

This document outlines the architectural roadmap for expanding `zqk grep` from its current Go-native engine to a universal, **polyglot Tree-sitter AST indexing engine** covering Python, TypeScript/JavaScript, Rust, Java, C/C++, and beyond.

---

## 1. Current State: In-Process Go AST Engine

### Current Capabilities
- **Sub-15ms Trigram Pre-Filtering**: Maintains an in-memory / persistent trigram index cache (`.zqk/cache/trigram.idx`) to eliminate non-matching files before syntax parsing.
- **Native Go AST Symbol Traversal**: Uses Go's standard `go/parser` and `go/ast` packages to extract and query:
  - Declarations: `--kind func|method|struct|interface|type|var|const`
  - Receiver Methods: `--recv <TypeName>` (e.g. methods attached to `Engine` or `Storage`)
- **Strict Token Budgeting**: Capped JSON envelopes (`--max-tokens 4000 -f json`) ensure LLM agents never receive unbudgeted, context-overflowing file dumps.
- **Zero External Dependencies**: Operates entirely within the compiled `zqk` binary without invoking shell `grep`, `awk`, or `ripgrep`.

---

## 2. Polyglot Architecture: Tree-sitter Integration

To support multi-language enterprise monorepos without sacrificing sub-30ms retrieval speeds, ZQK is adopting an embedded **Tree-sitter** grammar architecture.

```
┌─────────────────────────────────────────────────────────────┐
│                    zqk grep CLI / MCP API                   │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────▼──────────────────┐
            │   Trigram Index Filter (<10ms)      │
            │   Narrows 50,000 files -> 5 files   │
            └──────────────────┬──────────────────┘
                               │
            ┌──────────────────▼──────────────────┐
            │      Language Parser Dispatcher      │
            └──────┬───────────┬───────────┬──────┘
                   │           │           │
       ┌───────────▼┐    ┌─────▼─────┐   ┌─▼───────────┐
       │   Go AST   │    │  Python   │   │ TypeScript/ │
       │  (native)  │    │Tree-sitter│   │Tree-sitter  │
       └───────────┬┘    └─────┬─────┘   └─┬───────────┘
                   │           │           │
            ┌──────▼───────────▼───────────▼──────┐
            │       Unified Symbol Schema         │
            │  (struct, class, func, method, intf)│
            └──────────────────┬──────────────────┘
                               │
            ┌──────────────────▼──────────────────┐
            │ Token Budgeter & JSON/Lines Envelope│
            └─────────────────────────────────────┘
```

### Unified Symbol Schema
Tree-sitter grammars produce distinct AST node names across languages (`class_definition` in Python vs `class_declaration` in TypeScript vs `struct_item` in Rust). ZQK maps language-specific nodes into a **Canonical Symbol Taxonomy**:

| Canonical Kind | Go Equivalent | Python Equivalent | TypeScript / JS | Rust Equivalent | Java / C# |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `struct` | `ast.StructType` | `@dataclass`, `class` | `interface`, `type` | `struct_item` | `class`, `record` |
| `class` | N/A | `class_definition` | `class_declaration` | N/A | `class_declaration` |
| `func` | `ast.FuncDecl` | `function_definition` | `function_declaration` | `function_item` | `method_declaration` |
| `method` | `ast.FuncDecl` (recv) | `function_definition` | `method_definition` | `impl_item` | `method_declaration` |
| `interface`| `ast.InterfaceType` | `Protocol`, `ABC` | `interface_declaration`| `trait_item` | `interface_declaration`|
| `enum` | `const (...)` | `Enum` | `enum_declaration` | `enum_item` | `enum_declaration` |

---

## 3. Performance & Memory SLAs

1. **Cold Repository Scan**:
   - Monorepo of 100,000 LOC indexed in <1.2 seconds.
   - Persistent index stored in `.zqk/cache/ast_index.db` using compact binary encoding.
2. **Warm Query Execution**:
   - Trigram candidate narrowing: **<10ms**.
   - Tree-sitter AST symbol resolution on candidates: **<15ms**.
   - Total round-trip latency: **<25ms**.
3. **Memory Footprint**:
   - Resident set size (RSS) overhead of <50MB during active query execution.
   - Zero background memory leak; memory reclaimed immediately post-query.

---

## 4. Implementation Milestones

### Phase 1: Pure-Go Baseline & Verification (Completed)
- [x] In-process trigram engine (`pkg/search/trigram.go`).
- [x] Go AST parser with symbol, method, and receiver queries (`pkg/search/ast.go`).
- [x] Token budgeting and structured JSON output for AI agent consumption (`pkg/search/budget.go`).
- [x] Integration with `zqk grep` and `zgrep` CLI aliases.

### Phase 2: Python & TypeScript/JavaScript (Q4 2026)
- [ ] Embed Tree-sitter runtime via CGO-free WebAssembly or pure-Go bindings (`smacker/go-tree-sitter` or `wasmer-go`).
- [ ] Implement Python grammar queries (`class_definition`, `function_definition`, decorators).
- [ ] Implement TypeScript/JavaScript grammar queries (`class_declaration`, `interface_declaration`, `method_definition`).
- [ ] Add language flags: `zqk grep --lang py,ts --kind class 'Service'`.

### Phase 3: Systems Languages — Rust, Java, C/C++ (Q1 2027)
- [ ] Add Rust grammar queries (`struct_item`, `trait_item`, `impl_item`).
- [ ] Add Java and C# grammar queries (`class_declaration`, `interface_declaration`).
- [ ] Add C/C++ header and symbol queries.

### Phase 4: MCP Mesh & Autonomous Swarm Hook (Q2 2027)
- [ ] Expose polyglot AST queries as a first-class MCP tool (`tools/zqk_grep`) in `zqk mcp`.
- [ ] Autonomous context pre-fetch in `zqk do`: automatically retrieve relevant AST symbols before dispatching agent subtasks.

---

## 5. Summary

By bridging trigram pre-filtering with embedded Tree-sitter grammars and unified cross-language schemas, ZQK provides AI coding agents with the highest-precision, lowest-latency, and most token-economical code search engine in the agentic ecosystem.
