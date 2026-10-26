# Polyglot Test Discovery, Engine Registry & Cache Subsystem Operational Architecture Guide

## 1. Architectural Overview

The **Polyglot Test Discovery, Engine Registry & Cache Subsystem** (`pkg/testdiscovery`) provides high-performance, deterministic static test extraction across multi-language codebases without code execution or subprocess overhead.

In complex polyglot repositories containing Go, Python, TypeScript, and Rust components, naive dynamic test discovery methods (such as invoking external runtimes `pytest --collect-only`, `cargo test --list`, or spinning up Node runtimes) introduce severe security vulnerabilities, heavy subprocess latency, and flakiness from unbuilt dependencies or uninitialized runtime states.

Under the core architecture invariants (POL-DEFAULT-142277d3):
- **Zero-Subprocess Static Analysis:** Test discovery occurs entirely in-memory using AST parsing (`go/ast`) and compiled regular expression tokenization for Python, TypeScript, and Rust.
- **Incremental SHA-256 Memoization:** Discovery results are cached with dual-coordinate fast-path checking (mtime fast path + SHA-256 content hash fallback), eliminating redundant parsing on untouched source files.
- **Unified Kernel Object Projection:** Discovered targets across all supported languages are projected into standard `test_case` objects compatible with the ZQK Knowledge Kernel relational graph and test verification runners.
- **Fail-Closed Jail Containment:** Strict path lexical normalization and symlink evaluation prevent directory traversal escapes outside the canonical project boundary.

---

## 2. Component Topology & Architecture

The subsystem consists of five primary architectural primitives:

```mermaid
flowchart TD
    subgraph ClientPlane["Callers & Tooling"]
        CLI["zqk test discover"]
        Runner["Test Execution Harness"]
        Agent["Autonomous Worker Swarm"]
    end

    subgraph TestDiscoverySubsystem["pkg/testdiscovery Subsystem"]
        Engine["Engine (Discovery Coordinator)"]
        Options["DiscoveryOptions"]
        Cache["DiscoveryCache (JSON Storage)"]

        subgraph DiscovererRegistry["Discoverer Registry (Discoverer Interface)"]
            GoDisc["GoDiscoverer (AST Parser)"]
            PyDisc["PythonDiscoverer (Token Scanner)"]
            TSDisc["TypeScriptDiscoverer (Regex Scanner)"]
            CargoDisc["CargoDiscoverer (Rust Scanner)"]
            CustomDisc["Custom Discoverers (Plugin / Mock)"]
        end

        WorkerPool["Goroutine Worker Pool (goroutinelabels)"]
        Aggregator["Target Aggregator & TestCase Generator"]
    end

    ClientPlane -->|Discover(ctx, opts)| Engine
    Engine --> Options
    Engine --> Cache
    Engine --> WorkerPool
    WorkerPool --> DiscovererRegistry
    WorkerPool --> Aggregator
    Aggregator -->|[]DiscoveredTarget| ClientPlane
```

### Subsystem Components

1. **`Engine` (`engine.go`):** The central discovery coordinator. Orchestrates candidate file collection, path containment verification, worker pool distribution via `goroutinelabels.NewGoroutine`, and result synthesis.
2. **`DiscoveryOptions` (`types.go`):** Declarative configuration struct specifying `ProjectRoot`, target `Paths`, `Languages` filters, `ExcludePaths`, `Workers` concurrency ceilings, and `Incremental` caching flags.
3. **`DiscoveryCache` (`cache.go`):** Thread-safe (`sync.RWMutex`) memoization store holding file modification timestamps, SHA-256 content hashes, and extracted targets with atomic serialized JSON persistence.
4. **`Discoverer` Interface (`types.go`):** Pluggable language abstraction defining `Language() string`, `CanHandle(relPath string) bool`, and `Discover(ctx, projectRoot, relPath, content) ([]DiscoveredTarget, error)`.
5. **Language Discoverers:**
   - **`GoDiscoverer` (`go_discoverer.go`):** Go standard AST parsing (`go/parser`, `go/token`), extracting `Test*`, `Benchmark*`, and `Example*` declarations, build tags, and doc comment metadata (`criteria`, `requirement`, `scenario`).
   - **`PythonDiscoverer` (`python_discoverer.go`):** Static line scanning detecting `def test_*`, `class Test*`, `@pytest.mark.*` decorators, and comment metadata.
   - **`TypeScriptDiscoverer` (`ts_discoverer.go`):** Static scanning for `.test.ts`, `.spec.ts`, `.test.tsx`, `.spec.tsx`, `.test.js`, and `.spec.js`, extracting `describe`, `suite`, `it`, and `test` blocks.
   - **`CargoDiscoverer` (`cargo_discoverer.go`):** Rust source scanning detecting `#[test]`, `#[tokio::test]`, `#[ignore]`, `mod tests`, and function signatures (`fn`, `pub fn`, `async fn`).

---

## 3. Polyglot Test Discovery & Caching Lifecycle

The discovery lifecycle combines directory walk filtering, worker distribution, and incremental cache lookups:

