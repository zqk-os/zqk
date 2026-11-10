# 5-Minute Quickstart: ZQK with Docker

Run autonomous AI coding swarms with zero host configuration and zero dependency installation.

---

## 1. Zero-Install CLI & Kernel (Using Cloud or Local API Keys)

Mount your existing project workspace directly into the lightweight community container:

```bash
# Initialize ZQK in your current repository
docker run --rm -it -v $(pwd):/workspace -w /workspace \
  ghcr.io/zqk-os/zqk:latest init

# Run an autonomous multi-agent engineering swarm
docker run --rm -it -v $(pwd):/workspace -w /workspace \
  -e OPENAI_API_KEY="sk-..." \
  ghcr.io/zqk-os/zqk:latest do "Implement a RESTful health check endpoint with test coverage"
```

---

## 2. Turnkey Standalone Appliance (Zero Cloud Keys, 100% Local Inference)

For privacy-sensitive environments, offline evaluation, or developers without LLM API keys, use the self-contained standalone container bundled with `llama.cpp` and a quantized model (e.g., Qwen2.5-Coder-7B Q4_K_M):

```bash
docker run --rm -it \
  -v $(pwd):/workspace -w /workspace \
  -v /path/to/local/models:/models \
  -e MODEL_PATH=/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf \
  ghcr.io/zqk-os/zqk-standalone:latest do "Add JWT middleware with unit tests"
```

### Hardware Requirements for Standalone
- **CPU**: 4+ cores (AVX2 recommended)
- **RAM**: 8GB minimum (16GB recommended)
- **Network**: Completely offline (0 outbound connections)
