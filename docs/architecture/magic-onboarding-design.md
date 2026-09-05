# Design Specification: Magic Onboarding (`zqk join`)

**Last Verified:** 2026-08-31


## Vision
The Sovereign Mesh must be as easy to join as a Wi-Fi network, but as secure as a military-grade bunker. The `zqk join` command is the "front door" to the federated economy. It should feel like magic—abstracting away the complexities of PKI, capability manifests, and mesh topology into a single, fluid interaction.

## The Experience (The "Keynote" Moment)
When a user runs `zqk join <url>`, they should experience:
1.  **Instant Identification**: "Connecting to Nexus-Prime..."
2.  **Visual Proof of Trust**: A clear summary of the peer's identity and trust metrics.
3.  **Capacity Discovery**: "Nexus-Prime offers: 64 Core Compute, 12 Specialized Agents, 2TB Knowledge Graph."
4.  **One-Click Federation**: "Join Nexus-Prime? [Y/n]"
5.  **Success Milestone**: "Welcome to the Mesh. Nexus-Prime is now a trusted peer."

## Technical First Principles
1.  **URL-First**: The entry point is always a URL (MCP endpoint).
2.  **Bidirectional Handshake**: Both kernels must agree on the terms of the lease.
3.  **Local Persistence**: The peer is registered as a `remote_kernel` object in the local kernel immediately.
4.  **Automatic Alias**: If no alias is provided, the command should suggest one based on the peer's Title.

## Command Structure
`zqk join <peer-url> [--alias <alias>] [--no-interactive]`

### Stages of Execution
1.  **IDENTIFY**: Resolve the URL and fetch the peer's `kernel_id` and `public_key`.
2.  **VERIFY**: Check the peer's signature and trust level (local policy check).
3.  **DISCOVER**: Fetch the peer's `capability_manifest`.
4.  **REGISTER**: Create a `remote_kernel` object in local storage.
5.  **SYNCHRONIZE**: Push local public goals to the peer (optional first-sync).

## Visual Guidelines
- Use **cyan** for progress and discovery.
- Use **green** for successful trust establishment.
- Use **yellow** for advertised capacity highlights.
- Use **interactive prompts** (using `survey` or similar) for the "Join?" moment.

## Future Evolution
- `zqk join --invite-code <code>`: Invite-driven mesh joining with pre-negotiated trust.
- `zqk join --discovery`: Scan local network for available kernels.
