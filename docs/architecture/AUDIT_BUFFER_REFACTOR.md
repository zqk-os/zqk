# Audit Event Buffer Refactor: Singleton to Registry Pattern

**Last Verified:** 2026-08-31


## Overview
To address cross-test resource contention and intermittent deadlocks identified in integration tests, the global audit event buffer singleton has been deprecated in favor of an instance-based registry pattern.

## Changes
1.  **Registry Pattern:** Introduced `AuditEventBufferRegistry` in `pkg/storage/audit_event_buffer_initialization.go` to manage buffer instances mapped by project root.
2.  **Instance Lifecycle:** Buffers are now explicitly created and lifecycle-managed within the scope of their owner (e.g., test case, router instance) rather than via global lazy-init.
3.  **Removal of Global State:** `GetGlobalAuditEventBuffer` and `InitializeGlobalBufferWithConfig` have been deprecated; components now use the `Registry` to obtain the buffer instance scoped to their execution context.

## Architectural Impact
*   **Reduced Contention:** Eliminates cross-test lock contention on the `bufferMu` mutex.
*   **Resource Management:** Fixes potential leaks where background goroutines (`periodicFlush`) outlived test environments.
*   **API Update:** `CreateAuditEventWithBuilder` and `StorageAuditRouter` signatures updated to require context-aware buffer retrieval.
