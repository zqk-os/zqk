# Session Lifecycle, Worker Context & Identity Engine Subsystem Operational Architecture Guide (`pkg/zqksession`)

## Executive Summary

The `pkg/zqksession` subsystem implements session management, cross-process CLI reuse, autonomous worker seat registration, context propagation, and identity prefix resolution for the **ZQK Knowledge Kernel**. In a distributed multi-agent swarm ecosystem executing across interactive developer terminals, scheduled cron jobs, background worker daemons, and Model Context Protocol (MCP) bridges, strict accounting of who is executing an action, what session encapsulates that effort, and how child worker processes inherit context from parent sessions is essential for auditability and coordination.

This subsystem provides:
1. **Interactive CLI Session Lifecycle Management:** Seamless session creation (`StartCLISession`), active state tracking, throttling-aware touch updates (`Touch`, `TouchIfNotThrottled`), and graceful completion (`End`).
2. **Deterministic Cross-Command CLI Session Reuse:** Local state persistence (`WritePersistedID`, `ReadPersistedID`, `TryReuse`, `ClearPersisted`) protected by cross-process filesystem locks (`flock`) to reuse active sessions across discrete command invocations without account collisions.
3. **Autonomous Agent Worker Context Registration:** Strongly-typed worker seat sessions (`WorkerSessionInput`, `StartWorkerSession`) capturing agent identity, model identifier, provider profile, persona, and parent-child session hierarchy.
4. **Parent Reference Storage Quarantine:** Automatic discernment between file-backed persistent parent references and segment-offset stream locators (`persistableParentSessionRef`), preventing reference corruption under stream storage backends.
5. **Context Key Propagation:** Thread-safe Go `context.Context` carriers (`WithID`, `GetIDFromContext`) for propagating active session identities down call trees, database storage calls, and subagent invocations.
6. **Session Identity Pattern Validation:** Resilient heuristic and configured prefix matching (`LooksLikeID`, `IDPrefixes`, `idCore`) supporting pipe-delimited persona suffixes (e.g., `ZQK-ZS-...|persona`).

---

## Architectural Principles & Invariants

```
                               ┌──────────────────────────────────────────────┐
                               │           Execution Entrypoint               │
                               │   (Interactive CLI / Worker Daemon / MCP)   │
                               └──────────────────────┬───────────────────────┘
                                                      │
                       ┌──────────────────────────────┴──────────────────────────────┐
                       ▼                                                             ▼
         ┌───────────────────────────┐                                 ┌───────────────────────────┐
         │     CLI Session Engine    │                                 │   Agent Worker Engine     │
         │  - StartCLISession        │                                 │  - StartWorkerSession     │
         │  - TryReuse / Touch       │                                 │  - Parent Session Linkage │
         │  - End / Idle Invalidation│                                 │  - Model / Persona Context│
         └─────────────┬─────────────┘                                 └─────────────┬─────────────┘
                       │                                                             │
                       ├──────────────────────────────┬──────────────────────────────┤
                       ▼                              ▼                              ▼
         ┌───────────────────────────┐  ┌───────────────────────────┐  ┌───────────────────────────┐
         │    Local State & Lock     │  │   Context Carrier Engine  │  │   Identity & Prefixes     │
         │  - session.lock (flock)   │  │  - WithID(ctx, id)        │  │  - LooksLikeID            │
         │  - .zqk/state/session     │  │  - GetIDFromContext(ctx)  │  │  - IDPrefixes / idCore    │
         │  - last_session_touch     │  └───────────────────────────┘  └───────────────────────────┘
         └─────────────┬─────────────┘
                       │
                       ▼
         ┌───────────────────────────┐
         │   Kernel Storage Plane    │
         │  - Kind: zqk_session      │
         │  - Authorization Membrane │
         └───────────────────────────┘
```

### 1. Account Isolation and Anti-Clobber Invariant
A persisted CLI session on a local developer machine belongs strictly to the originating account (`account_id`). When multiple engineers or automated service principals share a filesystem or switch identities, `WritePersistedID` and `TryReuse` verify that an active existing session is not clobbered by a differing account without explicit completion or expiration.

