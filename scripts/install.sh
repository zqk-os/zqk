#!/bin/sh
# ZQK Installer — supports Go Fast (token-budgeted agent search) and Walk Through (interactive kernel onboarding).
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh
#   curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh -s -- --fast
#   curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh -s -- --walkthrough
set -e

REPO="${ZQK_REPO:-zqk-os/zqk}"
MODULE="${ZQK_MODULE:-github.com/zqk-os/zqk}"
INSTALL_DIR="${ZQK_INSTALL_DIR:-/usr/local/bin}"
# INSTALL_METHOD: auto | binary | goinstall | source
INSTALL_METHOD="${ZQK_INSTALL_METHOD:-auto}"

MODE="${ZQK_MODE:-}"
VERSION="latest"

for arg in "$@"; do
  case "$arg" in
    --fast|-f)
      MODE="fast"
      ;;
    --walkthrough|-w)
      MODE="walkthrough"
      ;;
    v*|latest)
      VERSION="$arg"
      ;;
    *)
      if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ]; then
        VERSION="$arg"
      fi
      ;;
  esac
done

# If mode was not explicitly supplied, prompt when interactive, default to fast when headless
if [ -z "$MODE" ]; then
  if [ -t 0 ] && [ -t 1 ]; then
    echo "========================================================================"
    echo "⚡ Zen Quantum Kernel (ZQK)"
    echo "========================================================================"
    echo ""
    echo "Choose your path:"
    echo ""
    echo "  [1] Go Fast"
    echo "      • Installs zqk and links zgrep into PATH"
    echo "      • Cuts AI agent search token waste by 98.8% in Cursor/Claude/Cline"
    echo "      • Zero setup (< 5 seconds)"
    echo ""
    echo "  [2] Walk Through"
    echo "      • Interactive walkthrough of the Knowledge Kernel"
    echo "      • Explains the 5-layer cascade & fail-closed done-gates"
    echo "      • Connects your project to verifiable graph memory"
    echo ""
    printf "Select [1] or [2] (default: 1): "
    read -r choice < /dev/tty || choice="1"
    case "$choice" in
      2) MODE="walkthrough" ;;
      *) MODE="fast" ;;
    esac
    echo ""
  else
    MODE="fast"
  fi
fi

# Auto-detect GITHUB_TOKEN via gh CLI if not explicitly set
if [ -z "$GITHUB_TOKEN" ] && command -v gh >/dev/null 2>&1; then
  GITHUB_TOKEN="$(gh auth token 2>/dev/null || true)"
fi
GITHUB_TOKEN="$(printf '%s' "$GITHUB_TOKEN" | tr -d '\r\n')"

# ---------------------------------------------------------------------------
# 1. Detect OS and Architecture
# ---------------------------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
if [ "$OS" != "darwin" ] && [ "$OS" != "linux" ]; then
  echo "Unsupported OS: $OS" >&2; exit 1
fi

# Fallback install directory if /usr/local/bin is not writable and not running as root
if [ ! -w "$INSTALL_DIR" ] && [ "$(id -u)" != "0" ] && [ -z "$ZQK_INSTALL_DIR" ]; then
  if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

# ---------------------------------------------------------------------------
# 2. Resolve 'latest' version tag
# ---------------------------------------------------------------------------
resolve_latest() {
  local tag=""
  local token_hdr=""
  if [ -n "$GITHUB_TOKEN" ]; then
    token_hdr="Authorization: Bearer $GITHUB_TOKEN"
  fi

  tag=$(curl -sSL ${token_hdr:+-H "$token_hdr"} \
    -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' | cut -d'"' -f4 | head -1)

  if [ -z "$tag" ]; then
    tag=$(curl -sSL ${token_hdr:+-H "$token_hdr"} \
      -H "Accept: application/vnd.github.v3+json" \
      "https://api.github.com/repos/${REPO}/releases" 2>/dev/null \
      | grep '"tag_name"' | cut -d'"' -f4 | head -1)
  fi
  printf '%s' "$tag"
}

# ---------------------------------------------------------------------------
# 3. Installation Methods
# ---------------------------------------------------------------------------
install_local() {
  if [ -x "./bin/zqk" ]; then
    echo "  • Found local build at ./bin/zqk"
    mkdir -p "$INSTALL_DIR" 2>/dev/null || true
    ln -sf "$(pwd)/bin/zqk" "${INSTALL_DIR}/zqk" 2>/dev/null || cp "./bin/zqk" "${INSTALL_DIR}/zqk"
    ln -sf "${INSTALL_DIR}/zqk" "${INSTALL_DIR}/zgrep" 2>/dev/null || cp "./bin/zqk" "${INSTALL_DIR}/zgrep"
    return 0
  fi
  return 1
}

install_binary() {
  local ver="$1"
  local archive_name="zqk_${OS}_${ARCH}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/${ver}/${archive_name}"

  echo "  • Fetching release ${ver} for ${OS}/${ARCH}..."
  local tmp_dir
  tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t 'zqk_install')
  trap 'rm -rf "$tmp_dir"' EXIT INT TERM

  local auth_header=""
  if [ -n "$GITHUB_TOKEN" ]; then
    auth_header="Authorization: Bearer $GITHUB_TOKEN"
  fi

  if ! curl -sSL -f ${auth_header:+-H "$auth_header"} -o "${tmp_dir}/${archive_name}" "$url"; then
    echo "  • Release archive not found at ${url}"
    return 1
  fi

  tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"
  mkdir -p "$INSTALL_DIR" 2>/dev/null || true

  if [ -f "${tmp_dir}/zqk" ]; then
    if [ ! -w "$INSTALL_DIR" ]; then
      sudo install -m 755 "${tmp_dir}/zqk" "${INSTALL_DIR}/zqk"
      sudo ln -sf "${INSTALL_DIR}/zqk" "${INSTALL_DIR}/zgrep" 2>/dev/null || true
    else
      install -m 755 "${tmp_dir}/zqk" "${INSTALL_DIR}/zqk"
      ln -sf "${INSTALL_DIR}/zqk" "${INSTALL_DIR}/zgrep" 2>/dev/null || true
    fi
    return 0
  fi
  return 1
}

