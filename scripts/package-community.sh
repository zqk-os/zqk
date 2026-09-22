#!/bin/bash
# package-community.sh — Build cross-platform release binaries and create a GitHub release for Community Edition.
#
# Usage:
#   ./scripts/package-community.sh v2.7.0         # build + create GitHub release
#   ./scripts/package-community.sh v2.7.0 --dry   # build only, don't push to GitHub

set -euo pipefail

VERSION="${1:?Usage: $0 <version-tag> [--dry]}"
DRY_RUN="${2:-}"

# Validate semver tag format (e.g., v1.2.3, v0.1.0-alpha.1, v0.0.0-test)
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Error: Invalid version tag format '$VERSION'. Expected semver format 'vX.Y.Z' (e.g. v2.7.0, v0.0.0-test)" >&2
  exit 1
fi

# Strip leading 'v' for archive names
VER_NUM="${VERSION#v}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ZQK_DIST_DIR:-${REPO_ROOT}/dist-community}"
GIT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$REPO_ROOT" log -1 --pretty=%ct 2>/dev/null || date +%s)}"
export TOUCH_TS="$(date -u -r "$SOURCE_DATE_EPOCH" '+%Y%m%d%H%M.%S' 2>/dev/null || date -u '+%Y%m%d%H%M.%S')"
BUILD_DATE="$(date -u -r "$SOURCE_DATE_EPOCH" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u '+%Y-%m-%dT%H:%M:%SZ')"

# Public module is github.com/zqk-os/zqk; the community SKU ships ./cmd/zqk.
# Archive/binary name stays zqk-community so Homebrew formula + dist tests stay stable.
# private identity must not appear in packaging.
LDFLAGS="-s -w -buildid= \
  -X github.com/zqk-os/zqk/cmd/zqk/app.version=${VERSION} \
  -X github.com/zqk-os/zqk/cmd/zqk/app.buildDate=${BUILD_DATE} \
  -X github.com/zqk-os/zqk/cmd/zqk/app.gitCommit=${GIT_COMMIT}"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

BINARIES=(
  "zqk-community:./cmd/zqk"
)

echo "🔨 Building ZQK Community Edition ${VERSION} (commit ${GIT_COMMIT})"
echo ""

# Ensure bootstrap archive is built if script exists
if [ -f "${REPO_ROOT}/scripts/build-bootstrap-archive.sh" ]; then
  echo "  📦 Generating bootstrap archive..."
  bash "${REPO_ROOT}/scripts/build-bootstrap-archive.sh" "${REPO_ROOT}"
fi

export GH_TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  ARCHIVE_NAME="zqk-community_${VER_NUM}_${GOOS}_${GOARCH}"
  STAGE_DIR="${DIST_DIR}/${ARCHIVE_NAME}"
  mkdir -p "$STAGE_DIR"

  echo "  📦 ${GOOS}/${GOARCH}"

  for entry in "${BINARIES[@]}"; do
    BIN_NAME="${entry%:*}"
    BIN_PATH="${entry#*:}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
      go build -trimpath -ldflags "$LDFLAGS" -o "${STAGE_DIR}/${BIN_NAME}" "$BIN_PATH"
  done

  # Also provide "zqk" alias alongside "zqk-community" so unpacked archives have ./zqk immediately
  if [ -f "${STAGE_DIR}/zqk-community" ] && [ ! -f "${STAGE_DIR}/zqk" ]; then
    cp "${STAGE_DIR}/zqk-community" "${STAGE_DIR}/zqk"
  fi

  # Include essential files
  [ -f "${REPO_ROOT}/LICENSE" ] && cp "${REPO_ROOT}/LICENSE" "$STAGE_DIR/"
  [ -f "${REPO_ROOT}/README.md" ] && cp "${REPO_ROOT}/README.md" "$STAGE_DIR/"

  # Normalize file timestamps for reproducible builds
  TOUCH_TS="$(date -u -r "$SOURCE_DATE_EPOCH" '+%Y%m%d%H%M.%S' 2>/dev/null || date -u '+%Y%m%d%H%M.%S')"
  find "$STAGE_DIR" -exec touch -h -t "$TOUCH_TS" {} + 2>/dev/null || true

  # macOS Code Signing & Notarization Gate
  if [[ "$GOOS" == "darwin" && -f "${REPO_ROOT}/scripts/notarize-and-sign-darwin.sh" ]]; then
    echo "  🔏 Signing Darwin binaries in stage..."
    for entry in "${BINARIES[@]}"; do
      BIN_NAME="${entry%:*}"
      bash "${REPO_ROOT}/scripts/notarize-and-sign-darwin.sh" "${STAGE_DIR}/${BIN_NAME}"
    done
    find "$STAGE_DIR" -exec touch -h -t "$TOUCH_TS" {} + 2>/dev/null || true
  fi

  # Create archive
  (cd "$DIST_DIR" && find "$ARCHIVE_NAME" | sort | tar -cf - -T - | gzip -n > "${ARCHIVE_NAME}.tar.gz")
  # Also create standard zqk_ archive alias for installer and release parity
  (cd "$DIST_DIR" && cp "${ARCHIVE_NAME}.tar.gz" "zqk_${VER_NUM}_${GOOS}_${GOARCH}.tar.gz")
  if [[ "$GOOS" == "darwin" && -f "${REPO_ROOT}/scripts/notarize-and-sign-darwin.sh" ]]; then
    bash "${REPO_ROOT}/scripts/notarize-and-sign-darwin.sh" "${DIST_DIR}/${ARCHIVE_NAME}.tar.gz"
    bash "${REPO_ROOT}/scripts/notarize-and-sign-darwin.sh" --verify "${DIST_DIR}/${ARCHIVE_NAME}.tar.gz"
  fi
  rm -rf "$STAGE_DIR"
