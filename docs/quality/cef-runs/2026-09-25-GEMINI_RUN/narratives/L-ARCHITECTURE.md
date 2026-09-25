# L-ARCHITECTURE Narrative: Architecture & Abstractions Evaluation

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-ARCHITECTURE`  
**Density Class:** `D-LOW` (Top-N Budget: 10; Emitted: 10)  
**Primary Axes:** `MNT` (Maintainability), `MOD` (Modularity), `RDB` (Readability)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files; 709 Go packages; 1,100,614 Go LOC)

---

## 1. Executive Assessment

The ZQK codebase represents an ambitious, event-driven knowledge kernel architecture supporting declarative spec-driven development, content-addressable storage (CAS), write-ahead logging (WAL), and agentic coordination. However, comprehensive architectural analysis across package boundaries, interface contracts, and execution paths reveals critical systemic antipatterns and boundary failures:

1. **Subprocess Fork Indirection Bypassing In-Process Architecture (`F-ARCH-001`, High):** In multiple critical subsystems—most prominently the Model Context Protocol (MCP) server bridge in `pkg/mcp/cli_bridge.go` and the Scheduler Convergence Engine in `pkg/scheduler/convergence_engine.go`—internal components execute domain operations by shelling out to the `bin/zqk` CLI binary via `os/exec` (`exec.CommandContext`), capturing stdout strings, regex-filtering log events, and re-deserializing JSON. This introduces severe process fork overhead, latency, memory spikes, and risks subprocess fork-bombs (`isUnsafeCLIBridgeBinary`), entirely bypassing in-process Go component boundaries.
2. **Monolithic God Package in `pkg/storage` (`F-ARCH-002`, High):** Spanning 324 Go files, `pkg/storage` has expanded far beyond persistence concerns to absorb embedded/external TSDB providers (`tsdb_provider_embedded.go`), metrics collectors (`unified_metrics_collector.go`), asynchronous validation strategies (`validation_strategy_async.go`), YAML formatting (`yaml_formatter.go`), waitgroup lifecycle monitoring (`waitgroup_manager.go`), and synonym loading. This directly conflicts with dedicated sibling packages (`pkg/metrics`, `pkg/validation`, `pkg/concurrency`, `pkg/kindsynonyms`).
3. **Asymmetric Interface Contracts in `ObjectStorageProvider` (`F-ARCH-003`, High):** The core storage contract in `pkg/storage/object_storage_interface.go` defines a monolithic 25-method interface implemented by both `FileObjectStorage` and `GraphObjectStorage`. However, their physical semantics and capabilities diverge sharply: `FileObjectStorage` implements a complex 4-tier storage hierarchy (CAS, draft plane, stream storage, and direct YAML) with listing index queues and WAL, none of which exist in `GraphObjectStorage`. Conversely, `FileObjectStorage.Query` explicitly rejects non-filter queries (`query.Type != QueryTypeFilter`) with an error, causing consumers like `pkg/verification/engine.go` (which issues `QueryTypeCypher`) to fail unconditionally, violating the Liskov Substitution Principle (LSP).
4. **Unindexed O(N) Disk & Deserialization Scan in `GetNeighbors` (`F-ARCH-004`, High):** In `pkg/storage/object_storage_file_graph_impl.go` (`GetNeighbors`, lines 247-333), incoming graph relationship traversal ("incoming" or "both") performs an unindexed full scan of the entire `.zqk/process/` directory tree, reading every YAML file on disk and parsing its contents in memory on every query. This violates the kernel's own Magician Principle and creates severe I/O shockwaves under load.
5. **Untyped `map[string]any` Domain Architecture Driving 2,300+ Type Assertions (`F-ARCH-005`, High):** The core domain architecture treats objects as generic `map[string]any` dictionaries (documented as the "croptop" approach in `pkg/objects/types.go`). Across `pkg/`, an automated census identifies over 2,308 manual `.(string)` type assertions, resulting in rampant nil-dereference risk, boilerplate type assertions, and a complete absence of compiler-enforced schema safety.
6. **Package Sprawl and Micro-Package Atomization (`F-ARCH-006`, Moderate):** The repository has proliferated into 709 Go packages across 137 top-level directories in `pkg/`. Code generation has atomized logic into extreme micro-packages: `pkg/specbuilder/bldr_enum_v1` contains 329 isolated packages, each containing a single ~15-line enum file. Over 50 root packages under `pkg/` contain 1 or fewer Go files, defying the documented 3-Tier Layering Model in `community-arch-design`.
7. **Subsystem Fragmentation Across 15+ Orchestration & Swarm Packages (`F-ARCH-007`, Moderate):** Agent orchestration, dispatching, and execution loops are fragmented across at least 15 uncoordinated packages (`pkg/orchestration`, `pkg/primaryorch`, `pkg/agentorch`, `pkg/swarm`, `pkg/scheduler`, `pkg/coordination`, `pkg/seatworker`, and 9 `pkg/agent*` packages). Several packages exist as 37-line stubs (`pkg/agentorch`), while dispatch and convergence logic is duplicated.
8. **Anti-Idiomatic Control-Flow DSLs and Abandoned Error Paradigms (`F-ARCH-008`, Moderate):** The codebase incorporates non-idiomatic abstractions that reinvent Go language features: `pkg/when` reimplements `if/else` control flow using fluent closure chains (`when.When(...).Then(...)`) imported by 105 files, causing heap allocation overhead and obscured stack traces. Meanwhile, `pkg/functional` introduced a Rust-like `Result[T]` error monad that was abandoned after adoption in only 2 files in `cmd/zqk/scheduler/`.
9. **Orphaned Dead Code and Conflicting Codegen Defaults (`F-ARCH-009`, Low):** An orphaned directory `pkg/cli/command_builders/bldr_cli_cmd_v1` containing 211 generated command builder files sits completely unimported, while active commands use `pkg/cli/bldr_cli_cmd_v1` (671 files). The generator default in `pkg/zqkdev/generate_command_builders.go` still specifies the unused directory, maintaining a confusing dual source of truth.
10. **Lock Instrumentation Over-Abstraction and Allocation Overhead (`F-ARCH-010`, Low):** In `pkg/concurrency` and `pkg/storage/locknames`, every lock acquisition is wrapped in `concurrency.RunInLockWithLogger`, backed by a registry of 150+ lock name string constants. This allocates closures, captures timestamps, and defers monitoring functions even on high-frequency in-memory struct locks.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-ARCH-001` | Subprocess Fork Indirection Bypasses In-Process Component Architecture | high | E2 | MOD, MNT, ROB, REL | D-LOW |
| `F-ARCH-002` | Monolithic God Package Anti-Pattern in pkg/storage | high | E2 | MOD, MNT, RDB | D-LOW |
| `F-ARCH-003` | Asymmetric Persistence Guarantees and Interface Contract Divergence in ObjectStorageProvider | high | E2 | MOD, REL, TST | D-LOW |
| `F-ARCH-004` | Unindexed O(N) Disk & Deserialization Scan in FileObjectStorage.GetNeighbors | high | E2 | REL, ROB, MNT | D-LOW |
| `F-ARCH-005` | Untyped map[string]any Domain Architecture Driving 2,300+ Runtime Type Assertions | high | E2 | RDB, MNT, ROB, TST | D-LOW |
| `F-ARCH-006` | Severe Package Sprawl and Micro-Package Atomization (709 Packages) | moderate | E2 | MNT, MOD, RDB | D-LOW |
| `F-ARCH-007` | Fragmentation of Orchestration, Swarm, and Agent Execution Runtimes Across 15+ Packages | moderate | E2 | MOD, RDB, MNT | D-LOW |
| `F-ARCH-008` | Anti-Idiomatic Control-Flow DSLs and Abandoned Error-Handling Frameworks (pkg/when & pkg/functional) | moderate | E2 | RDB, MNT | D-LOW |
| `F-ARCH-009` | Orphaned Dead Code and Conflicting Codegen Defaults in pkg/cli/command_builders | low | E2 | MNT, MOD | D-LOW |
| `F-ARCH-010` | Lock Instrumentation Over-Abstraction and Allocation Overhead via 150+ String Constant Registry | low | E2 | MNT, ROB | D-LOW |

