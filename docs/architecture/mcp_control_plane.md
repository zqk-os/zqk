# Model Context Protocol (MCP): Control Plane vs Data Plane

**Last Verified:** 2026-08-31


## Problem Statement
During the Phase 5 AV Commercial generation, the `Audio-Video Expert` subagent encountered a fatal `500 Unknown Error` when attempting to pass a massive Base64-encoded image payload through the MCP `generate_video` tool. 

Passing multi-megabyte binary assets via JSON-RPC payloads saturates the socket, increases serialization latency, and guarantees memory/context crashes. 

## Architectural Mandate: TCP/IP Inspired Orchestration
Going forward, MCP within the ZQK OS must strictly operate as a **Control Plane**, not a Data Plane.

1. **Control Plane (MCP)**: Agents use MCP tools to pass *pointers*, metadata, and instructions. (e.g., `{"file_path": "/absolute/path/to/image.png", "prompt": "cinematic pan"}`)
2. **Data Plane (Execution)**: The underlying MCP Server reads the massive payload directly from the local filesystem or a chunked blob storage layer, streams it to the remote API, and returns a remote URI or a new local `file_path`.

## Implementation Rules
- Agents are strictly forbidden from embedding Base64 strings or massive binary blobs into tool call arguments.
- Any tool requiring large I/O (Video, Audio, massive codebases) must accept `file_paths` or `cas_hashes` (Content Addressable Storage).
- As the swarm scales, we will build a chunked data-routing protocol for multi-node agent communication.
