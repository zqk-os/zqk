# Architecture Decision Record: Graph Database Engine (QLever vs Neo4j)

**Last Verified:** 2026-08-31


## Context
The ZQK Knowledge Kernel requires a backend capable of storing complex, property-rich nodes (YAML objects) and tracing execution graphs in real-time. We evaluated QLever (an extremely fast SPARQL engine for RDF graphs) against Property Graph models (Neo4j/MemGraph) to determine if QLever's billion-triple performance claims could scale ZQK's architecture.

## Evaluation
1. **Data Model**: ZQK relies heavily on rich properties embedded in objects. A Property Graph (Neo4j) maps these properties directly onto single nodes. QLever (RDF) requires generating a separate triple for every single property, leading to massive graph bloat for our schema.
2. **Execution Tracing**: ZQK traces agent executions, requiring edge properties (e.g., duration, step index, status). Neo4j treats edges as first-class citizens that hold properties natively. QLever requires complex RDF reification to attach metadata to edges, significantly degrading the developer experience and query simplicity.
3. **Workload Profile**: QLever is incredibly fast for bulk loading static analytical data. However, ZQK requires real-time, transactional writes to log live agent execution paths and dynamically update orchestration state.

## Decision
**Rejected QLever**. While QLever's performance on massive, read-heavy analytical RDF graphs is unparalleled, a Property Graph (Neo4j or MemGraph) is unequivocally the correct architecture for ZQK's operational, property-heavy, transactional workload.

## Status
Accepted
