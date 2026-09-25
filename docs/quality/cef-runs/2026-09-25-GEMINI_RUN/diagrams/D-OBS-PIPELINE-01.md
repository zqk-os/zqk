# D-OBS-PIPELINE-01: ZQK Observability Architecture, Diagnostic Flows, and Visibility Gaps

```mermaid
flowchart TD
    subgraph ExecutionPlanes["Execution Planes & Clients"]
        HumanCLI["Human CLI Operator<br/>(zqk system status)"]
        MCPAgent["Autonomous Agent / MCP Client<br/>(profile: mcp)"]
        SchedulerDaemon["Scheduler Daemon<br/>(Background Jobs / Timers)"]
    end

    subgraph LoggingSubsystem["Structured Logging & Protocol Protection (pkg/logging)"]
        Logger["Logger.Info / Error / Warn"]
        WriterSelector{"selectWriterForLogLevel<br/>& shouldSuppressStdio"}
        StdErrSinks["os.Stderr<br/>(Errors / MCP Diagnostics)"]
        StdOutSinks["os.Stdout<br/>(Protected JSON-RPC Stream)"]
        FileBuffer["Buffered File Sinks<br/>(.zqk/logs/*.log)"]
    end

    subgraph StatusEngine["Status & Health Diagnostics (cmd/zqk/system)"]
        BuildStatus["buildStatusData"]
        MCPFastPath{"Profile == 'mcp'?"}
        FastStub["getSystemHealthDataMCPFast<br/>status: 'unknown'<br/>(F-OBS-MCP-FAST-PATH-MASKING)"]
        FullCheck["getSystemHealthData<br/>(exec zqk system check --fast)"]
        TableFormatter["formatStatusTableStatus<br/>'Status: ✅ Initialized'<br/>(F-OBS-FALSE-GREEN-STATUS)"]
    end

    subgraph TelemetrySubsystem["Telemetry & Tracing (pkg/telemetry)"]
        GlobalMgr["GlobalManager().StartSpan"]
        HooksList["hooks []Hook<br/>(Empty - zero production hooks)<br/>(F-OBS-TELEMETRY-SINK-DISCONNECTION)"]
        Tracer["W3C TraceContext<br/>(No propagation across queues/RPC)<br/>(F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED)"]
        Histograms["PrometheusHistogram<br/>(No export API or /metrics)<br/>(F-OBS-UNEXPOSED-HISTOGRAM-METRICS)"]
    end

    subgraph FailureModes["Operational Hotspots & Unobserved States"]
        FailedJobs["5 Failed Scheduler Jobs<br/>(SCH-autofix-run exit 1)"]
        Quarantine["CAS Quarantine<br/>(15 duplicate hash files)"]
    end

    %% Wiring
    HumanCLI -->|"Runs zqk system status"| BuildStatus
    MCPAgent -->|"Runs status via MCP"| BuildStatus
    BuildStatus --> MCPFastPath
    MCPFastPath -->|"Yes"| FastStub
    MCPFastPath -->|"No"| FullCheck

    BuildStatus --> TableFormatter
    TableFormatter -->|"Masks failed jobs behind ✅ Initialized"| HumanCLI

    SchedulerDaemon -->|"Job failures recorded"| FailedJobs
    FailedJobs -.->|"NOT surfaced in system status table"| TableFormatter

    Logger --> WriterSelector
    WriterSelector -->|"MCP Mode / Error"| StdErrSinks
    WriterSelector -->|"CLI Data Output"| StdOutSinks
    WriterSelector -->|"File Logging"| FileBuffer

    HumanCLI -.->|"Invokes commands"| GlobalMgr
    GlobalMgr --> HooksList
    HooksList -->|"Dropped (No-op)"| TelemetrySubsystem
```

### Architectural Hotspot Summary

1. **False-Green Barrier (`F-OBS-FALSE-GREEN-STATUS`):** The primary human CLI status command (`zqk system status`) evaluates only directory presence for its main status badge, returning `✅ Initialized` even while the background scheduler reports persistent failures in `SCH-autofix-run`.
2. **MCP Fast-Path Disconnection (`F-OBS-MCP-FAST-PATH-MASKING`):** Agents executing with `--context mcp` are routed around system validation checks into `getSystemHealthDataMCPFast`, stubbing status to `unknown` and blinding autonomous agents to kernel degradation.
3. **Telemetry Sink Void (`F-OBS-TELEMETRY-SINK-DISCONNECTION`):** Tracing and metric hooks called by CLI dispatchers are dropped because `GlobalManager` contains zero registered production hooks and `Daemon.Aggregate` is a no-op stub.
4. **Isolated Latency Histograms (`F-OBS-UNEXPOSED-HISTOGRAM-METRICS`):** Exponential histogram buckets for IPC and ghost-drift MTTR cannot be exported or scraped.
5. **Trace Propagation Gap (`F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED`):** W3C TraceContext headers are defined but not passed across storage contexts, worker pools, or daemon RPCs.
