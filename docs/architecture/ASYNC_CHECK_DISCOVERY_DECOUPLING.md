# Asynchronous Object Discovery & Domain Decoupling Specification

## Overview

This architecture document defines the structural boundary and operational contract for asynchronous object discovery across the ZQK Knowledge Kernel, resolving Technical Debt `TDE-CEF-F-ARCH-002` under Priority Plan `PRI-CEF-ARCH-DECOUPLING-P33` and Backlog Item `BLI-CEF-ARCH-DECOUPLING-P33`.

## Problem Statement

Historically, `cmd/zqk/system/async_check.go` was responsible for both presentation concerns (Cobra CLI flag parsing, interactive terminal progress rendering, output formatting) and domain-level object discovery logic. Specifically, over 200 lines of low-level filesystem traversal (`filepath.Walk`), Content-Addressable Storage (CAS) index lookups, AppleDouble file suppression, and bounded goroutine scanning resided directly within the command implementation.

This coupling violated core architectural layering:
1. **Presentation / Domain Boundary Leak**: Reusable filesystem discovery and CAS cache index evaluation could not be leveraged by other daemons or background workers without importing the CLI command layer.
2. **Monolithic Complexity**: `cmd/zqk/system/async_check.go` swelled beyond 1,200 lines, violating single-responsibility principles and elevating cognitive overhead for maintainers.
3. **Redundant Logic & Testing Gaps**: Parallel implementations of object scanning and extraction existed across CLI and domain packages, risking drift and inconsistent cancellation behaviors.

## Architectural Design & Subsystem Boundary

### 1. The Domain Engine: `pkg/systemcheck/asynccheck`

All filesystem scanning, CAS index acceleration, and object ID extraction logic are consolidated within `pkg/systemcheck/asynccheck`:

- **`ScanObjectFilesWithContext(ctx context.Context, dir, kind string, logger logging.Logger, storageProvider storage.ObjectStorageProvider) ([]ScannedFile, error)`**:
  - **CAS Fast Path**: Checks for existence of the `.{kind}.index` file. If present and populated, resolves hashes directly via `storage.UnwrapToFileObjectStorage` or `storage.NewContentAddressableStorage`, avoiding disk traversal of tens of thousands of objects.
  - **Date-Bucketed Layout Detection**: Automatically resolves YYYY-MM or YYYY-MM-DD bucket directories for partition-aware kinds.
  - **Filesystem Traversal Fallback**: When CAS indices are absent, executes a bounded `filepath.Walk` wrapped in labeled goroutine budgets (`labelScanObjectFilesWalk`, `labelScanObjectFilesColl`).
  - **AppleDouble Suppression**: Automatically skips macOS metadata dotfiles via `appledouble.SkipPathInTreeWalk`.
  - **Cancellation & Timeout Resilience**: Bounded contexts enforce deterministic termination without leaking worker goroutines.

- **`ExtractObjectIDFromFileWithContext(ctx context.Context, filePath, kind string, logger logging.Logger) string`**:
  - Distinguishes CAS hash files from human-readable object files.
  - Uses bounded concurrency semaphores to protect against file descriptor exhaustion during disk stress.

### 2. The Presentation Layer: `cmd/zqk/system`

The CLI command `cmd/zqk/system/async_check.go` now acts as a thin orchestrator:
- Parses CLI flags and initializes progress reporting structures.
- Delegates scanning directly to `asynccheck.ScanObjectFilesWithContext`.
- Aliases `scannedFile` to `asynccheck.ScannedFile` to preserve internal command signatures without code duplication.

## Verification & Traceability

1. **Unit Test Suite (`pkg/systemcheck/asynccheck/discovery_test.go`)**:
   - `TestScanObjectFilesWithContext_Walk`: Verifies accurate object discovery across standard YAML extensions, skipping non-YAML files.
   - `TestScanObjectFilesWithContext_CancelledContext`: Asserts immediate termination and clean error propagation when the context is cancelled.
   - `TestExtractObjectIDFromFile`: Validates both CAS content inspection and filename-based ID extraction.
   - `TestFilterFilesByIDs` and `TestDeduplicateFilesByObjectID`: Ensures correct object partitioning and deduplication.

2. **Integration Verification (`cmd/zqk/system`)**:
   - `TestDiscoverFromCache_StreamAndFilter` and `TestDiscoveryEarlyCompletion_*` confirm end-to-end compatibility with async check pipelines.