### 2. Cross-Process File Lock Synchronization
All reads, writes, touches, and clearing of session state files (`.zqk/state/session`, `.zqk/state/last_session_touch`) are synchronized via `storage.NewFileLock` on `.zqk/state/session.lock`. This guarantees that rapid, parallel CLI invocations from shell scripts or agent swarms cannot corrupt session state or trigger write races.

### 3. Touch Throttling Membrane Invariant
To prevent overwhelming kernel storage providers with high-frequency timestamp updates during rapid command bursts, `TouchIfNotThrottled` enforces a 5-second throttle window (`sessionTouchThrottleWindow`). Timestamps are recorded in local state, and storage updates occur only when the throttle window has elapsed, reducing storage I/O by orders of magnitude.

### 4. Idle Timeout Automatic Eviction
CLI sessions maintain a configurable idle lifespan (`defaultIdleTimeout` of 24 hours, overridable via `.zqk/config/zqk.yaml` under `session.idle_timeout`). When `TryReuse` encounters an active session whose `updated_at` exceeds the idle threshold, the session is deterministically transitioned to `completed` in kernel storage and purged from local disk.

### 5. Stream Storage Parent Reference Quarantine
Kernel sessions can be persisted to traditional file/CAS storage or high-volume append-only stream storage. In stream storage mode, object locators take the form of segment offsets (`segmentPath::offset`), which cannot survive standard filesystem reference validation. `persistableParentSessionRef` evaluates `storage.StreamStorageEnabledForKind(objects.KindZqkSession)` and omits parent references when stream storage is active, ensuring downstream reference validators remain fail-closed and healthy.

---

## Core Component Reference

### 1. CLI Session Lifecycle (`session.go`)

- `StartCLISession(ctx, projectRoot, title, accountID, sp)`:
  Initializes an active session object of kind `zqk_session` with `session_type = "cli"`. Generates a collision-resistant batch ID using the kernel CAS ID provider and returns the minted session identifier. Automatically injects `promoteOnCreate` context flags.
- `Touch(ctx, projectRoot, sessionID, title, accountID, sp)`:
  Refreshes the session's `updated_at` RFC3339 timestamp and optionally updates the session title.
- `TouchIfNotThrottled(ctx, projectRoot, sessionID, title, accountID, sp)`:
  Executes `Touch` only if at least 5 seconds have elapsed since the last recorded touch in `.zqk/state/last_session_touch`. If the file lock cannot be acquired, it fails open to a direct touch to guarantee heartbeat preservation.
- `End(ctx, projectRoot, sessionID, status, accountID, sp)`:
  Transitions the session to a terminal lifecycle status (`completed` or `error`), stamping `updated_at` and `updated_by`.

### 2. Local State Filesystem Topology & Reuse (`session.go`)

Local session state is maintained in the project's state directory:
- `.zqk/state/session`: Contains the single active session ID for the workspace.
- `.zqk/state/session.lock`: Advisory `flock` protecting session read/write transitions.
- `.zqk/state/last_session_touch`: High-resolution RFC3339 timestamp of the last touch event.

Key API functions:
- `GetCurrentID(projectRoot)`: Evaluates the `ZQK_SESSION_ID` environment variable first (allowing container/pod injection); if empty, safely reads `.zqk/state/session` under file lock.
- `ReadPersistedID(projectRoot)`: Directly reads `.zqk/state/session` under file lock, bypassing environment variable overrides.
- `WritePersistedID(ctx, projectRoot, sessionID, accountID, sp)`: Safely writes the session ID. If an active session already exists belonging to another account, the write is refused to protect multi-user integrity.
- `ClearPersisted(projectRoot)`: Deletes `.zqk/state/session` and purges the in-memory stamp memoization cache.
- `TryReuse(ctx, projectRoot, title, accountID, sp)`: Validates the currently persisted session against kernel storage. If the session is active, owned by `accountID`, and within the configured idle timeout, it returns `(sessionID, true)`.

### 3. Agent Worker Context (`worker.go`)

The `WorkerSessionInput` struct encapsulates execution telemetry for autonomous agents:

```go
type WorkerSessionInput struct {
    ProjectRoot        string
    AccountID          string
    Title              string
    ParentSessionID    string
    AgentID            string
    PersonaRef         string
    ExecutorType       string // Defaults to "agentx"
    Provider           string // e.g., "anthropic", "cursor", "gemini"
    ProviderProfileRef string
    ModelID            string
}
```

