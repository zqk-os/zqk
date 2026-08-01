# Fluent structured logging (POL-CODE-007)

**Purpose:** One consistent pattern for attaching multiple structured fields to a single log line using pooled builders, instead of variadic `Logger.Info(msg, logging.String(...), ...)`.

**Implementation:** `pkg/logging/fluent_builder.go` (`FluentRoot`, `FluentEntry`, `Log()`).

---

## Which entrypoint to use

| Context | Use | Notes |
|--------|-----|--------|
| Most packages | `logging.Fluent(logger)` | `logger` from profile or embedded context. |
| `pkg/scheduler` handlers | `scheduler.SLog(logger)` | Same pooled type as `Fluent`; greppable scheduler tag. |
| `pkg/storage` runtime | `storage.StorageLog(logger)` | Same pooled type as `Fluent`; greppable storage tag. |
| `*logging.EventLogger` | `logging.FluentEvent(el)` | Avoids repeating `el.Logger()` at call sites. |

Interfaces that intentionally take `...logging.Field` (**`CRUDLogger`**, **`DryRunLogger`**, **`ProgressLogger`**, etc.) keep that API at boundaries. Implementations that wrap a **`Logger`** or **`EventLogger`** should delegate into **`Fluent`** / **`FluentEvent`** when emitting multi-field messages.

Single-field logs may stay variadic where clarity is unchanged; prefer fluent when adding fields or touching the line anyway.

---

## Shape

```go
logging.Fluent(logger).Warn("operation failed").
    ProjectRoot(projectRoot).
    WithError(err).
    Log()
```

Prefer keyed helpers on **`FluentEntry`** (`Kind`, `RuleName`, `RuleConfigName`, `RulesDir`, `RuleCount`, `ValidationRowField`, `ValidationRowMessage`, `LoaderPhase`, `KindsCount`, `PanicSummary`, `ObjectID`, `Path`, `JobID`, `MessageID`, `WorkerID`, `GoroutineID`, `ActiveGoroutines`, `SemaphoreCapacity`, `ActiveWorkersRemainingAtTimeout`, `WorkerStateLabel`, `DiagnosticDetail`, `WaitTime`, `TotalDrained`, `ValidationDrainCount`, `ValidationProgressStatus`, `URL`, `Protocol`, `KeyID`, `OutputDir`, `FilePath`, `Profile`, `ProfileName`, `Total`, `TotalTasks`, `ProgressCount`, `CachedCount`, `TotalAccounted`, `PercentComplete`, `QueueEmptyDuration`, `CacheHits`, `EntryCount`, `QueueSize`, `QueueName`, `MaxWorkers`, `ActiveWorkers`, `Completed`, `Enqueued`, `BuildTime`, `ElapsedString`, `ExecPath`, `Recovered`, `FailedOps`, `ProcessedCount`, `ExpectedURLCount`, `VerifiedURLCount`, `FailedURLCount`, `Success`, `Enabled`, `Timeout`, `IdleDuration`, `RetryAttempt`, `RetryMaxAttempts`, `RetryDelay`, `BackoffStrategy`, `MaxBackoffMs`, `DefaultTimeoutMs`, `ValidationErrorCount`, `ErrorCount`, `WorkerStopTimeout`, `CacheSaveTimeout`, `HTTPStatusCode`, `StdoutSnippet`, `AuthType`, `DiagNote`, `FilterExpr`, `NotifyType`, `Capped`, `KindsSummary`, `EmitComponent`, `TraceMarker`, `RPCMethod`, `RequestID`, `RPCMessageType`, `ParamsText`, `ParamsData`, `RPCResult`, `TraceFile`, `MCPLogLevel`, `Reason`, `Op`, `OpDetail`, `StorageProfileName`, …) when they match your payload so wire keys stay consistent across packages.

Within a single **`Log()` chain**, avoid interleaving generic **`.String` / `.Int` / `.Bool`(key, …)** with those named helpers—use named methods for every field that has one (wire keys unchanged), or use **only** generic **`.String`/`.Int`/`.Bool`** for an entry that carries no semantic helpers yet. Prefer adding a **`FluentEntry`** accessor over ad-hoc keys when the same payload repeats across commands.

---

## References

- Policy: `docs/architecture/policies/POL-CODE-007.yaml` (conceptual); enforcement checklist in **`docs/architecture/README.md`**.
- Log files on disk (different topic): **`docs/architecture/LOG_NAMING_CONVENTIONS.md`**.