```mermaid
sequenceDiagram
    autonumber
    actor Caller as Client (CLI / Harness)
    participant Engine as Engine
    participant Walker as Candidate Walker
    participant Cache as DiscoveryCache
    participant Workers as Worker Pool
    participant Discoverer as Language Discoverer

    Caller->>Engine: Discover(ctx, opts)
    Engine->>Walker: collectCandidateFiles(cleanRoot, canonicalRoot, opts)
    Note over Walker: Jail containment check & symlink evaluation
    Walker-->>Engine: []fileCandidate (relPath, absPath, info)

    alt opts.Incremental == true
        Engine->>Cache: LoadCache(cachePath)
    end

    loop For each file candidate in parallel
        Engine->>Workers: Dispatch candidate to worker channel
        Workers->>Cache: Get(relPath, modTime, content)
        alt Cache Hit (mtime or SHA-256 match)
            Cache-->>Workers: Cached []DiscoveredTarget
        else Cache Miss
            Workers->>Discoverer: CanHandle(relPath)
            Discoverer-->>Workers: true
            Workers->>Discoverer: Discover(ctx, root, relPath, content)
            Discoverer-->>Workers: []DiscoveredTarget
            Workers->>Cache: Set(relPath, modTime, content, targets)
        end
    end

    alt opts.Incremental == true
        Engine->>Cache: Save()
    end
    Engine-->>Caller: []DiscoveredTarget
```

### Cache Resolution Logic
1. **Index Lookup:** Evaluates whether `relPath` exists in the cache map.
2. **mtime Fast Path:** If `entry.ModTimeUnix == modTime.Unix()`, returns cached targets immediately without computing content hashes or reading file buffers.
3. **SHA-256 Fallback:** If modification times differ (such as after `git checkout` or touch operations) and file content is provided, computes SHA-256 hash. If `hStr == entry.Hash`, updates mtime and returns cached targets.
4. **Miss & Re-parse:** If hashes differ or key is unknown, delegates to the matching `Discoverer`, updating cache entries with newly parsed targets.

---

## 4. Engine Registry & Lifecycle State Machine

The Engine transition lifecycle guarantees thread-safe initialization, discovery coordination, and clean cache eviction:

```mermaid
stateDiagram-v2
    [*] --> Initialized: NewEngine()
    
    Initialized --> Configured: RegisterDiscoverer(custom)
    Initialized --> Scanning: Discover(ctx, opts)
    Configured --> Scanning: Discover(ctx, opts)

    state Scanning {
        [*] --> WalkFiles
        WalkFiles --> FilterCandidates: Symlink & Ignore Gates
        FilterCandidates --> Dispatched: Populate Worker Channel
    }

    Scanning --> Parsing: Workers Active
    state Parsing {
        [*] --> CacheQuery
        CacheQuery --> FastReturn: Cache Hit
        CacheQuery --> StaticParse: Cache Miss
        StaticParse --> CacheStore: Update Dirty Entry
        FastReturn --> MergedResults
        CacheStore --> MergedResults
    }

    Parsing --> Serializing: Workers Complete
    Serializing --> Flushed: cache.Save()
    Flushed --> [*]: Return []DiscoveredTarget
```

---

## 5. Security & Isolation Boundaries

1. **Jail Containment (Zero Symlink Traversal):**
   `collectCandidateFiles` strictly resolves symlinks on the project root (`canonicalRoot`) and on each candidate path (`canonicalPath`). Any directory or file whose canonical target escapes `canonicalRoot` is rejected or skipped via `filepath.SkipDir`.
2. **Zero Code Execution:**
   No child processes (`os/exec`) or external interpreters (`python`, `node`, `cargo`) are invoked. Discovery is purely in-process and static, guaranteeing resilience against arbitrary code execution vulnerabilities in untrusted repositories.
3. **Automated Directory Pruning:**
   Hardcoded exclusion lists automatically prune VCS directories (`.git`), dependency folders (`node_modules`, `vendor`), build outputs (`dist`, `build`, `target`), and internal runtime trees (`.zqk`), preventing CPU exhaustion and memory bloat.

---

## 6. Concurrency & Thread-Safety Guarantees

- **Concurrent Cache Invariants:** `DiscoveryCache` guards internal map reads with `sync.RWMutex.RLock` and updates with `sync.RWMutex.Lock`. Multiple worker goroutines can safely read and populate cache entries simultaneously.
- **Worker Pool Supervision:** Worker goroutines are dispatched under `goroutinelabels.NewGoroutine("testdiscovery.worker", "discover test targets in files")`, guaranteeing structured goroutine naming and leak prevention in diagnostic telemetry.
- **Context-Bound Lifetime:** Worker loops monitor `select { case <-ctx.Done(): ... }` to honor caller-directed timeouts and cancellations immediately.

---

## 7. Verification & Three-Fold Proof

The subsystem adheres strictly to the Three-Fold Proof model:

| Dimension | Proof Mechanism | Verification Gate |
| :--- | :--- | :--- |
| **Static Floor Invariant** | Statement test coverage must reach $\ge 85.0\%$ across all subsystem routines. | `go test -cover ./pkg/testdiscovery` (Achieved: **95.7%**) |
| **Operational Dynamic Proof** | Multi-language test discovery across Go, Python, TypeScript, and Rust, SHA-256 incremental cache hits/misses, custom discoverer registration, and concurrent worker thread safety. | `pkg/testdiscovery/testdiscovery_coverage_elevation_cycle225_test.go` |
| **Negative Adversarial Boundary** | Corrupt JSON cache tolerance, symlink directory traversal rejection, syntax error isolation, invalid worker configurations, and graceful context cancellation. | `go test -race -timeout 30s ./pkg/testdiscovery` (0 failures, 0 races) |

---

## 8. Open-Core Compliance & Invariants

1. **Deterministic Execution:** Test parsing produces identical, byte-reproducible `DiscoveredTarget` outputs across all operating platforms (Linux, macOS, Windows).
2. **Zero Swallowed Errors:** All error branches in discovery and cache operations are explicitly captured, logged, or propagated.
3. **Structured Goroutine Lifecycle:** Concurrency operations utilize `goroutinelabels` for leak-free, traceable background execution.
4. **Identifier Sanitization:** The public specification and implementation remain strictly decoupled from project-specific or nanosecond kernel identifiers.