- `StartWorkerSession(ctx, in, sp)`:
  Validates that `AgentID` is non-empty, sets default executor type if unspecified, checks parent session persistability via `persistableParentSessionRef`, and creates a typed session with `session_type = "agent_worker"`.

### 4. Context Propagation (`context.go`)

- `WithID(ctx, id)`: Wraps a Go context with the unexported `sessionIDKey` carrying the session ID.
- `GetIDFromContext(ctx)`: Extracts and type-asserts the session ID string from `ctx`. Returns `""` if context is nil or the key is absent/malformed.

### 5. Identity Invariants & Prefix Parsing (`id.go`)

- `LooksLikeID(raw)`: Strips optional pipe-separated persona suffixes (e.g., `ZQK-12345|developer` becomes `ZQK-12345`) and checks against recognized session prefixes.
- `IDPrefixes()`: Dynamically aggregates session prefixes from the kernel's `id_prefixes_config.yaml`, the canonical `CanonicalIDPrefix`, and the active brand environment prefix, ensuring trailing hyphens are consistently normalized.
- `idCore(raw)`: Extracts the core identifier preceding any `|` persona delimiter.

---

## Configuration Reference

Session behaviors can be customized via `.zqk/config/zqk.yaml`:

```yaml
session:
  idle_timeout: "12h"      # Inactivity threshold before session auto-completion (default: 24h)
```

Supported time units: `s`, `m`, `h`, `d` (standard Go duration parsing).

---

## Operational Troubleshooting & Runbooks

### Runbook 1: Stale Session Lock Contention
**Symptom:** CLI commands stall for 5 seconds with lock timeout warnings.
**Root Cause:** A crashed background process or abruptly killed shell held `.zqk/state/session.lock`.
**Resolution:**
The subsystem utilizes kernel `FileLock` which releases locks automatically upon OS process termination. If a hung process still holds the file descriptor:
```bash
# Identify process holding session.lock
lsof .zqk/state/session.lock
# Safely clear the lock file if no processes are active
rm -f .zqk/state/session.lock
```

### Runbook 2: Multi-Account Session Clobber Rejection
**Symptom:** `WritePersistedID` returns `false` and CLI commands start in stateless mode.
**Root Cause:** A previous session owned by account `ACC-A` is still `active` in `.zqk/state/session`, and account `ACC-B` is invoking commands.
**Resolution:**
Either complete the prior session or explicitly clear the local persisted file:
```bash
# Check current session ID
cat .zqk/state/session

# Explicitly clear local session state
rm -f .zqk/state/session
```

### Runbook 3: Child Worker Missing Parent Reference
**Symptom:** Worker session objects do not have `parent_session_ref` populated.
**Root Cause:**
1. Parent session does not exist in kernel storage (unreadable).
2. Stream storage is enabled for `zqk_session` in `config/zqk.yaml` (`system.stream_storage_enabled: true`). Stream offset locators cannot be validated as filesystem paths and are deliberately omitted by `persistableParentSessionRef`.
**Resolution:**
If parent reference propagation is required, ensure the parent session is committed to standard storage or configure `zqk_session` as a non-stream kind.

---

## Verification & Conformance Invariants

| Proof Dimension | Target Invariant | Verification Mechanism |
| :--- | :--- | :--- |
| **Static Floor Invariant** | `pkg/zqksession` statement coverage $\ge 85.0\%$ (achieved 98.8%) | `go test -v -coverprofile=coverage.out ./pkg/zqksession` |
| **Operational Dynamic Proof** | Session state transitions, worker registration, context propagation, lock contention | `go test -v ./pkg/zqksession -run 'TestCycle246_.*'` |
| **Negative Adversarial Boundary** | Zero prohibited literal nanosecond IDs, clean public release payload | `sh scripts/open-core/check-public-release-payload.sh .` |

---

## Operational Conformance & Governance

The session lifecycle and worker context engine conforms to kernel governance standards and automated verification suites. All active session state transitions, lock contention recovery mechanisms, and worker context hierarchies are verified during continuous integration.