done

# Packaging Static Documentation Portal
if [ -f "${REPO_ROOT}/scripts/generate-docs-portal.sh" ]; then
  echo ""
  echo "  📚 Packaging static documentation portal..."
  bash "${REPO_ROOT}/scripts/generate-docs-portal.sh" "${DIST_DIR}/docs-portal"
  bash "${REPO_ROOT}/scripts/generate-docs-portal.sh" --verify "${DIST_DIR}/docs-portal.tar.gz"
fi

# Packaging Community Helm Chart
if [ -f "${REPO_ROOT}/scripts/package-helm-chart.sh" ]; then
  echo ""
  echo "  ☸️ Packaging Community Helm chart..."
  bash "${REPO_ROOT}/scripts/package-helm-chart.sh" "${VERSION}" "${DIST_DIR}"
  bash "${REPO_ROOT}/scripts/package-helm-chart.sh" --verify "${DIST_DIR}/zqk-community-${VER_NUM}.tgz"
fi

# Generating OpenVEX Security Attestation
if [ -f "${REPO_ROOT}/scripts/generate-openvex.sh" ]; then
  echo ""
  echo "  🛡️ Generating OpenVEX security attestation..."
  bash "${REPO_ROOT}/scripts/generate-openvex.sh" "${VERSION}" "${DIST_DIR}/openvex.json"
  bash "${REPO_ROOT}/scripts/generate-openvex.sh" --verify "${DIST_DIR}/openvex.json"
fi

# Packaging Multi-Language OpenAPI Client SDKs
if [ -f "${REPO_ROOT}/scripts/generate-openapi-clients.sh" ]; then
  echo ""
  echo "  📦 Generating multi-language OpenAPI client SDKs..."
  bash "${REPO_ROOT}/scripts/generate-openapi-clients.sh" "${DIST_DIR}/sdk"
  bash "${REPO_ROOT}/scripts/generate-openapi-clients.sh" --verify "${DIST_DIR}/sdk"
  if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
    find "${DIST_DIR}/sdk" -exec touch -h -t "$TOUCH_TS" {} + 2>/dev/null || true
  fi
  (cd "$DIST_DIR" && find sdk | sort | tar -cf - -T - | gzip -n > "zqk-client-sdks.tar.gz")
fi


# Generate checksums
echo ""
echo "  🔒 Generating checksums"
(cd "$DIST_DIR" && (shasum -a 256 *.tar.gz *.tgz openvex.json 2>/dev/null || shasum -a 256 *.tar.gz) > checksums.txt)

echo "  🔍 Verifying checksum manifest integrity"
(cd "$DIST_DIR" && shasum -a 256 -c checksums.txt)

# Generate Homebrew tap formula
echo ""
echo "  🍺 Generating Homebrew formula"
DARWIN_ARM64_SHA="$(grep "darwin_arm64.tar.gz" "${DIST_DIR}/checksums.txt" | awk '{print $1}')"
DARWIN_AMD64_SHA="$(grep "darwin_amd64.tar.gz" "${DIST_DIR}/checksums.txt" | awk '{print $1}')"
LINUX_ARM64_SHA="$(grep "linux_arm64.tar.gz" "${DIST_DIR}/checksums.txt" | awk '{print $1}')"
LINUX_AMD64_SHA="$(grep "linux_amd64.tar.gz" "${DIST_DIR}/checksums.txt" | awk '{print $1}')"

mkdir -p "${DIST_DIR}/Formula"
cat <<EOF > "${DIST_DIR}/Formula/zqk.rb"
class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "${VER_NUM}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_arm64.tar.gz"
      sha256 "${DARWIN_ARM64_SHA}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_amd64.tar.gz"
      sha256 "${DARWIN_AMD64_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_arm64.tar.gz"
      sha256 "${LINUX_ARM64_SHA}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_amd64.tar.gz"
      sha256 "${LINUX_AMD64_SHA}"
    end
  end

  def install
    bin.install "zqk-community" => "zqk"
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
EOF

