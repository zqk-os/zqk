# Package Consolidation Governance and Codebase Complexity Budget

This document establishes the official architectural governance policies, package consolidation rules, and complexity limits for the ZQK kernel repository, resolving `F-ARCH-006` and `F-MNT-MONOLITH-PACKAGE-OUTLIERS`.

## 1. 3-Tier Layering Model

All Go packages within `pkg/` and `cmd/` must conform to the 3-Tier Layering Model:

1. **Tier 1: Foundation Primitives** (`pkg/concurrency`, `pkg/paths`, `pkg/logging`, `pkg/objects`, `pkg/brand`):
   - Zero internal dependencies on higher tiers.
   - Reusable across any Go subsystem.
   - Fast compile times, strictly isolated.

2. **Tier 2: Core Domain & Storage Engine** (`pkg/storage`, `pkg/scheduler`, `pkg/orchestration`, `pkg/swarm`):
   - Implements domain workflows, distributed state coordination, persistence engines, and lifecycle state machines.
   - May depend on Tier 1 primitives, but never on Tier 3 CLI commands or applications.

3. **Tier 3: Application & Delivery Surface** (`cmd/zqk`, `pkg/mcp`, `pkg/cli`):
   - Exposes user-facing commands, HTTP/JSON-RPC protocols, and IDE integrations.
   - Consumes Tier 1 and Tier 2 engines.

## 2. Anti-Atomization & Package Sprawl Governance (`F-ARCH-006`)

- **Prohibition on Micro-Package Generation**: Code generation pipelines (such as `pkg/specbuilder/bldr_enum_v1`) must not emit single-file packages consisting solely of a single type or enum definition.
- **Grouping Rule**: Domain enums, constants, and builders must be grouped logically by subsystem or domain boundary (e.g., `pkg/specbuilder/enums`).
- **Zero-File Package Elimination**: Automated CI hygiene scans reject packages containing 0 Go files or uncompilable test-only stubs.

## 3. Complexity Budget & File Size Limits (`F-MNT-MONOLITH-PACKAGE-OUTLIERS`)

To prevent monolithic god files and sprawling modules:

- **Single-File Line Budget**: Individual Go source files should not exceed **1,500 LOC**. Files exceeding **2,500 LOC** trigger strict CI failure.
- **Package File Budget**: A single package directory should not exceed **500 source files** without modular subdirectory partitioning.
- **Refactoring Strategy**: Monolithic files (e.g. `cmd/zqk/ui/model.go`, `cmd/zqk/test/dashboard.go`) must be partitioned into functional components (e.g. state, handlers, rendering).
