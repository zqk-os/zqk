# Workflow Engine & Continuous Autonomy Subsystem Guide

## Overview

The `pkg/workflow` subsystem powers autonomous orchestration, lifecycle gating, quality linter controls, and real-time execution guidance across the ZQK Knowledge Kernel. It houses both the core **Ontological Linter** enforcing measurable, non-superficial engineering standards and the **What's-Next Decision Engine** (`pkg/workflow/whatsnext`), which determines the exact operational next step for agents and human operators without relying on unstructured LLM prompt drifting.

```
+-----------------------------------------------------------------------------+
|                        ZQK Workflow Engine Subsystem                        |
+-----------------------------------------------------------------------------+
                                       |
                   +-------------------+-------------------+
                   |                                       |
                   v                                       v
     +--------------------------+            +--------------------------+
     |   Anti-Superficiality    |            |   Whats-Next Materialized|
     |   & Three-Fold Linter    |            |   View & Decision Engine |
     |   (pkg/workflow/linter)  |            |   (pkg/workflow/whatsnext) |
     +--------------------------+            +--------------------------+
                   |                                       |
                   +-------------------+-------------------+
                                       |
                                       v
                    +-------------------------------------+
                    |       CAP Flywheel Orchestrator     |
                    +-------------------------------------+
                     /                 |                 \
                    v                  v                  v
     +--------------------+  +--------------------+  +--------------------+
     | Continuous Loop    |  | Ambient Kernel     |  | Strategic Align    |
     | Anti-Idleness Gate |  | Signal Stewardship |  | Cache Enforcement  |
     +--------------------+  +--------------------+  +--------------------+
```

---

## Core Domain Models & Subsystem Taxonomy

### 1. Ontological Anti-Superficiality Linter (`linter.go`)

Quality in the ZQK Knowledge Kernel is enforced programmatically before any requirement or criteria can cross into execution or shovel-ready states. The linter inspects kernel objects across five fields (`title`, `description`, `statement`, `acceptance_criteria`, `rationale`):

- **Banned Superficial Phrasing:** Rejects vague, unmeasurable directives (e.g., *"make code better"*, *"make it cleaner"*, *"improve the code"*, *"fix things"*, *"various improvements"*, *"good enough"*, *"works fine"*). Violations emit `ErrSuperficialContent`.
- **The Three-Fold Proof Formula:** Every `requirement` object must be backed by at least three verifiable `criteria` spanning the three essential dimensions:
  1. **Static Floor Invariant (`invariant`):** Structural invariants, code-level metrics, statement test coverage thresholds, schema conformations, and static analysis constraints.
  2. **Operational Dynamic Proof (`dynamic`):** Functional executions, end-to-end integration workflows, deterministic state transitions, and observable runtime outcomes.
  3. **Negative Adversarial Boundary (`adversarial`):** Fail-closed boundaries, rejected malformed inputs, concurrency conflict handling, unauthorized access denial, and corruption recovery.

Failure to satisfy all three dimensions causes `CheckShovelReadyQuality` to return `ErrThreeFoldFormulaUnsatisfied`, blocking premature execution promotion.

### 2. Whats-Next Decision Engine (`pkg/workflow/whatsnext`)

The `whatsnext` engine evaluates live kernel state, active priority plans, and backlog items to derive the immediate highest-leverage action for autonomous agents:

- **Phi Lead Plan Evaluation (`EvaluatePhiLead`):** Prioritizes in-progress priority plans over active plans. When multiple plans share status, priority order is determined deterministically by `active_order` ascending.
- **Backlog Triage & Starvation Detection:** Analyzes shovel-ready backlog items linked to candidate plans. When a plan has zero executable items, it flags a starvation warning (`SwarmStarved`) and redirects effort toward backlog grooming.
- **Materialized View Caching (`materialized_view.go`):** Pre-computes graph rollups into compact local state files (`.zqk/state/materialized_view.json`), ensuring `zqk workflow whats-next` responds in sub-5ms latency without scanning the entire content-addressed storage on every tick.
- **Ambient Kernel Signals (`kernel_ambience.go`):** Enriches execution context with Git branch divergence (`BranchAhead`/`BranchBehind`), orphaned temporary file alerts, stale lockfiles, auto-remedy recipes, and TPM process admin seat protections.

### 3. Continuous Autonomy Flywheel (`cap_cycle.go`)

The Continuous Autonomous Progression (CAP) flywheel governs multi-agent iteration cycles:
- **Phase Transitions:** Cycles move strictly through validated stages (`grooming`, `testing`, `implementation`, `verification`, `milestone_handoff`).
- **Tamper-Evident Advance Journal:** Every stage transition appends a cryptographic SHA-256 entry to `.zqk/state/cap_advance_journal.jsonl`. If manual file tampering is detected (`CapCycleTamperDetected`), the engine safely rolls back state to the last verified journal entry.
- **Anti-Idleness Protocol:** Merging a pull request or producing an artifact summary is treated as a milestone transition, never a stopping condition. The engine directs the agent to immediately fetch upstream, promote binaries, query what's-next, and advance to the next priority plan.

---

## Verification & Conformance Architecture

```
+-----------------------------------------------------------------------------+
|                         Three-Fold Verification Matrix                      |
+-----------------------------------+-----------------------------------------+
| Proof Dimension                   | Subsystem Implementation                |
+-----------------------------------+-----------------------------------------+
| 1. Static Floor Invariant         | pkg/workflow test coverage >= 85.0%     |
|                                   | (98.7% verified across linter & engine) |
| 2. Operational Dynamic Proof      | Deterministic whats-next recommendations|
|                                   | and CAP flywheel journal validation     |
| 3. Negative Adversarial Boundary  | Fail-closed rejection of vague phrases, |
|                                   | incomplete 3-fold criteria, and tampered|
|                                   | advance journals                        |
+-----------------------------------+-----------------------------------------+
```

### 1. Static Floor Invariant

The `pkg/workflow` package maintains a strict test coverage invariant ($\ge 85.0\%$). Unit tests verify formula classification heuristics, explicit category overrides, text scanning tokenizers, and full coverage of the anti-superficiality rule catalog.

### 2. Operational Dynamic Proof

Operational correctness is validated across end-to-end scenarios:
- `TestLintRequirementWithCriteria_ThreeFoldFormula`: Proves that requirements with all three dimensions pass verification, while missing any single dimension triggers deterministic rejection.
- `TestCycle147_CompressWhatsNextMeasure_NilAndPopulated`: Proves convergence session measures are compressed accurately into lean agent context payloads without loss of router hints.
- `TestCycle147_HumanLogClusters_FileParsing`: Verifies human log event extraction, clustering, ranking, and digest synthesis.

### 3. Negative Adversarial Boundary

The subsystem acts as a fail-closed gate against low-quality inputs:
- Disallows superficial, ambiguous, or unverifiable requirement descriptions.
- Rejects criteria bundles missing static, dynamic, or adversarial proofs.
- Ensures zero prohibited studio nanos-hex identifiers appear in production packages, docs, and CLI specifications.

---

## Developer & Operator Usage Guide

### Inspecting Workflow Next Steps

To query the kernel's real-time execution guidance:

```bash
zqk workflow whats-next
```

For detailed metrics and synchronous storage sweep:

```bash
zqk workflow whats-next --sync-sweep
```

### Validating Requirement Quality

To evaluate a requirement against the Three-Fold Proof before promotion:

```bash
zqk workflow lint REQ-DEMO-001
```

If quality violations or banned phrases exist, the linter reports explicit blocker findings detailing the offending field and phrase.
