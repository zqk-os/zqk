#!/bin/sh
# ZQK Installer — supports public release download, go install, and build-from-source.
#
# Public OSS install (no token required — once repo is public):
#   curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh
#
# Private / pre-release install (requires GITHUB_TOKEN):
#   export GITHUB_TOKEN="ghp_..."
#   curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh -s -- v2.7.0
#
# Build from source (Go 1.21+ required — no token, no binary release needed):
#   ZQK_INSTALL_METHOD=source ./scripts/install.sh
#
# go install (module must be public):
#   ZQK_INSTALL_METHOD=goinstall ./scripts/install.sh
set -e

VERSION="${1:-latest}"
REPO="${ZQK_REPO:-zqk-os/zqk}"
MODULE="${ZQK_MODULE:-github.com/zqk-os/zqk}"
INSTALL_DIR="${ZQK_INSTALL_DIR:-/usr/local/bin}"
# INSTALL_METHOD: auto | binary | goinstall | source
INSTALL_METHOD="${ZQK_INSTALL_METHOD:-auto}"

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

# ---------------------------------------------------------------------------
# 2. Resolve 'latest' version tag (unauthenticated for public repo)
# ---------------------------------------------------------------------------
resolve_latest() {
  if [ -n "$GITHUB_TOKEN" ]; then
    curl -sSL -H "Authorization: token $GITHUB_TOKEN" \
      -H "Accept: application/vnd.github.v3+json" \
      "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | grep '"tag_name"' | cut -d'"' -f4 | head -1
  else
    # Public unauthenticated endpoint
    curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | grep '"tag_name"' | cut -d'"' -f4 | head -1
  fi
}

# ---------------------------------------------------------------------------
# Helper: install binary from GitHub Releases (public or private)
# ---------------------------------------------------------------------------
install_binary() {
  local ver="$1"
  local ver_num="${ver#v}"
  local archive="zqk_${ver_num}_${OS}_${ARCH}.tar.gz"

  echo "📥 Downloading ZQK ${ver} (${OS}/${ARCH})..."

  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT

  if [ -n "$GITHUB_TOKEN" ]; then
    # Private repo: use asset API
    _download_private "$ver" "$archive" "${TMPDIR}/${archive}"
    _download_private "$ver" "checksums.txt" "${TMPDIR}/checksums.txt"
  else
    # Public repo: direct release asset URL
    local base_url="https://github.com/${REPO}/releases/download/${ver}"
    curl -sSLf "${base_url}/${archive}" -o "${TMPDIR}/${archive}" || {
      echo "Binary release not found for ${ver}. Try ZQK_INSTALL_METHOD=source." >&2; exit 1
    }
    curl -sSLf "${base_url}/checksums.txt" -o "${TMPDIR}/checksums.txt" 2>/dev/null || true
  fi

  # Verify checksum if available
  if [ -f "${TMPDIR}/checksums.txt" ]; then
    echo "🔒 Verifying checksum..."
    (
      cd "$TMPDIR"
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c checksums.txt --ignore-missing || {
          echo "❌ Checksum verification failed!" >&2
          exit 1
        }
      elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 -c checksums.txt --ignore-missing || {
          echo "❌ Checksum verification failed!" >&2
          exit 1
        }
      else
        echo "❌ Neither sha256sum nor shasum found; cannot verify checksum." >&2
        exit 1
      fi
    )
  fi

  echo "📦 Extracting..."
  tar -xzf "${TMPDIR}/${archive}" -C "$TMPDIR"
  local extract_dir="${TMPDIR}/zqk_${ver_num}_${OS}_${ARCH}"
  [ -d "$extract_dir" ] || extract_dir="${TMPDIR}"  # goreleaser flat layout fallback

  _place_binary "$extract_dir/zqk" "$extract_dir/zqk-mcp"
}

_download_private() {
  local ver="$1" asset_name="$2" out="$3"
  local asset_url
  asset_url=$(curl -sSL -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/${REPO}/releases/tags/${ver}" \
    | grep -A1 "\"name\": \"${asset_name}\"" | grep '"url"' | cut -d'"' -f4 | head -1)
  [ -n "$asset_url" ] || { echo "Asset ${asset_name} not found in release ${ver}" >&2; exit 1; }
  curl -sSL -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/octet-stream" "$asset_url" -o "$out"
}

# ---------------------------------------------------------------------------
# Helper: go install (requires public module or GONOSUMCHECK)
# ---------------------------------------------------------------------------
install_go() {
  local ver="$1"
  command -v go >/dev/null 2>&1 || { echo "Go toolchain not found. Install from https://go.dev/dl/" >&2; exit 1; }
  local pkg="${MODULE}/cmd/zqk@${ver}"
  echo "🔧 Installing via go install ${pkg}..."
  go install "$pkg"
  local gobin
  gobin="$(go env GOPATH)/bin"
  if [ -f "${gobin}/zqk" ]; then
    _place_binary "${gobin}/zqk" ""
  else
    echo "go install succeeded; zqk is in $(go env GOPATH)/bin — add it to your PATH." >&2
  fi
}

