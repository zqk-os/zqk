---
diagram_id: D-SECURITY-01
type: data_flow
title: "ZQK Trust Boundaries, Privilege Membranes, and Attack Surfaces"
anchors:
  - path: cmd/zqk/app/auth_middleware.go
    symbol: AuthMiddleware
    note: "CLI authentication interceptor"
  - path: pkg/storage/privileged_writer_membrane.go
    symbol: ProjectScopedPrivilegedWriterSocketPath
    note: "Privileged writer IPC boundary"
  - path: pkg/mcp/cpcp_interceptor.go
    symbol: CPCPInterceptor.ValidateToolCall
    note: "MCP tool membrane interceptor"
  - path: pkg/scheduler/handlers_callback_listener.go
    symbol: CallbackListenerHandler.handleCallback
    note: "Scheduler callback listener HTTP surface"
claims:
  - "AuthMiddleware gates CLI commands into storage engine with test flag bypass"
  - "CAS mutations delegate to the PrivilegedWriter daemon over local Unix domain socket"
  - "MCP tool invocations pass through CPCP interceptor to prevent direct mutations to .zqk/process"
  - "Callback listener and ambient ingest expose HTTP loopback endpoints"
evidence_grade: E2
---

# ZQK Trust Boundaries, Privilege Membranes, and Attack Surfaces

```mermaid
flowchart TD
    subgraph UntrustedCallers["Callers & Autonomous Agents"]
        Agent["Swarm / AI Agent"]
        CLIUser["Interactive Terminal User"]
        BrowserClient["Local Browser / Web App"]
    end

    subgraph EntrySurfaces["Entrypoint & Routing Surfaces"]
        CLI["cmd/zqk CLI"]
        MCP["pkg/mcp Server"]
        HTTPAPI["pkg/agentfeed/httpapi (:8787)"]
        CallbackHTTP["pkg/scheduler callback listener (:8080)"]
        AmbientHTTP["cmd/zqk/ambient ingest (:port)"]
    end

    subgraph Membranes["Validation & Privilege Membranes"]
        AuthMid["cmd/zqk/app.AuthMiddleware\n(RBAC & Credential Check)"]
        CPCP["pkg/mcp.CPCPInterceptor\n(Tool Name & Path Filter)"]
        PWMembrane["pkg/storage.writeCASThroughMembrane\n(Unix Domain Socket Proxy)"]
    end

    subgraph Daemons["Privileged Daemons & Kernel Storage"]
        PWDaemon["Privileged Writer Daemon\n(.zqk/run/zqk-privileged-writer.sock)"]
        StorageEngine["pkg/storage.FileObjectStorage\n(.zqk/process, .zqk/cas, .zqk/wal)"]
        Scheduler["pkg/scheduler.SchedulerEngine"]
    end

    %% Invocations
    CLIUser -->|argv| CLI
    Agent -->|JSON-RPC| MCP
    BrowserClient -->|HTTP / WS| CallbackHTTP
    BrowserClient -->|HTTP / WS| AmbientHTTP
    Agent -->|HTTP| HTTPAPI

    %% Gating
    CLI --> AuthMid
    AuthMid -.->|Bypass via -test.*| StorageEngine
    AuthMid -->|Authenticated| StorageEngine

    MCP --> CPCP
    CPCP -->|Tool Execution| StorageEngine

    StorageEngine --> PWMembrane
    PWMembrane -->|RPC / checkAffinity| PWDaemon
    PWDaemon -->|Direct Write| StorageEngine

    CallbackHTTP -->|Unauthenticated trigger| Scheduler
    HTTPAPI -->|Steer / Ack| StorageEngine
```
