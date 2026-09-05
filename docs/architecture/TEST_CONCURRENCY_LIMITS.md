# Package Test Concurrency Limits & Justification

**Last Verified:** 2026-08-31


## Overview
Package test concurrency is governed dynamically and on-disk via `package_concurrency_limits.json` under `.zqk/test-bundles/`.

## Measured Package Limits
- **`pkg/storage`**: Limit = 4. Storage operations perform disk I/O, CAS operations, and index lock synchronization. 4 parallel workers maximize throughput while avoiding I/O starvation.
- **`pkg/mcp`**: Limit = 8. Protocol handlers and subprocess tools are I/O-bound with minimal disk contention.
- **`pkg/scheduler`**: Limit = 2. Daemon loops, trigger queue consumption, and job dispatch locks require serialization to prevent race conditions during state inspection.
- **Default (Unspecified Packages)**: Limit = 4 (bounded at 32 max).

## Verification
- Satisfies `BLI-CEF-R13-PARALLEL-SAFE-001` and `CRIT-CEF-R13-PARALLEL-SAFE-001`.
- Tests run under `internal/testpackageconcurrency` ensure limits merge correctly and respect per-key constraints.
