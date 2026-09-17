# ZQK Local Inference Testing Guide

This guide details best practices and testing strategies for validating ZQK's local LLM orchestration capabilities. Running ZQK against local open-weights models is critical for hardening the engine against edge cases, JSON mangling, and context window limits.

**Which model for which job:** [`docs/architecture/LLM_INTERACTION_MODEL_RUBRIC.md`](../architecture/LLM_INTERACTION_MODEL_RUBRIC.md) — AgentX doer vs code-draft vs prose vs embed. Do not treat HumanEval as proof of native `tool_calls`.

## 1. Supported Model Tiers

### The Core Baseline (The "Must-Work" Tier)
These models are the community standard. ZQK must work flawlessly with them.
*   **`llama3:8b-instruct` / `llama3:70b-instruct`:** The gold standard for open instruction-following. Llama 3 does *not* use Sliding Window Attention (SWA), providing a stable baseline for pure orchestration speed and cache reuse.
*   **`qwen2.5-coder:32b` / `7b`:** Strong at writing file bodies. **Not** a drop-in OpenAI tools doer — Coder weights use the Qwen2/Qwen-Agent template, so `/v1/chat/completions` usually returns JSON in `content` instead of `tool_calls`. Use **`Qwen2.5-*-Instruct`** (or Qwen3) with a Hermes/jinja tool parser for AgentX doer seats; keep Coder-7B on the write-only draft harness.

### The Architectural Outliers (The "Flexibility" Tier)
These models use different architectures or training paradigms that stress-test ZQK's `pkg/llm` abstractions.
*   **`mixtral:8x7b-instruct`:** A Mixture of Experts (MoE) model. High token generation speed but demands significant RAM. Validates multi-turn speed.
*   **`gemma2:9b` / `gemma2:27b`:** Google's latest architecture. Exceptional reasoning, but heavily reliant on exact formatting and SWA. Validates strict parsing and caching edge cases.
*   **`deepseek-coder-v2`:** A code-centric model that relies heavily on strict JSON compliance for tool-calling.

## 2. Bypassing Sliding Window Attention (SWA) Cache Invalidation
When running local models that use SWA (like Qwen 3.5 or Gemma 4) via `llama.cpp` or Ollama on CPU-only hardware, the KV cache is often entirely invalidated between turns. 

Ollama's architecture hardcodes KV cache topology per model family, preventing you from passing `--disable-swa` or `--ctx-checkpoints` directly.

**Option A: Route to raw `llama.cpp server` (Recommended)**
Bypass Ollama and run `llama.cpp` directly for full control over context shifting:
```bash
./server \
  --model qwen3.6.Q4_K_M.gguf \
  --port 8080 \
  --ctx-size 8192 \
  --flash-attn \
  --no-mmap \
  --threads $(nproc) \
  --tensor-split 0.95,0.05
```
*   ZQK can then route to `http://localhost:8080/v1/chat/completions` (OpenAI-compatible).

**Option B: Stay in Ollama + Enforce Context Budgeting**
```bash
export OLLAMA_MAX_CONTEXT_LENGTH=8192
export OLLAMA_NUM_GPU_LAYERS=999        # Full VRAM offload if ≥24GB
export OLLAMA_FLASH_ATTENTION=1         # Speeds up attention computation
```
*   Pair this with strict Go-side context truncation.

## 3. Context Compression Strategy (Critical for Agents)
Never dump full MCP schemas into the system prompt. ZQK must implement dynamic, phased registration.
1.  **Disable CLI Tool Registration:** Ensure `register_cli_tools: false` in `.zqk/mcp/config.yaml`.
2.  **Schema Compression Rules:** Strip `examples`, `patternProperties`, and `additionalProperties` from JSONSchemas. Keep only `title`, `description`, `required`, and `properties.{type,enum}`. Agents parse descriptions fine; full JSONSchemas waste 60%+ of tool context.

## 4. Orchestrator Hardening
When rotating models through ZQK, monitor for these failure modes:
1.  **The "Schizophrenic" Tool Call:** Smaller models (7b/8b) often generate a valid tool call, but instead of stopping to wait for the orchestrator, they keep typing and hallucinate the tool's output themselves. ZQK's orchestrator must inject strict `stop` sequences.
2.  **JSON Mangling:** The orchestrator needs robust fallback logic (lenient JSON decoders or self-correction loops) when a model forgets closing braces `}`.
3.  **Adaptive Timeouts:** Local inference is slow. Implement adaptive timeout calculations based on estimated token limits and baseline tokens/sec, rather than static limits.
