# Developer Swarm Test-Drive & Interactive Walkthrough Experience

## Overview

The Developer Swarm Test-Drive (`zqk test-drive` and `zqk system test-drive`) is an interactive, guided onboarding and simulation engine within the ZQK Knowledge Kernel. It allows operators, developers, and autonomous agents to experience the full 5-layer cascade and concurrent AST-verified swarm execution model without requiring manual orchestration setup.

Unlike transient, prompt-promiscuous chat harnesses where task context and specifications evaporate after every prompt, the Knowledge Kernel turns raw intent into versioned, verifiable graph objects in a persistent Content-Addressed Storage (CAS) hierarchy, executes concurrent verification across multi-worker swarms, and mints tamper-proof Verification Data Specification (VDS) cryptographic delivery receipts.

---

## 1. Architectural Stages

The test-drive walks through four distinct stages of the ZQK lifecycle:

```
┌─────────────────────────────────────────────────────────────┐
│ Stage 1: Seed Planting                                      │
│ • Detect execution environment & agent seating              │
│ • Establish ambient holon context and baseline health       │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Stage 2: Intent Crystallization                             │
│ • Parse developer prompt or selected scenario               │
│ • Mint 5-layer cascade: Vision → Goal → Plan → Req → BLI   │
│ • Establish cryptographic CAS pointers and bidirectional refs│
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Stage 3: Swarm Execution                                    │
│ • Allocate token-budgeted multi-agent swarm workers         │
│ • Parallel AST validation and dynamic test execution        │
│ • Real-time telemetry propagation across agent mesh         │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Stage 4: Cryptographic Verification Receipt                 │
│ • Generate SHA-256 VDS proof receipt                        │
│ • Verify static floor invariants & operational boundaries   │
│ • Emit actionable next steps and operator handoff briefing  │
└─────────────────────────────────────────────────────────────┘
```

### Stage 1: Seed Planting
Initializes local runtime structures, verifies ambient seating (`PER-DEFAULT-OPERATOR`), and confirms kernel CAS integrity before starting execution.

### Stage 2: Intent Crystallization
Transforms human natural language into persistent graph objects across the five relational tiers:
- **Vision / Mission**: Anchors timeless organizational purpose.
- **Goal**: High-level strategic compass outside calendar constraints.
- **Priority Plan**: 1-cycle time-bounded execution container.
- **Requirement & Criteria**: Static, dynamic, and adversarial quality contracts.
- **Backlog Items (BLIs)**: Atomic units of engineering effort.

### Stage 3: Swarm Execution
Simulates concurrent multi-worker execution across multiple agent seats (Craftsman, Sentinel, Steward). AST structures and test fixtures are compiled and verified concurrently under strict token budgeting.

### Stage 4: Cryptographic Verification Receipt
Issues a deterministic SHA-256 Verification Data Specification (VDS) delivery receipt confirming that all static invariants, dynamic proofs, and negative boundaries were validated.

---

## 2. CLI Command Taxonomy & Ergonomics

The test-drive engine is exposed at two ergonomic entry points:

```bash
# Top-level entry point
zqk test-drive [flags]

# Subsystem entry point
zqk system test-drive [flags]
```

### Aliases
- `zqk td`
- `zqk testdrive`
- `zqk system td`

### Command Flags

| Flag | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `--intent` | `string` | Custom developer intent prompt (min 3 chars, max 512 chars) | Built-in scenario prompt |
| `--scenario` | `string` | Pre-configured scenario (`api-service`, `cli-tool`, `stream-pipeline`) | `api-service` |
| `--speed` | `string` | Pacing speed (`instant`, `fast`, `normal`) | `normal` |
| `--non-interactive` | `bool` | Headless execution mode for CI/scripting | `false` |
| `--dry-run` | `bool` | Simulate workflow progression without committing disk mutations | `false` |

---

## 3. Operational Scenarios & Examples

### Interactive Guided Onboarding
Ideal for first-time developers or pair-programming sessions with an AI assistant:
```bash
zqk test-drive
```
The console guides the operator through selecting a template scenario or typing an intent prompt, then visualizes each stage with progressive disclosure formatting.

### Custom Prompt Intent
Demonstrating how custom engineering requirements crystallize into kernel objects:
```bash
zqk test-drive --intent "Build an authenticated SQLite task API with telemetry"
```

### Headless CI / Automated Verification
Used in regression test pipelines and synthetic verification harnesses:
```bash
zqk test-drive --non-interactive --speed instant --scenario cli-tool
```

---

## 4. Verification Contracts & Quality Invariants

The test-drive subsystem is governed by rigorous Three-Fold Proof verification:

1. **Static Floor Invariant (`TestTestDrive_StaticFloorInvariant`)**:
   - Asserts default scenario bindings (`api-service`).
   - Asserts strict boundary guards on intent length ($3 \le \text{len} \le 512$).
   - Verifies step delay mapping across `instant`, `fast`, and `normal` speeds.

2. **Operational Dynamic Proof (`TestTestDrive_OperationalProof_EndToEndSimulation`)**:
   - Executes full 4-stage pipelines across all built-in scenarios.
   - Asserts 8 minted graph objects, 3 swarm worker allocations, and valid SHA-256 VDS receipts.
   - Verifies formatted stage headers and completion banners in stdout.

3. **Negative Adversarial Boundary (`TestTestDrive_NegativeBoundary_AdversarialScenarios`)**:
   - Enforces defensive handling of short inputs (`< 3` characters).
   - Enforces rejection of oversized payloads (`> 512` characters).
   - Validates EOF and empty-input fallback behavior in interactive stdin scanners.

---

## 5. Architectural Alignment & CAS Objectification

In accordance with `.agents/AGENTS.md` and the Knowledge Kernel CAS mandate:
- All architectural decisions and operator guides are checked directly into repository documentation under `docs/guides/`.
- Specifications are objectified into the Knowledge Kernel via `zqk new req` and `zqk workflow gen-trace-pipeline`.
- Verification receipts are cryptographically anchored and validated via `zqk test run <TST-ID>`.
