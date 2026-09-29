# Architecture Specifications

This directory contains formal engineering and architectural specifications for the ZQK Knowledge Kernel subsystem grammars, query planners, transaction execution engines, and interactive consoles.

## Specification Index

### ZPARQL Graph Query Language
- **[ZPARQL Graph Traversal Grammar](./SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.md)**: Formal EBNF grammar, pattern matching syntax, traversal algebra, and relational projection specifications.
- **[ZPARQL Query Planner](./SPEC-ZPARQL-QUERY-PLANNER.md)**: Query AST parsing, cost-based optimization, traversal index selection, and execution engine mechanics.
- **[ZPARQL Indexed Query Planner](./SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md)**: High-performance O(1) hash index resolution and reverse-reference lookup algorithms.
- **[ZPARQL Result Streaming](./SPEC-ZPARQL-RESULT-STREAMING.md)**: Low-latency iterator semantics, memory-bounded batching, and chunked HTTP/JSON-RPC streaming protocol.

### ZQL Declarative Mutations
- **[ZQL Declarative Mutation Grammar](./SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.md)**: EBNF grammar for declarative state transitions, CAS assertions, and multi-object mutations.
- **[ZQL Preflight Validation](./SPEC-ZQL-PREFLIGHT-VALIDATION.md)**: Pre-commit schema checks, policy compliance verifications, and fail-closed gate evaluation.
- **[ZQL Transaction Execution](./SPEC-ZQL-TRANSACTION-EXECUTION.md)**: Two-phase commit protocol, atomic WAL logging, rollback mechanics, and crash-resilient CAS updates.

### Interactive Tools & Consoles
- **[Object Inspector Console (SPEC-OBJECT-INSPECTOR-CONSOLE-001)](./SPEC-OBJECT-INSPECTOR-CONSOLE-001.md)**: Interactive terminal user interface (TUI) architecture, radar graphs, Policy Studio integration, and role-gated action palette.

---

## Related Documentation
- [Reference Manual](../manual/README.md)
- [ZPARQL Query Language Manual](../manual/ZPARQL_QUERY_LANGUAGE.md)
- [ZQL Mutations Manual](../manual/ZQL_MUTATIONS.md)
- [Architecture Overview](../architecture/README.md)
- [Documentation Index](../INDEX.md)