# Synthesize release notes and changelog
echo ""
echo "  📝 Synthesizing release notes and changelog..."
CHANGELOG_FILE="${DIST_DIR}/CHANGELOG.md"
RELEASE_NOTES_FILE="${DIST_DIR}/RELEASE_NOTES.md"

cat <<EOF > "$CHANGELOG_FILE"
# ZQK Community Edition ${VERSION} ChangeLog

Release Date: ${BUILD_DATE}
Commit: ${GIT_COMMIT}

## Highlights
- Standalone clean-tree offline bootstrap and airgap operation
- Automated cross-platform binary distribution and SHA256 checksum manifests
- Strict local token masking and privacy-preserving zero-PII telemetry sanitization
- Developer Certificate of Origin (DCO) sign-off compliance and pre-commit linting gates
- Automated conventional changelog generation and artifact verification
EOF

cat <<EOF > "$RELEASE_NOTES_FILE"
## ZQK Community Edition ${VERSION}

The Zen Quantum Kernel Community Edition.

### Highlights
- Automated release artifact distribution and SHA256 checksum verification
- Multi-arch platform support (macOS Darwin arm64/amd64, Linux arm64/amd64)
- Zero-PII privacy-preserving telemetry guard
- Developer DCO sign-off compliance

### Install

**Homebrew (Recommended):**
\`\`\`bash
brew tap zqk-os/zqk
brew install zqk
\`\`\`

**Manual Checksum Verification:**
\`\`\`bash
curl -fsSL https://github.com/zqk-os/zqk/releases/download/community-${VERSION}/checksums.txt | sha256sum -c
\`\`\`
EOF

# Update repo-level Formula if in standard repository tree (and not a dry run)
if [ -d "${REPO_ROOT}/Formula" ] && [ "$DRY_RUN" != "--dry" ]; then
  cp "${DIST_DIR}/Formula/zqk.rb" "${REPO_ROOT}/Formula/zqk.rb"
fi

echo ""
echo "✅ Release artifacts in ${DIST_DIR}/"
ls -lh "$DIST_DIR/"
echo ""
cat "$DIST_DIR/checksums.txt"

# Create GitHub release (unless --dry)
if [ "$DRY_RUN" = "--dry" ]; then
  echo ""
  echo "🏁 Dry run complete. To create the release:"
  echo "   git tag community-${VERSION} && git push origin community-${VERSION}"
  echo "   gh release create community-${VERSION} dist-community/*.tar.gz dist-community/checksums.txt dist-community/Formula/zqk.rb --title 'Community Edition ${VERSION}' --notes-file -"
  exit 0
fi

HOLD_FILE="${REPO_ROOT}/.zqk/state/remote_hold.json"
if [ -f "$HOLD_FILE" ] && grep -q '"hold": true' "$HOLD_FILE"; then
  echo "BLOCK: remote_hold=true; refusing GitHub publish. Artifacts are in ${DIST_DIR}." >&2
  echo "Re-run with --dry, or wait for a human-written public_push_ack.json." >&2
  exit 1
fi

echo ""
echo "🚀 Creating GitHub release community-${VERSION}..."

# Tag if not already tagged
TAG_NAME="community-${VERSION}"
if ! git -C "$REPO_ROOT" rev-parse "$TAG_NAME" &>/dev/null; then
  git -C "$REPO_ROOT" tag "$TAG_NAME"
  git -C "$REPO_ROOT" push origin "$TAG_NAME"
fi

# Create release with gh CLI
gh release create "$TAG_NAME" \
  "${DIST_DIR}"/*.tar.gz \
  "${DIST_DIR}"/*.tgz \
  "${DIST_DIR}/openvex.json" \
  "${DIST_DIR}/checksums.txt" \
  "${DIST_DIR}/Formula/zqk.rb" \
  --title "ZQK Community Edition ${VERSION}" \
  --notes "## ZQK Community Edition ${VERSION}

The Zen Quantum Kernel Community Edition.

### Install

**Homebrew (Recommended):**
\`\`\`bash
brew tap zqk-os/zqk
brew install zqk
\`\`\`

**Manual:** Download the archive for your platform below and verify checksums:
\`\`\`bash
curl -fsSL https://github.com/zqk-os/zqk/releases/download/${TAG_NAME}/checksums.txt | sha256sum -c
\`\`\`

**Build from source:**
\`\`\`bash
git clone https://github.com/zqk-os/zqk && cd zqk && make
\`\`\`
"

echo ""
echo "✅ Release ${VERSION} published: https://github.com/zqk-os/zqk/releases/tag/${TAG_NAME}"
