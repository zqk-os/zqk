# Storage Write Queue Observer Architecture & Inverted Dependency Governance

## Overview

This architecture document defines the governance rules for decoupled event dispatching and dependency inversion within the Content-Addressable Storage (CAS) write queue subsystem (`pkg/storage/cas`).

## Background & Problem Statement

Prior to this specification, `ListingIndexWriteQueue` relied on global atomic function pointers (`globalListingIndexBatchEventCallback`, `SetListingIndexBatchEventCallback`) to communicate batch lifecycle events across packages. The presentation layer (`cmd/zqk/system`) mutated these global variables at startup to bind coordinator event emission.

This pattern violated core architectural layering invariants:
1. **Presentation Layer Contamination**: Presentation components mutated low-level storage internal hooks.
2. **Multi-Instance Interference**: Concurrent test cases or isolated storage instances shared the same mutable global pointer, causing event leakage and test race conditions.
3. **Inverted Dependency Injection**: Hook mutation bypassed compile-time interface contracts.

## Architectural Solution: Observer Pattern

The storage write queue subsystem implements the Observer pattern at the instance level:

```
┌─────────────────────────────────┐
│     ListingIndexWriteQueue      │
│  (Manages per-kind write queues)│
└────────────────┬────────────────┘
                 │
                 ▼ RegisterBatchEventListener(listener)
┌──────────────────────────────────────────────────┐
│         ListingIndexBatchEventListener           │
│                    <<interface>>                 │
│  + OnListingIndexBatchEvent(...)                 │
└──────────────────────────────────────────────────┘
                 ▲
                 │ implements
┌────────────────┴──────────────────┐
│   Coordinator/Logging Observers   │
│   (e.g. ListingIndexBatchEventFunc)
└───────────────────────────────────┘
```

### Invariants

1. **Instance-Scoped Listeners**: `ListingIndexWriteQueue` maintains an internal slice of registered listeners protected by a read-write mutex.
2. **Zero Global Mutation**: Presentation and coordinator layers wire observers onto the queue instance directly rather than mutating package-level function pointers.
3. **Concurrency Safety**: Workers dispatch events asynchronously to instance-registered listeners without holding internal queue locks, ensuring non-blocking event flow and zero cross-queue leakage.
