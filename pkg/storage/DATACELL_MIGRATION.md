# DataCell Migration Plan for pkg/storage

## Objective
Migrate legacy FileObjectStorage tests and implementations to the DataCell/Stream backend pattern to ensure concurrency stability and architectural alignment.

## Module Mapping (Target for Migration)
| Legacy Component | Target DataCell/Stream Component | Status |
| :--- | :--- | :--- |
| AuditEventBuffer | AuditStreamProcessor | Planned |
| CASOrphanCleanupQueue | CASOrphanStreamCoordinator | Planned |
| ObjectWriteBehindWorker | StreamWriteBuffer | Planned |
| FileObjectStorage (Registry) | DataCellRegistry | In-Progress |

## Immediate Stabilization (Strategy B)
Stabilize legacy tests by implementing deterministic shutdown:
1. Register t.Cleanup hooks for all storage workers.
2. Remove global singleton usage in tests.
