# Project-Scoped Reverse Reference Index and Thread-Safe Caches

## Overview

This architectural specification documents the transition away from process-wide global singleton caches toward thread-safe, project-scoped instance registries for reverse reference indexing and storage write queues (resolving `TDE-CEF-F-ARCH-003` and fulfilling `REQ-GOD-PACKAGES-SCOPED-INDEX`).

## Problem Statement

Historically, the ZQK knowledge kernel relied on global singleton accessors for caching and indexing:
1. **Destructive Cross-Project Cache Invalidation**: `storage.GetGlobalReverseReferenceIndex()` maintained a single process-wide index. When `BuildFromScan` was executed on one workspace, it ran an un-scoped `r.Clear()`, wiping cached dependency edges across all concurrent projects.
2. **Atomic Project Root Clobbering**: `cas.GetGlobalListingIndexWriteQueue()` maintained a single global `projectRoot` atomic variable. In multi-tenant, embedded, or concurrent test contexts, successive writes across projects clobbered each other's project paths.
3. **Hidden Tight Coupling**: Modules implicitly depended on shared mutable global state rather than explicit project context or dependency injection.

## Architecture & Implementation Patterns

### 1. Workspace-Scoped Reverse Reference Index (`pkg/storage`)

`storage.ReverseReferenceIndex` is enhanced with explicit project binding and a thread-safe registry:
- **Project-Scoped Registry**: `storage.GetReverseReferenceIndexForProject(projectRoot string) *ReverseReferenceIndex` provides isolated instances keyed by normalized project root paths using double-checked locking (`sync.RWMutex`).
- **Explicit Project Binding**: `ReverseReferenceIndex.ProjectRoot() string` exposes the associated workspace path.
- **Isolated Cache Files**: Cache paths are strictly anchored under `<projectRoot>/.zqk/cache/reverse-reference-index.json`. Calling `Clear()` or `BuildFromScan()` on one project index only modifies its own map entries and cache files without affecting peer project workspaces.
- **Deprecation of Global Accessors**: `GetGlobalReverseReferenceIndex()` and `NewReverseReferenceIndex()` are marked deprecated with migration paths pointing to `GetReverseReferenceIndexForProject(projectRoot)`.

### 2. Multi-Tenant Concurrent Isolation

The scoped registry guarantees:
- **Thread Safety**: Concurrent readers (`RLock`) and writers (`Lock`) operate safely without race conditions.
- **Isolated State Mutations**: Operations such as `AddReference`, `RemoveReference`, `RemoveObject`, and `Clear` are strictly contained within the project's memory boundary.

### 3. Verification & Guardrails

- `TestReverseReferenceIndex_ProjectScopedIsolation`: Validates that distinct project roots yield independent index instances, and that mutating or clearing index A leaves index B intact.
- `TestReverseReferenceIndex_MultiTenantConcurrentAccess`: Validates high-concurrency read/write operations across multiple parallel project roots without deadlocks, data races, or state pollution.

## Invariants

1. **Strict Project Isolation**: Cache mutations and index resets in one workspace never affect another workspace's index state.
2. **Explicit Workspace Context**: Storage and caching abstractions must accept and normalize workspace roots (`filepath.Clean`).
3. **Deprecation Path**: All newly authored components must use project-scoped constructors (`GetReverseReferenceIndexForProject`) rather than legacy process-wide singletons.
