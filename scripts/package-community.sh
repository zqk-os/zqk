#!/bin/bash
# package-community.sh — Build cross-platform release binaries and create a GitHub release for Community Edition.
# TRACK: TDE-1789699310016987000-5703b344 — homepage/URLs still github.com/zqk-os/zqk until zqk-os module move.
#
# Usage:
#   ./scripts/package-community.sh v2.7.0         # build + create GitHub release
#   ./scripts/package-community.sh v2.7.0 --dry   # build only, don't push to GitHub

set -euo pipefail

VERSION="${1:?Usage: $0 <version-tag> [--dry]}"
DRY_RUN="${2:-}"

# Strip leading 'v' for archive names
VER_NUM="${VERSION#v}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ZQK_DIST_DIR:-${REPO_ROOT}/dist-community}"
GIT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"
BUILD_DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

LDFLAGS="-s -w \
  -X github.com/zqk-os/zqk/cmd/zqk-community/app.version=${VERSION} \
  -X github.com/zqk-os/zqk/cmd/zqk-community/app.buildDate=${BUILD_DATE} \
  -X github.com/zqk-os/zqk/cmd/zqk-community/app.gitCommit=${GIT_COMMIT}"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

BINARIES=(
  "zqk:./cmd/zqk-community"
)

echo "🔨 Building ZQK Community Edition ${VERSION} (commit ${GIT_COMMIT})"
echo ""

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  ARCHIVE_NAME="zqk_${VER_NUM}_${GOOS}_${GOARCH}"
  STAGE_DIR="${DIST_DIR}/${ARCHIVE_NAME}"
  mkdir -p "$STAGE_DIR"

  echo "  📦 ${GOOS}/${GOARCH}"

  for entry in "${BINARIES[@]}"; do
    BIN_NAME="${entry%:*}"
    BIN_PATH="${entry#*:}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
      go build -ldflags "$LDFLAGS" -o "${STAGE_DIR}/${BIN_NAME}" "$BIN_PATH"
  done

  # Include essential files
  [ -f "${REPO_ROOT}/LICENSE" ] && cp "${REPO_ROOT}/LICENSE" "$STAGE_DIR/"
  [ -f "${REPO_ROOT}/README.md" ] && cp "${REPO_ROOT}/README.md" "$STAGE_DIR/"

  # Create archive
  (cd "$DIST_DIR" && tar -czf "${ARCHIVE_NAME}.tar.gz" "$ARCHIVE_NAME")
  rm -rf "$STAGE_DIR"
done

# Generate checksums
echo ""
echo "  🔒 Generating checksums"
(cd "$DIST_DIR" && shasum -a 256 *.tar.gz > checksums.txt)

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
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_arm64.tar.gz"
      sha256 "${DARWIN_ARM64_SHA}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_amd64.tar.gz"
      sha256 "${DARWIN_AMD64_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_arm64.tar.gz"
      sha256 "${LINUX_ARM64_SHA}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_amd64.tar.gz"
      sha256 "${LINUX_AMD64_SHA}"
    end
  end

  def install
    bin.install "zqk"
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
EOF

# Update repo-level Formula if in standard repository tree
if [ -d "${REPO_ROOT}/Formula" ]; then
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
  "${DIST_DIR}/checksums.txt" \
  "${DIST_DIR}/Formula/zqk.rb" \
  --title "ZQK Community Edition ${VERSION}" \
  --notes "## ZQK Community Edition ${VERSION}

The Zen Quantum Kernel Community Edition.

### Install

**Homebrew (Recommended):**
\`\`\`bash
brew tap lanceman/zqk
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
