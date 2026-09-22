# Maintenance and Development Guide

Welcome to the ZQK Core Maintenance and Development documentation. This section covers foundational engineering conventions, policy durability models, static analysis gates, and lifecycle hygiene for contributors and autonomous agents maintaining the ZQK Knowledge Kernel.

---

## Key Topics

| Guide | Description |
| :--- | :--- |
| **[Policy Governance & Durability](./POLICY_GOVERNANCE_AND_DURABILITY.md)** | The authoritative specification of policy conventions, cross-instance durability tiers, prefix schemas, and AST enforcement. |
| **[CLI Command Taxonomy](../architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md)** | Naming grammar, command DNA, output contracts, and exit codes for all CLI verbs. |
| **[Scheduler & Maintenance](../howto/SCHEDULER_AND_MAINTENANCE.md)** | Daemon management, kernel survival jobs, and periodic verification timers. |
| **[Quality & Verification Gates](../quality/README.md)** | Verifiable Decomposition Spine (VDS), test dashboards, and pre-commit checks. |
| **[Contributing & DCO](../../CONTRIBUTING.md)** | Code of conduct, developer certificate of origin (DCO), and integration branch workflows. |

---

## Core Engineering Principles

1. **Kernel Primacy (Object-First Invariant)**:
   Every architectural decision, requirement, policy, work unit, and verification result starts as a typed Knowledge Kernel object. Ephemeral chat prose and vendor-local configs are demoted to temporary projections.

2. **Cross-Instance Durability**:
   Authoritative system specifications and default policies are deterministic across all installations, ensuring that autonomous swarms moving between nodes operate against an identical governance model.

3. **Fail-Closed Gate Discipline**:
   Any missing required attribute (such as `description` per `POL-DOC-001`), unverified invariant, or ambiguous graph state fails closed at the cellular membrane boundary.

4. **Continuous Autonomous Loop Discipline**:
   Engineers and agents follow deterministic handoffs: claim atomic work, execute test-driven changes, pass pre-commit integrity gates, open plan-scoped integration branches, and advance the milestone without idle stalls.
