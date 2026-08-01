# Design Specification: The Federated Economy

## Vision
The Sovereign Mesh is not just a network of connected kernels; it is a **Federated Marketplace** where autonomous nodes exchange specialized intelligence, idle compute, and managed storage. The Federated Economy transforms "Skill Leasing" from a technical data-sharing task into a strategic **Value Exchange** model.

## Core Economic Primitives

### 1. Capacity Advertisement (`capacity_advertisement`)
A declaration of available resources a kernel is willing to lease to peers.
- **Resource Types**: 
    - `skill`: Specialized AI agent skills (e.g., "Go AST Expert").
    - `compute`: Goroutine pools, execution threads.
    - `storage`: Content-addressed storage (CAS) quotas.
    - `throughput`: IOPS or API call rate limits.
- **Metadata**: Quantity, availability window, and terms of service (policy reference).

### 2. Federated Lease (`zqk_session`)
A formal agreement (contract) between a **Provider Kernel** and a **Consumer Kernel**, utilizing the `zqk_session` object configured with `session_mode: federated_lease`.
- **Lifecycle**: Managed via standard status transitions (e.g., `proposed` -> `implemented` -> `expired` | `revoked`).
- **Authorization**: A cryptographic `token_id` generated during the lease handshake that the consumer must present to the provider's MCP server.
- **Enforcement**: Security models (e.g., `grantor_enforced`), `term_type` (invocations, time, etc.), and `contract_digest` are validated directly on the session object.
- **Auditability**: Every use of a leased skill is recorded in the provider's audit trail and billed (metrically) to the consumer's identity.

### 3. The Marketplace (`mesh market`)
A decentralized view of all available advertisements across the federated peers.
- **Discovery**: When a kernel joins the mesh, it automatically pulls the `capacity_advertisement` set from the peer.
- **Selection**: A kernel can search the market for a missing skill (e.g., "Need a kernel with `rust-architect` skill").

## The User Experience
1.  **Advertise**: `zqk mesh advertise compute --quantity 100 --units "goroutines"`
2.  **Discover**: `zqk mesh list-market`
3.  **Lease**: `zqk mesh lease AGE-123 --from REM-NEXUS`
4.  **Utilize**: Once leased, the skill appears in `zqk mesh list-skills` and can be invoked as if it were local.

## Technical Architecture
- **Handshake Extension**: The `HandshakeResponse` now includes the `capacity_advertisement` set.
- **Sync Create**: Leases must use `storage.WithSyncCreateForKind` to ensure both kernels see the agreement immediately.
- **Policy Enforcement**: The provider's `SecurityContext` must check for an active federated lease (`zqk_session`) before allowing a remote consumer to execute a skill.

## Evolution
- **Dynamic Pricing**: (Future) Automated negotiation based on supply/demand of compute across the mesh.
- **Recursive Leasing**: A consumer can sub-lease resources to its own peers (The Mesh Relay).
