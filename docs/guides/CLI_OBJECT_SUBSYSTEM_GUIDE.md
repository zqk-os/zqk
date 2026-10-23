# CLI Object Subsystem Architectural Guide

## Overview

The **ZQK Object Subsystem** (`cmd/zqk/object`) serves as the universal administrative interface and schema-governed membrane for the Zen Quantum Kernel (ZQK). Unlike file-based note vaults or transient chat memory stores, ZQK organizes all program elements—strategic goals, verifiable requirements, execution plans, criteria, backlog items, and architectural decisions—into strongly typed, versioned graph objects stored in Content Addressable Storage (CAS) under `.zqk/process/`.

The CLI Object subsystem provides:
1. **Atomic Lifecycle Transitions**: Strict state-machine progression (`originated` $\rightarrow$ `planned` $\rightarrow$ `ready` $\rightarrow$ `in_progress` $\rightarrow$ `complete`) enforced by invariant validators and AST evidence check-valves.
2. **First-Class Relational Traversal**: Bidirectional relationship management, parent-child rollups, graph path resolution, and neighbor topology discovery.
3. **Optimistic Concurrency & CAS Integrity**: Cryptographic content-addressing, collision-safe mutations, and timestamp-based concurrency guards preventing race conditions across autonomous agent swarms.
4. **Declarative Mutation Membrane**: Schema introspection, field type coercion, multi-kind bulk operations, and policy rule evaluation.

---

## Subsystem Architecture & File Layout

The object subsystem is implemented in `cmd/zqk/object/` and delegates to kernel abstractions in `pkg/objects/`, `pkg/storage/`, and `pkg/validation/`:

| Source File | Primary Responsibility |
| :--- | :--- |
| `object.go` | Root command constructor and flag dispatch |
| `create.go` | Interactive wizard and headless object minting pipeline |
| `update.go` | Atomic field updates, optimistic locking, and draft materialization |
| `delete.go` | Cascade deletion, reference unlinking, and confirmation bypass |
| `promote.go` / `demote.go` | Directed lifecycle transitions with validation precondition checks |
| `ref.go` | Graph reference manipulation (`add`, `remove`, `list`) with scope-lock enforcement |
| `neighbors.go` / `related.go` / `path.go` | Graph topology traversal, edge filtering, and pathfinding |
| `import_cmd.go` / `export.go` | Multi-format serialization (JSON, YAML, CSV) and bulk migration |
| `rename.go` | Atomic object identifier migration and reference rewriting |
| `root_fields.go` | Schema root inspection and schema discovery membrane integration |
| `policy_studio.go` | In-process policy DSL evaluation and rule prototyping |
| `update_all.go` | Bulk field updates with predicate filtering and schema guards |

---

## Command Taxonomy & Execution Semantics

### 1. Object Lifecycle & State Management
- `zqk object promote <id> [--to <status>]`: Advance an object along its valid lifecycle transitions. Verifies static invariants, operational proofs, and test completion gates.
- `zqk object demote <id> [--to <status>]`: Move an object backward to a recovery or planning status.
- `zqk object draft promote [--all]`: Materialize draft-plane objects parked under `.zqk/object_drafts/` into authoritative CAS storage.
- `zqk object draft sweep [--all]`: Purge unmaterialized draft objects.

### 2. Graph Traversal & Relational Operations
- `zqk object ref add <source_id> <target_id> [--field <name>]`: Establish typed graph relationships with duplicate suppression and cycle detection.
- `zqk object ref remove <source_id> <target_id>`: Safely sever directed graph edges.
- `zqk object neighbors <id> [--direction incoming|outgoing|both]`: Inspect adjacent graph nodes and edge weights.
- `zqk object related <id> [--depth <n>]`: Traverse multi-hop dependency clusters and hierarchically bound artifacts.
- `zqk object path <start_id> <end_id>`: Discover shortest directed or undirected traversal paths between any two objects.

### 3. Schema Discovery & Invariant Enforcement
- `zqk object root-fields [<kind>] [--all-kinds]`: Enumerate authorized schema fields, types, and validation constraints.
- `zqk object policy-studio`: Interactive and headless evaluation environment for kernel policy DSL rules.
- `zqk object update-all --kind <kind> --filter <predicate>`: Apply uniform schema updates across matching entities while preserving graph invariants.

---

## Security Model & Integrity Invariants

1. **Poison-Pill Prohibitions**: Direct manual edits to reference arrays (`requirement_refs`, `criteria_refs`, `goal_refs`) or status fields via raw update commands are prohibited. Mutating relationships requires `zqk object ref add/remove`; advancing status requires `zqk object promote/demote`.
2. **Git Mutation Evidence Gate**: Promoting execution units (`backlog_item`) to terminal `complete` requires non-merge git commits referencing the item ID and modifying code outside `.zqk/process/`.
3. **Audited Break-Glass Overrides**: In emergency recovery scenarios, human operators may supply `--override --reason-code="<explanation>"` to bypass sealed boundaries, creating an immutable audit trail.
