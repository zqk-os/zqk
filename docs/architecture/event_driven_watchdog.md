# Event-Driven Watchdog & Agent Pool Architecture

## Overview
The Event-Driven Watchdog is a specialized daemon process integrated into the ZQK scheduler. It monitors the ZQK graph kernel and filesystem for real-time events (e.g., stranded tasks, stalled orchestrator loops, syntax panics) and triggers autonomous fission (spawning scoped subagents) to resolve the blockage.

## Core Principles
1. **Object-Driven Registration:** Subscriptions are not hardcoded. Agents, Personas, or Agent Service Pools register their criteria in the graph using `watchdog_registration` objects (e.g., monitor `agent_task` where `status == error`).
2. **Unified Evaluation Loop:** The Watchdog service executes all active registrations in a single, optimized database/cache sweep rather than requiring agents to run independent polling loops, drastically reducing IO overhead.
3. **Decoupled Execution (Pub/Sub):** The Watchdog *evaluates* and *notifies*, but does not execute agent loops directly. It emits an `audit_event` payload directly to an `agent_feed`.
4. **Circuit Breakers:** Limits the re-queueing of tasks or event emission to 3 sequential failures per object to prevent recursive death spirals.

## Agent Pool Integration
A core architectural shift from Phase 12 to Phase 13 is the introduction of **Agent Pools**. Rather than having individual agents responsible for their own monolithic lifecycle loops, the system utilizes pools of specialized workers (e.g., a pool of `Triage-Engineers` or `Integration-Testers`).

### How Watchdog Drives Agent Pools
- **Feed Subscription:** An entire Agent Service Pool subscribes to a specific `agent_feed` populated by the Watchdog.
- **Dynamic Dequeueing:** Idle agents within the pool dynamically dequeue events from the feed. This achieves instant load-balancing. If a surge of test failures occurs, the pool scales to consume the feed concurrently.
- **No Redundant Polling:** Since the Watchdog centralizes the graph queries, the agents in the pool never need to query the database to ask "what should I do next?". They simply sleep until the Watchdog wakes them up via the feed.
- **All-Clear Signals:** Watchdog registrations can be configured with an `all_clear_interval`. The Watchdog will periodically send an all-clear payload to assure the pool the monitoring loop is alive even if no infractions occur.

## Common Pool Activities
Agent pools driven by the Watchdog share common lifecycle activities:
1. **Fission on Demand:** Spawning lightweight, scoped subagents to handle specific tasks (e.g., fixing a specific compiler error).
2. **State Synchronization:** Using the graph backend to coordinate locks so that two agents in a pool do not attempt to fix the same error simultaneously.
3. **Telemetry & Panics:** Capturing panics via `execute.go` defers, transitioning the active task to `error`, and exposing it back to the Watchdog's evaluation loop for another agent in the pool to pick up.
4. **Rollback & Retry:** Automatically restoring code state to a known-good checkpoint before attempting a new fix for a stranded task.
