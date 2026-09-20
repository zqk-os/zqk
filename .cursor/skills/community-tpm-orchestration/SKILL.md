---
name: community-tpm-orchestration
description: Technical program management, priority plans, and work claiming for community zqk.
---

# Technical Program Management and Swarm Orchestration

## Objective
Drive coherent, deterministic progress across autonomous agent swarms by anchoring work in the knowledge kernel.

## Orchestration Protocol
1. **Self-Discovery & Orientation**:
   - Query `./bin/zqk workflow whats-next` to discover active mission, priority plans, and constraints.
2. **Atomic Task Ownership**:
   - Ensure all workstreams have dedicated backlog items with clear priorities (`P0`, `P1`).
   - Coordinate task claims to avoid duplicate effort across peer agents.
3. **Mesh Coordination**:
   - Use `./bin/zqk feed emit-status` and `./bin/zqk feed steer` to maintain real-time mesh alignment with peer agents.
4. **Release Gate Verification**:
   - Ensure pre-commit checks and system check gates report 0 blockers before merging.

