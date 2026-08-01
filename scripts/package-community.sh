#!/bin/bash
# package-community.sh — Build cross-platform release binaries and create a GitHub release for Community Edition.
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
DIST_DIR="${REPO_ROOT}/dist-community"
GIT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"
BUILD_DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

LDFLAGS="-s -w \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.version=${VERSION} \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.buildDate=${BUILD_DATE} \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.gitCommit=${GIT_COMMIT}"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

# Ship as "zqk" — enterprise/admin binaries use distinguishing names.
BINARIES=(
  "zqk:./cmd/zqk"
)

echo "🔨 Building ZQK ${VERSION} (commit ${GIT_COMMIT})"
echo ""

# Portable binary contract: embedded bootstrap must extract to a temp project.
# Never initialize this source repo as a ZQK project root.
if [ ! -f "${REPO_ROOT}/internal/bootstrap/archive/bootstrap.tar.gz" ]; then
  echo "Error: missing embedded bootstrap archive (internal/bootstrap/archive/bootstrap.tar.gz)" >&2
  exit 1
fi
(
  cd "$REPO_ROOT"
  go test ./internal/bootstrap -count=1 -timeout 60s \
    -run 'TestManifestPaths_EmbeddedArchive|TestExtractEmbeddedToTempProject'
)

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
  echo "   gh release create community-${VERSION} dist-community/*.tar.gz dist-community/checksums.txt --title 'Community Edition ${VERSION}' --notes-file -"
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
  --title "ZQK Community Edition ${VERSION}" \
  --notes "## ZQK Community Edition ${VERSION}

The Zen Quantum Kernel Community Edition.

### Install

**Manual:** Download the archive for your platform below.

**Build from source:**
\`\`\`bash
git clone https://github.com/lanceman/zqk && cd zqk && make zqk-community
\`\`\`
"

echo ""
echo "✅ Release ${VERSION} published: https://github.com/lanceman/zqk/releases/tag/${TAG_NAME}"
