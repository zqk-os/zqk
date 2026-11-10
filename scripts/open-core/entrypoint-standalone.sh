#!/usr/bin/env bash
set -euo pipefail

# Standalone Entrypoint: Manages embedded local inference + ZQK swarms

if [ -f "${MODEL_PATH:-}" ]; then
  echo "🚀 [STANDALONE] Starting embedded llama-server with model: ${MODEL_PATH}"
  llama-server \
    --model "${MODEL_PATH}" \
    --host 127.0.0.1 \
    --port 8080 \
    --ctx-size 4096 \
    --threads "$(nproc)" \
    --log-disable \
    &
  LLAMA_PID=$!
  trap 'kill -TERM $LLAMA_PID 2>/dev/null || true' EXIT

  # Wait for local inference health
  for i in {1..30}; do
    if curl -s http://127.0.0.1:8080/health >/dev/null 2>&1; then
      echo "✓ [STANDALONE] Local inference server online."
      break
    fi
    sleep 1
  done
else
  echo "ℹ [STANDALONE] No local GGUF model detected at ${MODEL_PATH:-/models/model.gguf}."
  echo "  Falling back to configured external/environment provider."
fi

# Execute requested zqk command
ZQK_BIN="${ZQK_BIN:-/usr/local/bin/zqk}"
exec "${ZQK_BIN}" "$@"
