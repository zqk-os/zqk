# Federated Economy Governance

## Overview
This document outlines the governance and security model for the Sovereign Mesh—a distributed market for compute and agent-skill exchange.

## Core Protocols

### 1. Identity Verification (REM Objects)
All remote kernels must authenticate via mTLS before they are recognized by the `remote_kernel` identity registry. No capacity or skills can be leased from unverified identities.

### 2. Capacity Advertisement (CAP)
Kernels broadcast `CapacityAdvertisement` objects. 
- **Integrity**: Each `CAP` must be cryptographically signed by the kernel's private identity key.
- **Expiry**: All advertisements have a TTL; expired advertisements are automatically pruned from the local graph by the system heartbeat.

### 3. Skill Lease (TOK)
Leases are atomic contracts for compute.
- **Binding**: Every lease object must include a `backlog_item_ref` or `priority_plan_ref`.
- **Governance**: If the underlying `PriorityPlan` is revoked or status updated to `cancelled`, all associated `SkillLease` objects must trigger an immediate `policy_interrupt` and state reconciliation.

## Governance Enforcement
- **Anti-Pollution**: Any kernel attempting to advertise capacity beyond its actual resource utilization is subject to immediate jailing (revocation of trust).
- **Auditability**: All `CAP` and `TOK` operations are persisted in the `change_journal_entry` stream.
