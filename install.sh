#!/bin/sh
# ZQK (zqk.dev) Official POSIX Shell Installer
# Usage: curl -sSL https://zqk.dev/install.sh | sh

set -e

# --- Configuration & Defaults ---
REPO_OWNER="zqk-os"
REPO_NAME="zqk"
BINARY_NAME="zqk"
INSTALL_DIR="${ZQK_INSTALL_DIR:-/usr/local/bin}"
GITHUB_RELEASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"

# Styling output
BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}==>${NC} ${BOLD}%s${NC}\n" "$1"
}

success() {
    printf "${GREEN}==>${NC} ${BOLD}%s${NC}\n" "$1"
}

warn() {
    printf "${YELLOW}WARNING:${NC} %s\n" "$1"
}

error() {
    printf "${RED}ERROR:${NC} %s\n" "$1" >&2
    exit 1
}

# --- Utility Checks ---
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

detect_downloader() {
    if command_exists curl; then
        DOWNLOADER="curl"
    elif command_exists wget; then
        DOWNLOADER="wget"
    else
        error "Either 'curl' or 'wget' is required to run this installer script."
    fi
}

download_file() {
    url="$1"
    destination="$2"
    if [ "$DOWNLOADER" = "curl" ]; then
        curl -sSL -o "$destination" "$url"
    elif [ "$DOWNLOADER" = "wget" ]; then
        wget -q -O "$destination" "$url"
    fi
}

download_stdout() {
    url="$1"
    if [ "$DOWNLOADER" = "curl" ]; then
        curl -sSL "$url"
    elif [ "$DOWNLOADER" = "wget" ]; then
        wget -qO- "$url"
    fi
}

# --- System Detection ---
detect_os() {
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported operating system: $os. ZQK binaries are provided for Linux and macOS."
            ;;
    esac
}

detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $arch. ZQK binaries are compiled for x86_64 and aarch64 (ARM64)."
            ;;
    esac
}

# --- Version Resolution ---
get_latest_version() {
    if [ -n "$ZQK_VERSION" ]; then
        VERSION="$ZQK_VERSION"
        return
    fi

    info "Fetching latest release version from GitHub..."
    latest_tag=$(download_stdout "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$latest_tag" ]; then
        # Fallback if GitHub API rate limit is exceeded
        latest_tag=$(download_stdout "${GITHUB_RELEASE_URL}/latest" | \
            grep -o 'tag/[^"]*' | \
            head -n 1 | \
            cut -d'/' -f2)
    fi

    if [ -z "$latest_tag" ]; then
        error "Failed to resolve latest release version from GitHub."
    fi

    VERSION="$latest_tag"
}

# --- Checksum Verification ---
verify_checksum() {
    tarball="$1"
    checksums_file="$2"

    if command_exists sha256sum; then
        (cd "$(dirname "$tarball")" && sha256sum -c "$checksums_file" --ignore-missing >/dev/null 2>&1)
    elif command_exists shasum; then
        (cd "$(dirname "$tarball")" && shasum -a 256 -c "$checksums_file" --ignore-missing >/dev/null 2>&1)
    else
        warn "Neither 'sha256sum' nor 'shasum' found. Skipping binary integrity check."
    fi
}

# --- Main Installation Routine ---
main() {
    info "Installing ZQK Microkernel CLI..."

    detect_downloader
    detect_os
    detect_arch
    get_latest_version

    # Strip leading 'v' for the binary archive filename (e.g., v0.1.0-alpha.1 -> 0.1.0-alpha.1)
    VERSION_NO_V="${VERSION#v}"

    # Match GoReleaser output naming conventions
    TARBALL_NAME="${BINARY_NAME}_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="${GITHUB_RELEASE_URL}/download/${VERSION}/${TARBALL_NAME}"
    CHECKSUMS_URL="${GITHUB_RELEASE_URL}/download/${VERSION}/${BINARY_NAME}_${VERSION_NO_V}_checksums.txt"

    TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'zqk_install')"
    trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

    info "Target Platform: ${OS}-${ARCH}"
    info "Version:         ${VERSION}"
    info "Downloading binary archive..."

    download_file "$DOWNLOAD_URL" "${TMP_DIR}/${TARBALL_NAME}" || error "Failed to download release archive from ${DOWNLOAD_URL}"
    
    # Attempt optional checksum download and verification
    if download_file "$CHECKSUMS_URL" "${TMP_DIR}/checksums.txt" 2>/dev/null; then
        info "Verifying SHA256 integrity..."
        verify_checksum "${TMP_DIR}/${TARBALL_NAME}" "${TMP_DIR}/checksums.txt"
    fi

    info "Extracting payload..."
    tar -xzf "${TMP_DIR}/${TARBALL_NAME}" -C "$TMP_DIR"

    # Handle directory write permissions
    if [ ! -d "$INSTALL_DIR" ]; then
        if [ -w "$(dirname "$INSTALL_DIR")" ]; then
            mkdir -p "$INSTALL_DIR"
        else
            info "Requesting superuser privileges to create directory ${INSTALL_DIR}..."
            sudo mkdir -p "$INSTALL_DIR"
        fi
    fi

    info "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        info "Requesting superuser privileges to install binary to ${INSTALL_DIR}..."
        sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    success "ZQK ${VERSION} successfully installed to ${INSTALL_DIR}/${BINARY_NAME}!"

    # Path Verification Warning
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            warn "${INSTALL_DIR} is not currently in your system PATH."
            warn "Add it to your profile with: export PATH=\"\$PATH:${INSTALL_DIR}\""
            ;;
    esac

    printf "\nRun '${BOLD}zqk --help${NC}' to get started.\n"
}

main "$@"