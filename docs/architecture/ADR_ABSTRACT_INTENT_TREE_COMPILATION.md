# ADR: Abstract Intent Tree Compilation & Declarative AST Synthesis

## Status
Accepted

## Date
2026-11-20

## Context
Autonomous swarm orchestration requires translating conversational and programmatic human intent streams into deterministic, verifiable execution plans. Ad-hoc prompt interpretation introduces semantic drift, non-deterministic task ordering, and vulnerability to malformed or cyclic dependency specifications.

In Cycle 161, we introduce the Abstract Intent Tree Compilation and Declarative AST Synthesis engine in `pkg/community/intent_ast.go` to parse declarative intent expressions into validated Abstract Syntax Tree (AST) execution graphs, synthesize deterministic topological execution plans, and enforce fail-closed adversarial guards.

## Decision
1. **Intent Grammar Tokenization & AST Node Structure Invariants**:
   - The lexer tokenizes declarative intent streams into typed terminal and structural tokens, normalizing whitespace and rejecting unrecognized syntax or blank inputs before tree construction.
   - AST nodes enforce strict structural invariants: every node carries a unique non-empty identifier, a valid operation kind, and well-formed parameter bindings.
   - Static floor validation verifies terminal token integrity, balanced block delimiters, and required node attributes deterministically prior to plan synthesis.

2. **Deterministic Topological Execution Plan Synthesis & CAS Digesting**:
   - Validated AST nodes are lowered into a Directed Acyclic Graph (DAG) of execution steps with explicit dependency edges.
   - Topological sorting resolves execution order deterministically using canonical tie-breaking on node identifiers, guaranteeing identical execution plans for identical intent inputs with zero drift.
   - Synthesized execution trees are serialized canonically and hashed to produce deterministic Content-Addressable Storage (CAS) digests for verifiable provenance and deduplication.

3. **Adversarial Cycle Interception, Dangling Dependency Rejection & Syntax Guards**:
   - **Cycle Detection**: Graph traversal performs three-color depth-first cycle detection during compilation, rejecting any direct self-reference or transitive dependency loop with a fail-closed error.
   - **Dangling Dependency Rejection**: All declared dependency references must resolve to defined nodes within the intent tree; unresolved or forward-dangling references are rejected prior to execution.
   - **Syntax & Payload Guards**: Malformed prompt payloads, unbounded nesting depth, duplicate node identifiers, and empty intent streams are intercepted at the parser boundary.

## Consequences
- Guarantees deterministic, zero-drift compilation from declarative intent expressions to verifiable execution plans.
- Enables content-addressed caching and cryptographic provenance of compiled intent trees via canonical CAS digests.
- Prevents runtime deadlocks and infinite recursion by failing closed on cyclic dependencies, dangling references, and malformed syntax at compile time.
