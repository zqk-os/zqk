# L-OBSERVABILITY Narrative: Observability & Operability Evaluation

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-OBSERVABILITY`  
**Density Class:** `D-MED` (Top-N Budget: 10; Emitted: 7)  
**Primary Axes:** `OBS` (Observability), `OPS` (Operability), `ROB` (Robustness)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files)

---

## 1. Executive Assessment

Evaluation of the observability and operability posture across the ZQK candidate tree reveals a dichotomy between robust internal protocol hygiene and disconnected external visibility pipelines:

1. **Exemplary Protocol Logging Discipline (`F-OBS-STRUCTURED-LOGGING-DISCIPLINE`, Info):** The structured logging architecture in `pkg/logging` exhibits outstanding engineering discipline. Loggers strictly separate stdout from diagnostic logging streams, intelligently routing debug/error output to stderr and completely suppressing stdout writes when serving the MCP JSON-RPC protocol. Codebase audits confirm that `fmt.Print` and `log.Print` calls are strictly banned from production command dispatch paths, eliminating stdout stream pollution.
2. **False-Green Primary Status Reporting (`F-OBS-FALSE-GREEN-STATUS`, High):** In contrast to internal logging discipline, the human-facing command `zqk system status` emits `Status: ✅ Initialized` whenever the `.zqk` directory exists, completely omitting scheduler and job health in its default table rendering. Even when the background scheduler daemon encounters repeated task execution failures (e.g. 5 consecutive exit status 1 failures on `SCH-autofix-run`), `zqk system status` reports a pristine green verdict.
3. **MCP Fast-Path Visibility Masking (`F-OBS-MCP-FAST-PATH-MASKING`, High):** When commands are invoked under `--context mcp`, `getSystemHealthDataMCPFast` bypasses full kernel validation checks (`zqk system check`) to return in sub-second time. However, this stub returns `status: "unknown"` and skips CAS index and object integrity verification entirely, blinding autonomous agents to underlying repository corruption.
4. **Disconnected Telemetry & Tracing Pipelines (`F-OBS-TELEMETRY-SINK-DISCONNECTION`, `F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED`, High & Moderate):** While `pkg/telemetry` defines standard W3C `TraceContext` parsers and a telemetry `Hook` interface, production integration is missing. `GlobalManager` contains zero registered production hooks, `Daemon.Aggregate` is a no-op placeholder, and trace IDs are not propagated through storage contexts or daemon RPCs.
5. **Unexported Latency Histograms (`F-OBS-UNEXPOSED-HISTOGRAM-METRICS`, Moderate):** IPC latencies, ghost drift MTTR, and skill execution times are tracked in Prometheus-style exponential histograms (`PrometheusHistogram`), but `DefaultTracker` provides no export endpoints, CLI query commands, or metrics scrapers.
6. **Absence of Operator Runbooks (`F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE`, Moderate):** The repository provides developer specifications and CLI tutorials, but lacks on-call runbooks or recovery symptom trees for troubleshooting quarantine accumulation, lock contention deadlocks, or daemon failures.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-OBS-FALSE-GREEN-STATUS` | zqk system status table reports false-green status despite failing background scheduler jobs | high | E2 | OBS, OPS, ROB | D-MED |
| `F-OBS-MCP-FAST-PATH-MASKING` | System health MCP fast path bypasses validation checks and masks kernel degradation | high | E2 | OBS, ROB, OPS | D-MED |
| `F-OBS-TELEMETRY-SINK-DISCONNECTION` | Telemetry daemon aggregation and hook manager sinks are disconnected no-ops | high | E2 | OBS, CMP, MOD | D-MED |
| `F-OBS-UNEXPOSED-HISTOGRAM-METRICS` | Prometheus-style telemetry histograms lack export endpoints and CLI inspection views | moderate | E2 | OBS, OPS | D-MED |
| `F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED` | W3C TraceContext implementation lacks correlation propagation across IPC, CLI, and async queues | moderate | E2 | OBS, MOD, ROB | D-MED |
| `F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE` | Absence of structured runbooks, on-call alerts, and incident triage playbooks | moderate | E2 | OPS, OBS, RCV | D-MED |
| `F-OBS-STRUCTURED-LOGGING-DISCIPLINE` | Exemplary structured logging architecture with strict protocol protection against stdout pollution | info | E2 | OBS, ROB, RDB | D-MED |

---

## 3. Structural & Architectural Hotspots

### 3.1 Observability Pipelines & Diagnostic Data Flows
Refer to diagram `diagrams/D-OBS-PIPELINE-01.md` ("ZQK Observability Architecture, Diagnostic Flows, and Visibility Gaps") for the complete structural map.

The observability architecture spans three key layers:
1. **Logging & Protocol Boundary (`pkg/logging`):** Enforces structured semantic fields and protects JSON-RPC stdio channels via `selectWriterForLogLevel`. This layer is mature, reliable, and strictly adhered to across subsystems.
2. **Status & Diagnostic Consolidation (`cmd/zqk/system`):** Consolidates system health via `system status`, `system check`, and `system health-data`. The major failure mode is the decoupling between daemon issues and table status badges, resulting in false-green reports (`F-OBS-FALSE-GREEN-STATUS`).
3. **Telemetry & Distributed Tracing (`pkg/telemetry`):** Represents an incomplete abstraction plane. Hooks and aggregation loops are stubs, leaving span measurements and latency distributions unexported (`F-OBS-TELEMETRY-SINK-DISCONNECTION`, `F-OBS-UNEXPOSED-HISTOGRAM-METRICS`).

---

## 4. Key Recommendations

1. **Unify Status Badges with Subsystem Health (`F-OBS-FALSE-GREEN-STATUS`):** Update `formatStatusTableStatus` in `cmd/zqk/system/status_helpers.go` to inspect `system_health` and active scheduler issues. If background jobs are failing or the scheduler is down, transition the banner from `✅ Initialized` to `⚠️ Degraded` or `❌ Error`.
2. **Asynchronous Health Cache for MCP Clients (`F-OBS-MCP-FAST-PATH-MASKING`):** Replace `getSystemHealthDataMCPFast` stubbing with a sub-millisecond read of `.zqk/system-health/tier1-latest.json` populated periodically by the background scheduler.
3. **Connect Telemetry Sinks (`F-OBS-TELEMETRY-SINK-DISCONNECTION`):** Implement concrete `Hook` sinks in `pkg/telemetry` that write span and metric records to a JSONL log or SQLite ring buffer, enabling `zqk system telemetry` to surface real execution trends.
4. **Propagate W3C Trace Headers (`F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED`):** Inject W3C `traceparent` headers into `LogEntry` attributes and pass `TraceContext` across Privileged Writer IPC sockets and scheduler job metadata.
5. **Author Operator Runbooks (`F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE`):** Add `docs/runbooks/` with troubleshooting procedures for common operator failure scenarios (CAS quarantine cleanup, lock release, and scheduler restart).
