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

# ---------------------------------------------------------------------------
# 2. Resolve 'latest' version tag (unauthenticated for public repo)
# ---------------------------------------------------------------------------
resolve_latest() {
  local tag=""
  if [ -n "$GITHUB_TOKEN" ]; then
    tag=$(curl -sSL -H "Authorization: Bearer $GITHUB_TOKEN" \
      -H "Accept: application/vnd.github.v3+json" \
      "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | grep '"tag_name"' | cut -d'"' -f4 | head -1)
    if [ -z "$tag" ]; then
      tag=$(curl -sSL -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "Accept: application/vnd.github.v3+json" \
        "https://api.github.com/repos/${REPO}/releases" 2>/dev/null \
        | grep '"tag_name"' | cut -d'"' -f4 | head -1)
    fi
  else
    tag=$(curl -sSL -H "Accept: application/vnd.github.v3+json" \
      "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | grep '"tag_name"' | cut -d'"' -f4 | head -1)
    if [ -z "$tag" ]; then
      tag=$(curl -sSL -H "Accept: application/vnd.github.v3+json" \
        "https://api.github.com/repos/${REPO}/releases" 2>/dev/null \
        | grep '"tag_name"' | cut -d'"' -f4 | head -1)
    fi
  fi
  printf '%s' "$tag"
}

# Verify exactly one archive against checksums.txt. Multi-platform manifests list
# files we did not download; do not skip unmatched checksum lines.
_verify_archive_sha256() {
  local sums="$1"
  local archive_path="$2"
  local name
  name=$(basename "$archive_path")
  local expected
  expected=$(awk -v n="$name" '
    {
      f=$NF
      sub(/^\*/, "", f)
      if (f == n) { print $1; found=1; exit }
    }
    END { if (!found) exit 1 }
  ' "$sums") || {
    echo "No SHA256 for ${name} in checksums.txt" >&2
    exit 1
  }
  local actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive_path" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive_path" | awk '{print $1}')
  else
    echo "Neither sha256sum nor shasum found; cannot verify checksum." >&2
    exit 1
  fi
  expected=$(printf '%s' "$expected" | tr 'A-F' 'a-f')
  actual=$(printf '%s' "$actual" | tr 'A-F' 'a-f')
  if [ "$expected" != "$actual" ]; then
    echo "Checksum mismatch for ${name}" >&2
    exit 1
  fi
}

# Verify checksums.txt signature (checksums.txt.sig) using cosign if available.
_verify_checksums_signature() {
  local checksums_path="$1"
  local sig_path="${checksums_path}.sig" # verifies checksums.txt.sig
  if [ ! -f "$sig_path" ]; then
    return 0
  fi
  if command -v cosign >/dev/null 2>&1; then
    echo "🔒 Verifying checksums signature with cosign..."
    if ! cosign verify-blob --signature "$sig_path" "$checksums_path" >/dev/null 2>&1; then
      echo "Warning: cosign signature verification failed for checksums.txt (checksums.txt.sig)" >&2
    fi
  fi
}

# ---------------------------------------------------------------------------
# Helper: install binary from GitHub Releases (public or private)
# ---------------------------------------------------------------------------
install_binary() {
  local ver="$1"
  local ver_num="${ver#v}"
  local archive="zqk_${ver_num}_${OS}_${ARCH}.tar.gz"
  local comm_archive="zqk-community_${ver_num}_${OS}_${ARCH}.tar.gz"

  echo "📥 Downloading ZQK ${ver} (${OS}/${ARCH})..."

  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT

  local archive_path="${TMPDIR}/${archive}"
  local checksums_path="${TMPDIR}/checksums.txt"

  local base_url="https://github.com/${REPO}/releases/download/${ver}"
  if curl -sSLf "${base_url}/${archive}" -o "${archive_path}" 2>/dev/null && \
     curl -sSLf "${base_url}/checksums.txt" -o "${checksums_path}" 2>/dev/null; then
    : # downloaded from public release URL
  elif curl -sSLf "${base_url}/${comm_archive}" -o "${archive_path}" 2>/dev/null && \
     curl -sSLf "${base_url}/checksums.txt" -o "${checksums_path}" 2>/dev/null; then
    : # downloaded community-prefixed archive from public release URL
  elif [ -n "$GITHUB_TOKEN" ] || command -v gh >/dev/null 2>&1; then
    if ! _download_private "$ver" "$archive" "${archive_path}"; then
      if ! _download_private "$ver" "$comm_archive" "${archive_path}"; then
        echo "Neither ${archive} nor ${comm_archive} found in release ${ver}" >&2
        exit 1
      fi
    fi
    _download_private "$ver" "checksums.txt" "${checksums_path}" || {
      echo "Checksums file not found in release ${ver}" >&2
      exit 1
    }
  else
    echo "Binary release not found for ${ver}. Try ZQK_INSTALL_METHOD=source or set GITHUB_TOKEN." >&2
    exit 1
  fi

  echo "🔒 Verifying checksum..."
  _verify_archive_sha256 "${checksums_path}" "${archive_path}"
  _verify_checksums_signature "${checksums_path}"

  echo "📦 Extracting..."
  tar -xzf "${archive_path}" -C "$TMPDIR"
  local extract_dir="${TMPDIR}/zqk_${ver_num}_${OS}_${ARCH}"
  [ -d "$extract_dir" ] || extract_dir="${TMPDIR}/zqk-community_${ver_num}_${OS}_${ARCH}"
  [ -d "$extract_dir" ] || extract_dir="${TMPDIR}"  # goreleaser flat layout fallback

  if [ -f "$extract_dir/zqk-community" ] && [ ! -f "$extract_dir/zqk" ]; then
    cp "$extract_dir/zqk-community" "$extract_dir/zqk"
  fi

  _place_binary "$extract_dir/zqk" "$extract_dir/zqk-mcp"
}

_download_private() {
  local ver="$1" asset_name="$2" out="$3"

  # Fast path: use gh CLI if available
  if command -v gh >/dev/null 2>&1; then
    if gh release download "$ver" -p "$asset_name" --output "$out" --repo "$REPO" --clobber >/dev/null 2>&1; then
      return 0
    fi
  fi

  # Fallback: GitHub Releases REST API
  if [ -n "$GITHUB_TOKEN" ]; then
    local token
    token="$(printf '%s' "$GITHUB_TOKEN" | tr -d '\r\n')"
    local release_json
    release_json=$(curl -sSL -H "Authorization: Bearer $token" \
      -H "Accept: application/vnd.github.v3+json" \
      "https://api.github.com/repos/${REPO}/releases/tags/${ver}" 2>/dev/null)

    local asset_url=""
    if command -v python3 >/dev/null 2>&1; then
      asset_url=$(printf '%s' "$release_json" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    for a in d.get("assets", []):
        if a.get("name") == sys.argv[1]:
            print(a.get("url", ""))
            break
except Exception:
    pass
' "$asset_name" 2>/dev/null || true)
    fi

    if [ -z "$asset_url" ]; then
      asset_url=$(printf '%s' "$release_json" | awk -v name="$asset_name" '
        BEGIN { RS="{"; FS="," }
        $0 ~ ("\"name\":[ ]*\"" name "\"") {
          for (i=1; i<=NF; i++) {
            if ($i ~ /"url":/) {
              gsub(/.*"url":[ ]*"/, "", $i)
              gsub(/".*/, "", $i)
              print $i
              exit
            }
          }
        }
      ')
    fi

    if [ -n "$asset_url" ]; then
      if curl -sSL -H "Authorization: Bearer $token" \
        -H "Accept: application/octet-stream" "$asset_url" -o "$out"; then
        return 0
      fi
    fi
  fi

  return 1
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
