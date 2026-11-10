# ADR: Standalone Containerized Zero-Touch Distribution & Embedded Local Inference Swarms

## Context & Motivation
Adoption friction remains the primary barrier to broader developer engagement with the ZQK Knowledge Kernel. Currently, evaluating ZQK requires installing Go 1.26+, setting up local development tools, configuring host daemons (`launchctl` on macOS, `systemd` on Linux), and providing remote LLM API keys.

To achieve frictionless zero-touch evaluation and adoption:
1. Provide official, turnkey container images published to GitHub Container Registry (`ghcr.io/zqk-os/zqk`).
2. Offer a standalone variant bundled with embedded local inference via `llama.cpp` / quantized models (e.g. Qwen2.5-Coder-7B-Instruct-GGUF or DeepSeek-R1-Distill-Qwen-7B-GGUF).
3. Allow developers with standard hardware (CPU/RAM only) to execute complete autonomous swarms (`zqk do`) without cloud API keys, network ingress requirements, or host environment pollution.

## Architectural Decisions

### 1. Dual Container Topology
- **Image A: `ghcr.io/zqk-os/zqk:latest` (Slim Open-Core CLI & Daemon)**
  - Multi-stage minimal scratch/alpine build.
  - Contains compiled `zqk` binary, MCP adapter, and default ontological rules.
  - Mounts local directory (`-v $(pwd):/workspace`) for zero-install repository management.
- **Image B: `ghcr.io/zqk-os/zqk-standalone:latest` (Self-Contained Local Inference Swarm)**
  - Includes compiled `llama.cpp` server with CPU SIMD/AVX2 acceleration.
  - Bundles a 4-bit quantized high-capability coding model (e.g. Qwen2.5-Coder-7B Q4_K_M).
  - Automatically initializes local inference service at `http://127.0.0.1:8080/v1` (OpenAI-compatible) and routes ZQK's provider adapter directly to local localhost inference.
  - Zero cloud dependencies, zero external network egress required.

### 2. Operational Ergonomics
- Single command execution:
  ```bash
  docker run --rm -it -v $(pwd):/workspace -w /workspace ghcr.io/zqk-os/zqk-standalone:latest do "Add JWT authentication middleware with unit tests"
  ```
- Graceful resource adaptation: dynamically configures CPU thread count and context window (4096 tokens) based on detected container memory limits.

## Verification & Acceptance
- Static Invariant: Container definition adheres strictly to non-root execution (`USER 10001`), explicit healthcheck endpoint, and clean directory bindings.
- Dynamic Proof: Local inference provider successfully responds to standard chat completion and structured tool calls.
- Boundary Test: Container gracefully fails closed with actionable diagnostic error if allocated container memory is below 6GB.
