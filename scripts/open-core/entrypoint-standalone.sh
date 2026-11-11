#!/usr/bin/env bash
set -euo pipefail

# Standalone Entrypoint: Manages embedded local inference + ZQK swarms

# Boundary 1: Rejection of root UID 0
EFFECTIVE_UID="${ZQK_UID_OVERRIDE:-$(id -u)}"
if [ "${EFFECTIVE_UID}" -eq 0 ]; then
  echo "FATAL: [STANDALONE] Container execution as root (UID 0) is prohibited for security compliance." >&2
  exit 1
fi

# Boundary 2: Rejection of Sub-6GB RAM allocation
check_memory_floor() {
  local mem_mb=""
  if [ -n "${ZQK_CONTAINER_MEM_MB:-}" ]; then
    mem_mb="${ZQK_CONTAINER_MEM_MB}"
  elif [ -f /sys/fs/cgroup/memory.max ]; then
    local max_bytes
    max_bytes=$(cat /sys/fs/cgroup/memory.max 2>/dev/null || true)
    if [ -n "${max_bytes}" ] && [ "${max_bytes}" != "max" ]; then
      mem_mb=$((max_bytes / 1024 / 1024))
    fi
  fi

  if [ -z "${mem_mb}" ] && [ -f /proc/meminfo ]; then
    local total_kb
    total_kb=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || true)
    if [ -n "${total_kb}" ]; then
      mem_mb=$((total_kb / 1024))
    fi
  fi

  if [ -n "${mem_mb}" ] && [ "${mem_mb}" -lt 6144 ]; then
    echo "FATAL: [STANDALONE] Insufficient host RAM allocated (${mem_mb}MB detected). Turnkey local inference requires a minimum of 6144MB (6GB) RAM." >&2
    exit 1
  fi
}

check_memory_floor

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
