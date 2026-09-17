# Migration Strategy: File-Based to Graph (v1.0)

## Overview
This document outlines the strategy for migrating legacy file-based ontology objects into the new Graph Backend (Neo4j/MemGraph) within the ZQK (Zen Quantum Kernel) system.

## Phase Plan
The migration will be executed in three primary phases:

### Phase 1: Document Layer Population
- **Objective:** Scan existing YAML files in `.zqk/process/` and create baseline Document nodes in the graph.
- **Process:** 
  1. Use a robust YAML scanner (`pkg/migration/scanner`) to identify valid ontology objects.
  2. Ingest file paths, checksums, and metadata to establish traceability.
  3. Ensure idempotency by tracking migration state.

### Phase 2: Entity Layer Extraction
- **Objective:** Parse the scanned YAML documents to extract core ontology entities.
- **Process:**
  1. Parse YAML structures using `pkg/migration/parser`.
  2. Create Entity nodes mapping to ZQK object kinds (e.g., `goal`, `requirement`, `brand`, etc.).
  3. Generate embeddings (if applicable) for semantic search capabilities.

### Phase 3: Relationship Layer Construction
- **Objective:** Reconstruct relationships and dependencies between entities.
- **Process:**
  1. Extract references (e.g., `goal_refs`, `priority_plan_refs`) using `pkg/migration/resolver`.
  2. Create semantic edges in the graph backend to link Entities and Documents.
  3. Validate the structural integrity of the graph against schema expectations.

## Rollback Strategies
- **Pre-Migration Snapshot:** Before executing migration, a full snapshot of the Graph state will be exported.
- **Document Exporter (`pkg/migration/exporter`):** If the Graph becomes corrupted, the exporter tool will reverse-engineer Graph nodes back into standardized YAML files.
- **Idempotent Retries:** Migration steps must be idempotent, allowing partial restarts without data duplication.

## Data Integrity Verification Steps
- **Validation Gates:** Post-migration, run `zqk system check` or a specialized verification tool (`pkg/migration/validator`) to assert consistency.
- **Checksum Comparisons:** Compare checksums of source YAML files against the generated Document nodes.
- **Link Hydration Checks:** Ensure no orphaned references (`$req`, `$goal`) exist post-migration.

