# Ambient Orchestration: Omnipresent Context for the Swarm

**Last Verified:** 2026-08-31


**Status**: Draft
**Workstream**: WS-SYM-001 (Autonomy Inbox & TDE)
**Roadmap**: ROAD-008 (The Year of Symbiotic Mesh)
**Date**: 2026-06-11

## 1. Executive Summary
To achieve "omnipresent context" for the ZQK Swarm, we must extend the **Ambient Engine** from a local filesystem watcher into a distributed contextual transceiver. When a developer or an AI agent takes an action, the resultant AST changes, terminal outputs, and drift events must be immediately available to all specialized cells (Neuron, Muscle, Heart, Lung) in the Zen Quantum Cortex without relying on high-latency database queries.

## 2. Architectural Pillars

### 2.1. The Autonomy Inbox
Each swarm cell maintains an "Autonomy Inbox"—a lock-free, memory-mapped ring buffer receiving `ContextEvent` objects.
* **Sources**: The local `pkg/ambient` watcher, external MCP events, and cross-cell gossip protocols.
* **Filtering**: Inbox events are pre-filtered based on the cell's specialization (e.g., Muscle cells only receive test execution triggers; Neuron cells receive AST diffs).

### 2.2. Predictive Context Envelopes (.csnap)
Instead of broadcasting raw data, the Ambient Engine synthesizes `.csnap` (Compressed Snapshot) envelopes. 
* These envelopes contain pre-parsed AST partials and dependency graphs.
* When a Muscle cell is ordered to compile code, the requisite context is already pre-warmed in its local cache via the inbox.

### 2.3. The Hive Mind Sync (Gossip Protocol)
To avoid a single-point-of-failure global lock (as referenced in Phase 1 Swarm Stream Sharding), cells will utilize a lightweight gossip protocol over the ZQK stream storage profile.
* Sharded streams (`node_neuron_X.jsonl`) will be asynchronously tailed by sibling nodes.
* A dedicated "Heart Cell" orchestrates the compaction of these streams into the main Knowledge Kernel.

## 3. Scaffolding & Initial Implementation

1. **`pkg/ambient/inbox.go`**: Implement the `AutonomyInbox` struct and lock-free ring buffer.
2. **`pkg/ambient/transceiver.go`**: Extend the local watcher to emit `.csnap` envelopes to the swarm stream.
3. **`pkg/hive/gossip.go`**: Initial scaffolding for node-to-node context sharing.

## 4. Traceability
* **Goal**: GOAL-SYM-001 (Ambient Orchestration)
* **Workstream**: WS-SYM-001
* **Requirement**: REQ-SYM-001 (Distributed Contextual Awareness)
