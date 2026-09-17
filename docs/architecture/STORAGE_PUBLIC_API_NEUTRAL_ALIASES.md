# Storage public API: neutral aliases (BLI-177484)

**Last Verified:** 2026-08-31


**Backlog:** `[REDACTED-ID]` — optional CAS-prefixed export cleanup.

**Purpose:** `pkg/storage` historically exposed **CAS**-prefixed names for **content-addressed file object** storage (hash-keyed YAML under the project tree). That wording collides mentally with unrelated **stream** / **cell** work. Neutral aliases give new call sites mechanism-agnostic names; **CAS\*** symbols remain for compatibility.

## Aliases (same runtime behavior)

| Legacy | Neutral |
|--------|---------|
| `CASMetrics` | `ObjectStorageMetrics` (type alias) |
| `GetCASMetrics` | `GetObjectStorageMetrics` |
| `ResetCASMetrics` | `ResetObjectStorageMetrics` |
| `CASMetricsCollector` | `ObjectStorageMetricsCollector` (type alias) |
| `NewCASMetricsCollector` | `NewObjectStorageMetricsCollector` |
| `CASOrphanCleanupQueue` | `OrphanCleanupQueue` (type alias) |
| `GetGlobalCASOrphanCleanupQueue` | `GetGlobalOrphanCleanupQueue` |
| `CASMetricsAsyncCollector` | `ObjectStorageMetricsAsyncCollector` (type alias) |
| `GetCASMetricsAsyncCollector` | `GetObjectStorageMetricsAsyncCollector` |
| `NewCASMetricsAsyncCollector` | `NewObjectStorageMetricsAsyncCollector` |

Internal implementations and tests may continue to use **CAS\*** names until migrated opportunistically.

**Boundary:** See [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) (Compatibility with data cells and streams): file CAS storage naming is **orthogonal** to stream summaries and data cells.