---

## 3. Structural & Architectural Hotspots

### 3.1 Storage Multi-Tier Persistence & Mutation Lifecycle
Refer to diagram `diagrams/D-ARCH-STORAGE-01.md` ("Multi-Tier Storage Architecture & Mutation Lifecycle") for structural illumination.

The storage engine exhibits an overloaded internal topology:
- **`FileObjectStorage` Persistence Tiers:** Rather than a simple file storage backend, `FileObjectStorage` manages four separate storage disciplines:
  1. *Content-Addressable Storage (CAS):* Deduplicated blob storage with asynchronous listing index flush queues (`ListingIndexWriteQueue`).
  2. *Stream Storage:* High-volume append-only event streams (`StreamStorageEnabledForKind`), which bypass draft planes and indexing.
  3. *Draft Plane Isolation:* Divergent directory paths for incomplete objects awaiting promotion (`UseObjectDraftPlane`).
  4. *Direct Process YAML:* Legacy direct filesystem reads/writes under `.zqk/process/{kind}/`.
- **Recursive Mutation Re-Entry:** Object creation (`f.Create`) uses recursive re-entry via `kernelcas.RunCreate`. It enters the mutation pipeline, executes `StageIngest`, `StageNormalize`, `StageDecide`, and then in `StageCommit` re-invokes `f.Create` with `kernelcas.WithCommit(ctx)`.
- **Interface Contract Incoherence:** Because `ObjectStorageProvider` exposes `Query()` alongside `GetRelated()`, `GetPath()`, and `GetNeighbors()`, callers assume uniform capabilities. In reality, `FileObjectStorage.Query` throws an error on `QueryTypeCypher`, while `GraphObjectStorage` lacks draft plane and stream concepts.

