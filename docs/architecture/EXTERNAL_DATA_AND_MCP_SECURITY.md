# Non-Functional Requirements: External Data & MCP Security Boundaries

**Last Verified:** 2026-08-31


## 1. Zero-Trust Transit (Signed URLs)
As the ZQK OS federates outward, the Model Context Protocol (MCP) will increasingly route I/O to external compute nodes and third-party APIs (e.g., Video Generation, Cloud Storage). 
- **Mandate**: Raw unencrypted payloads and static API keys must never be transmitted via JSON-RPC over untrusted or interceptable boundaries.
- **Implementation**: Any data transiting the MCP boundary must utilize **Cryptographically Signed URLs** (e.g., short-lived, pre-signed S3/Cloud Storage URLs) or mutually authenticated TLS channels. This ensures payloads cannot be intercepted, replayed, or tampered with in transit.

## 2. External Manipulation & Provenance
When ZQK relies on external systems (APIs, third-party agents) to manipulate or synthesize data:
- **Immutability**: Data pulled back into the Knowledge Kernel from external sources must be treated as immutable and hashed immediately upon ingestion (Content Addressable Storage).
- **Provenance**: The graph must record the exact external origin, the MCP tool used, and the transit timestamp for every external asset, ensuring strict traceability.

## 3. Data-Plane Isolation & Socket Conservation (TCP/IP Pattern)
(See `mcp_control_plane.md`). The MCP layer is strictly the Control-Plane. The Data-Plane handling massive blobs must rely on the localized filesystem or secure blob storage governed by the Signed URL constraints above.
- **Socket Saturation Prevention**: We must not use MCP to pass monolithic large data directly (e.g., raw video assets). Small data payloads are acceptable, but massive assets require special out-of-band handling.
- **Chunking Protocol**: Similar to the TCP/IP stack at the physical layer, large data files must be chunked at the source before transmission. Agents will route these smaller chunks independently to avoid tying up sockets for extended periods, reassembling the asset and verifying its integrity (via hashing) at the destination.
