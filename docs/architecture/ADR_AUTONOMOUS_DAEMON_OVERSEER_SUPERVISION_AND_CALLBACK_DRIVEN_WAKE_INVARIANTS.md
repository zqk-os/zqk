# ADR: Autonomous Daemon Overseer Supervision & Callback-Driven Wake Invariants

## Context
In an autonomous operating system governed by a persistent Knowledge Kernel, background system daemons (scheduler, ambient event loop, steward, privileged-writer) provide essential event loop progression, periodic maintenance, and async coordination. If these daemons crash, lose registration, or become unresponsive without active supervisor recovery, autonomous swarms stall and human intervention is required. Furthermore, when agents wait for asynchronous external tasks, busy-polling or idle waiting introduces compute waste and latency; callback-driven reactive wakes are required to maintain continuous loop progression.

## Decision
1. **Unified Process Group & Launchctl Overseer Supervision**:
   - Background system daemons are coordinated through the Overseer subsystem (`pkg/daemon/overseer`), which maintains unified process group containment, atomic shutdown signaling, and subreaper process isolation.
   - On macOS hosts, `launchctl` service plists are codified with persistent keepalive contracts (`KeepAlive: true`, `SuccessfulExit: false`), ensuring host-level resurrection of supervisor infrastructure across crashes or reboots.
   - Daemons report periodic heartbeats and health probes; consecutive failures transition service state deterministically through healthy, degraded, and unhealthy states with exponential backoff bounds.

2. **Callback-Driven Reactive Wake Architecture**:
   - To eliminate agent idleness and polling loops, background jobs and external events dispatch asynchronous callback notifications (`pkg/daemon/overseer` wake dispatcher).
   - Agents register callbacks to sleep reactively and wake instantly upon task completion, timer expiration, or kernel mutations.
   - Malformed wake payloads lacking valid event kinds, source identifiers, or timestamps are quarantined immediately. Excessive malformed wake events trip an integrated circuit breaker to protect the event loop.

3. **Verification Invariants & DoD**:
   - Unit and integration tests in `pkg/daemon/overseer` enforce static floor invariants (launchctl plist schemas), operational proofs (heartbeat and health probe transitions), and negative adversarial boundaries (malformed wake quarantine and circuit breaker trip).

## Status
Accepted and codified in Knowledge Kernel for Cycle 87 (Autonomous Daemon Overseer Supervision & Callback-Driven Wake Invariants).