### 3.2 MCP Subprocess IPC Boundary
Refer to diagram `diagrams/D-ARCH-MCP-BRIDGE-01.md` ("MCP Subprocess Bridge vs In-Process Component Architecture") for sequence illumination.

- When an MCP tool call arrives (e.g. `object_get`), `pkg/mcp/cli_bridge.go` serializes tool arguments into command-line flags, resolves the `bin/zqk` executable, and spawns a separate operating system child process via `os/exec.CommandContext`.
- The parent MCP server captures child stdout and stderr buffers, applies a regular expression (`filterCommandOutput`) to strip out logging text, and unmarshals the remaining stdout string as JSON.
- This creates unnecessary CPU and fork latency (tens of milliseconds per tool call), breaks Go context propagation, consumes file descriptors, and necessitates explicit fork-bomb guards (`isUnsafeCLIBridgeBinary`) to keep test binaries from endlessly re-executing themselves.

---

## 4. Key Recommendations

1. **Establish In-Process Dispatch for MCP and Scheduler (`F-ARCH-001`):** Eliminate `os/exec` subprocess forks in `pkg/mcp/cli_bridge.go` and `pkg/scheduler/convergence_engine.go`. Expose a clean, in-process Go dispatcher (e.g. `cli.ExecuteInProcess(ctx, ...)` or a direct application service facade) that runs commands directly in memory.
2. **Modularize `pkg/storage` into Focused Packages (`F-ARCH-002`):** Carve out non-persistence systems: move TSDB providers to `pkg/metrics/tsdb`, workflow validation strategies to `pkg/validation`, and WaitGroup observation to `pkg/concurrency`. Keep `pkg/storage` focused on core storage providers and physical storage engines.
3. **Segregate `ObjectStorageProvider` by Interface Segregation Principle (`F-ARCH-003`):** Split `ObjectStorageProvider` into smaller, cohesive interfaces: `ObjectStore` (CRUD operations), `GraphTraversalStore` (`GetRelated`, `GetNeighbors`, `GetPath`), and `QueryableStore`. Avoid throwing runtime errors for unsupported interface methods.
4. **Implement Persistent Reverse-Reference Indexing (`F-ARCH-004`):** Replace the O(N) full-filesystem YAML parsing loop in `FileObjectStorage.GetNeighbors` with an incremental reverse-reference index that is updated synchronously or asynchronously on object mutations, conforming to the Magician Principle.
5. **Introduce Strongly-Typed Domain Entities (`F-ARCH-005`):** Transition the core domain from untyped `map[string]any` dictionaries to strongly-typed Go structs for first-class kernel entities (`Task`, `Criterion`, `PriorityPlan`, `BacklogItem`). Restrict untyped maps to wire/storage translation boundaries.
6. **Consolidate Generated Micro-Packages (`F-ARCH-006`):** Merge the 329 generated enum micro-packages in `pkg/specbuilder/bldr_enum_v1` into a single domain enum package (`pkg/domain/enums` or `pkg/objects/enums`). Consolidate single-file root packages into coherent subsystems per the 3-Tier Layering Model.
7. **Unify the Orchestration & Agent Ecosystem (`F-ARCH-007`):** Merge the 15+ fragmented orchestration, swarm, and agent packages into a clean two-tier model: `pkg/agent` for identity, seating, claims, and correspondence, and `pkg/swarm` for task dispatching and execution loops. Remove empty or skeletal stubs (`pkg/agentorch`).
8. **Deprecate Custom Control-Flow DSLs (`F-ARCH-008`):** Remove `pkg/when` in favor of standard Go `if/else` and `switch` statements. Decommission `pkg/functional` and standardize error handling on standard Go 1.13+ idioms (`fmt.Errorf("%w")`, `errors.Is`, `errors.As`).
9. **Prune Orphaned Code and Fix Codegen Defaults (`F-ARCH-009`):** Delete `pkg/cli/command_builders/bldr_cli_cmd_v1` (211 unimported files) and update the default output directory in `pkg/zqkdev/generate_command_builders.go` to `pkg/cli/bldr_cli_cmd_v1`.
10. **Simplify Mutex Synchronization (`F-ARCH-010`):** Eliminate `concurrency.RunInLockWithLogger` and its 150+ string constants on high-frequency in-memory structures, reserving telemetry wrapping only for slow I/O or WAL transactions.
