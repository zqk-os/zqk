# ADR: Autonomous Model Provider Federation & Zero-Egress Local Inference Harvester

## Context & Motivation
Following the introduction of turnkey container distributions and embedded local inference engines, production agent swarms running on air-gapped developer workstations and sovereign enterprise nodes require seamless, automated discovery of locally available Large Language Model runtimes.

Currently, configuring local inference engines (such as Ollama, vLLM, llama-server, LocalAI) requires manual environment variable plumbing (`LLM_API_BASE`, `LLM_MODEL_NAME`), brittle endpoint hardcoding, and exposes organizations to unintentional external network egress when local fallback fails.

To establish sovereign, zero-configuration local inference:
1. **Dynamic Engine Federation**: Automatically discover and enumerate active local model serving daemons across standard socket/HTTP endpoints (Ollama at `:11434`, vLLM / llama-server at `:8000`, `:8080`, `:8443`).
2. **Deterministic Priority & Fallback**: Implement a robust fallback chain prioritizing local quantized models (e.g. Qwen2.5-Coder, DeepSeek-R1-Distill) before querying optional upstream proxies.
3. **Zero-Egress Security Invariant**: Implement a strict fail-closed egress guard ensuring that air-gapped workloads cannot leak prompts or token streams to external public IP addresses or unauthorized cloud domains.

## Architectural Decisions

### 1. Provider Registry Schema & Interfaces (`pkg/llm/provider_registry.go`)
- Define `ProviderEngineType` (`EngineOllama`, `EngineVLLM`, `EngineLlamaServer`, `EngineCustomLocal`).
- Define `ProviderEndpointProfile` representing connection parameters, latency profiles, supported quantization types, context ceilings, and health check routes.
- Implement thread-safe registry `EngineRegistry` supporting dynamic provider registration, capability interrogation, and health probing.

### 2. Autonomous Local Discovery Chain (`pkg/llm/discovery.go`)
- Scan local loopback addresses (`127.0.0.1`, `::1`, `localhost`) across registered provider ports.
- Interrogate standard model enumeration APIs (e.g. `/api/tags` for Ollama, `/v1/models` for vLLM / OpenAI-compatible local engines).
- Evaluate candidate models against task capability requirements (coding vs. reasoning) and dynamically select the optimal local engine without user intervention.

### 3. Fail-Closed Zero-Egress Guardrails (`pkg/llm/egress_guard.go`)
- Enforce strict egress policy checking (`POL-LLM-ZERO-EGRESS-001`).
- Intercept HTTP transports to verify target destination against allowed local CIDRs (`127.0.0.0/8`, `::1/128`, UNIX domain sockets) when zero-egress mode is active (`ZQK_ZERO_EGRESS=1`).
- Fail closed with structured diagnostic rejection if a request targets a non-local or public IP address without an explicit cryptographic clearance token.

## Verification & Acceptance
- **Static Floor Invariant**: Complete interface and registry definitions with zero external network dependencies and strict struct validation.
- **Operational Dynamic Proof**: Mock Ollama and vLLM discovery probes correctly resolve active local models and construct an executable fallback client.
- **Negative Adversarial Boundary**: Malformed endpoints, unauthorized egress targets (e.g. public cloud IPs, non-loopback domains), and corrupted model responses are rejected fail-closed with structured error codes.
