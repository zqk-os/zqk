# The Cryptographic Transceiver: Zero-Trust Mesh Routing

**Date:** 2026-06-02
**Context:** The `External Mesh Gateway` requires NAT traversal (relays/tunnels) to enable asynchronous webhook reception. However, routing data through third-party or even first-party hosted relays introduces Man-in-the-Middle (MITM) vulnerabilities. To satisfy Enterprise CISO requirements, ZQK must guarantee that data transiting the relay is structurally opaque to the relay itself.

## The CISO Objection
*   "If our agent delegates a skill to an external node, and the response tunnels through `relay.zqk.network`, how do I know ZQK Inc. (or a compromised proxy) isn't inspecting or modifying the payload?"

## The ZQK Solution: End-to-End Cryptographic Envelopes

The Sovereign Relay is a "dumb pipe." It routes bytes based on session IDs but cannot read the contents. All data is sealed within a **Cryptographic Transceiver Protocol**.

### 1. Asymmetric Key Exchange (The Handshake)
When a local ZQK daemon delegates a task or registers for a webhook:
1. It generates a temporary, single-use `Curve25519` keypair.
2. It sends the `public_key` to the external service (e.g., VEED API or another Mesh Node).

### 2. The Sealed Payload (JWE - JSON Web Encryption)
When the external service is ready to send the webhook (or result) back to the local daemon:
1. The external service encrypts the payload using the daemon's single-use `public_key`.
2. The payload is signed with the external service's private key (ensuring non-repudiation/provenance).
3. The resulting JWE object is sent to the `relay.zqk.network` endpoint.

### 3. The "Dumb" Relay
The relay only sees:
```json
{
  "routing_id": "zqk_cb_987654321",
  "payload": "eyJlbmMiOiJBMjU2R0NNIiwiYWxnIjoiRUNESC1FUytBMjU2S1cifQ..." 
}
```
The relay uses the `routing_id` to push the encrypted blob through the websocket/tunnel to the local developer's machine. The relay mathematically *cannot* read the data.

### 4. Local Decryption & Hydration
1. The local ZQK daemon receives the blob.
2. It uses its single-use `private_key` (held safely in local memory or the local `.zqk-state` keystore) to decrypt the payload.
3. It verifies the signature.
4. Only then does it pass the data to the Agent Context for re-hydration.

## Strategic Impact
By implementing this, ZQK converts a massive security vulnerability (tunneling through firewalls) into an enterprise sales feature. We can definitively tell a bank or a hospital: **"Our network physically cannot read your agent's data. You hold the decryption keys locally."**