install_go() {
  local ver="$1"
  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi
  echo "  • Installing via go install (${MODULE}@${ver})..."
  mkdir -p "$INSTALL_DIR" 2>/dev/null || true
  GOBIN="$INSTALL_DIR" go install "${MODULE}/cmd/zqk@${ver}"
  ln -sf "${INSTALL_DIR}/zqk" "${INSTALL_DIR}/zgrep" 2>/dev/null || true
  return 0
}

install_source() {
  if ! command -v git >/dev/null 2>&1 || ! command -v go >/dev/null 2>&1; then
    echo "Building from source requires git and Go." >&2
    return 1
  fi
  local tmp_dir
  tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t 'zqk_src')
  trap 'rm -rf "$tmp_dir"' EXIT INT TERM

  echo "  • Cloning repository from https://github.com/${REPO}..."
  git clone --depth 1 "https://github.com/${REPO}.git" "$tmp_dir"
  (
    cd "$tmp_dir"
    mkdir -p "$INSTALL_DIR" 2>/dev/null || true
    make
    cp bin/zqk "${INSTALL_DIR}/zqk"
    ln -sf "${INSTALL_DIR}/zqk" "${INSTALL_DIR}/zgrep" 2>/dev/null || true
  )
  return 0
}

# ---------------------------------------------------------------------------
# 4. Main Dispatch
# ---------------------------------------------------------------------------
echo "🚀 ZQK Installer — mode=${MODE} version=${VERSION}"

DONE=""
# First check if local binary already exists in working tree
if install_local 2>/dev/null; then
  DONE=1
fi

if [ -z "$DONE" ] && [ "$INSTALL_METHOD" != "source" ] && [ "$INSTALL_METHOD" != "goinstall" ]; then
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(resolve_latest 2>/dev/null)" || true
  fi
  if [ -n "$VERSION" ] && [ "$VERSION" != "latest" ]; then
    install_binary "$VERSION" 2>/dev/null && DONE=1 || true
  fi
fi

if [ -z "$DONE" ] && [ "$INSTALL_METHOD" != "source" ]; then
  install_go "${VERSION:-latest}" 2>/dev/null && DONE=1 || true
fi

if [ -z "$DONE" ]; then
  install_source && DONE=1 || true
fi

# ---------------------------------------------------------------------------
# 5. Post-Install Guidance (Go Fast vs Walk Through)
# ---------------------------------------------------------------------------
if command -v zqk >/dev/null 2>&1 || [ -f "${INSTALL_DIR}/zqk" ]; then
  ZQK_BIN="${INSTALL_DIR}/zqk"
  if [ ! -x "$ZQK_BIN" ] && command -v zqk >/dev/null 2>&1; then
    ZQK_BIN="$(command -v zqk)"
  fi
  
  # Ensure zgrep symlink exists
  ln -sf "$ZQK_BIN" "${INSTALL_DIR}/zgrep" 2>/dev/null || true
  ZQK_VER="$("$ZQK_BIN" version 2>/dev/null || echo 'installed')"

  echo ""
  echo "✅ ZQK ${ZQK_VER} installed to ${INSTALL_DIR}/zqk"
  echo "✅ zgrep linked to ${INSTALL_DIR}/zgrep"
  echo ""

  if [ "$MODE" = "fast" ]; then
    echo "========================================================================"
    echo "⚡ Go Fast — Drop into your agent rules (.cursorrules, CLAUDE.md, .clinerules):"
    echo "========================================================================"
    echo "  • Search:  zgrep \"<query>\" --max-tokens 500 -f json"
    echo "  • Go AST:  zgrep --ast --kind struct|func \"<name>\""
    echo "  • Reindex: zgrep --reindex"
    echo ""
    echo "Ready to explore the kernel later? Run: zqk system start-here"
    echo "Docs: https://github.com/${REPO}#readme"
    echo "========================================================================"
  else
    echo "========================================================================"
    echo "🧭 Walk Through — Launching Knowledge Kernel Walkthrough..."
    echo "========================================================================"
    "$ZQK_BIN" system start-here || true
  fi
else
  echo "❌ Installation could not be completed." >&2
  exit 1
fi