# ---------------------------------------------------------------------------
# Helper: build from source (< 2 min on modern hardware with Go installed)
# ---------------------------------------------------------------------------
install_source() {
  command -v go >/dev/null 2>&1 || { echo "Go toolchain not found. Install from https://go.dev/dl/" >&2; exit 1; }
  command -v git >/dev/null 2>&1 || { echo "git not found." >&2; exit 1; }
  local src_dir
  src_dir="$(mktemp -d)/zqk-src"
  echo "📦 Cloning ${REPO} (shallow)..."
  git clone --depth=1 "https://github.com/${REPO}.git" "$src_dir"
  echo "🔧 Building..."
  make -C "$src_dir" zqk
  if [ -f "${src_dir}/bin/zqk" ]; then
    _place_binary "${src_dir}/bin/zqk" ""
  elif [ -f "${src_dir}/zqk" ]; then
    _place_binary "${src_dir}/zqk" ""
  else
    echo "❌ Built binary not found in ${src_dir}/bin/zqk" >&2
    exit 1
  fi
  rm -rf "$src_dir"
}

# ---------------------------------------------------------------------------
# Helper: place binary into INSTALL_DIR
# ---------------------------------------------------------------------------
_place_binary() {
  local bin="$1" mcp_bin="$2"
  if [ ! -w "$INSTALL_DIR" ]; then
    echo "🔑 Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo install -m 755 "$bin" "${INSTALL_DIR}/zqk"
    [ -f "$mcp_bin" ] && sudo install -m 755 "$mcp_bin" "${INSTALL_DIR}/zqk-mcp"
  else
    echo "📂 Installing to ${INSTALL_DIR}..."
    install -m 755 "$bin" "${INSTALL_DIR}/zqk"
    [ -f "$mcp_bin" ] && install -m 755 "$mcp_bin" "${INSTALL_DIR}/zqk-mcp"
  fi
  # Remove macOS quarantine flag
  if [ "$OS" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
    if [ ! -w "$INSTALL_DIR" ]; then
      sudo xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk" 2>/dev/null || true
    else
      xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk" 2>/dev/null || true
    fi
  fi
}

# ---------------------------------------------------------------------------
# 3. Main dispatch
# ---------------------------------------------------------------------------
echo "🚀 ZQK Installer — method=${INSTALL_METHOD} version=${VERSION}"

case "$INSTALL_METHOD" in
  source)
    install_source
    ;;
  goinstall)
    [ "$VERSION" = "latest" ] && VERSION="latest"
    install_go "$VERSION"
    ;;
  binary)
    if [ "$VERSION" = "latest" ]; then
      VERSION="$(resolve_latest)"
      [ -n "$VERSION" ] || { echo "Could not resolve latest version. Specify a version: ./install.sh v2.7.0" >&2; exit 1; }
    fi
    install_binary "$VERSION"
    ;;
  auto|*)
    # Auto: try binary (public) → go install → source
    if [ "$VERSION" = "latest" ]; then
      VERSION="$(resolve_latest 2>/dev/null)" || true
    fi
    if [ -n "$VERSION" ] && [ "$VERSION" != "latest" ]; then
      install_binary "$VERSION" 2>/dev/null && DONE=1 || true
    fi
    if [ -z "$DONE" ]; then
      echo "Binary release not available — trying go install..."
      install_go "latest" 2>/dev/null && DONE=1 || true
    fi
    if [ -z "$DONE" ]; then
      echo "go install not available — building from source (requires git + Go)..."
      install_source
    fi
    ;;
esac

# ---------------------------------------------------------------------------
# 4. Post-install verification and quick-start hint
# ---------------------------------------------------------------------------
if command -v zqk >/dev/null 2>&1 || [ -f "${INSTALL_DIR}/zqk" ]; then
  ZQK_BIN="${INSTALL_DIR}/zqk"
  ZQK_VER="$("$ZQK_BIN" version 2>/dev/null || echo 'installed')"
  echo ""
  echo "✅ ZQK ${ZQK_VER} installed to ${INSTALL_DIR}/zqk"
  echo ""
  echo "Quick start (< 2 min):"
  echo "  mkdir my-project && cd my-project"
  echo "  zqk system init --project-name my-project"
  echo "  zqk workflow whats-next          # discover mission + next tasks"
  echo "  zqk mcp proxy --tcp 127.0.0.1:7777 # expose MCP securely on loopback"
  echo ""
  echo "Docs: https://github.com/${REPO}#readme"
fi
