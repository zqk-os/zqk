#!/bin/bash
# package-community.sh — local zcom archives only.
# Public GitHub/Homebrew publish is held. This script never pushes.
# TRACK: TDE-1789681135032251000-e52ad7a7
set -euo pipefail

VERSION="${1:?Usage: $0 <version-tag> [--dry]}"
DRY_RUN="${2:-}"

VER_NUM="${VERSION#v}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ZQK_DIST_DIR:-${REPO_ROOT}/dist-community}"
GIT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"
BUILD_DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

LDFLAGS="-s -w \
  -X github.com/lanceman/zqk/cmd/zqk-community/app.version=${VERSION} \
  -X github.com/lanceman/zqk/cmd/zqk-community/app.buildDate=${BUILD_DATE} \
  -X github.com/lanceman/zqk/cmd/zqk-community/app.gitCommit=${GIT_COMMIT}"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

echo "Building local zcom archives ${VERSION} (commit ${GIT_COMMIT})"
echo "Public publish is held — no gh release, no brew tap, no git push."

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  ARCHIVE_NAME="zcom_${VER_NUM}_${GOOS}_${GOARCH}"
  STAGE_DIR="${DIST_DIR}/${ARCHIVE_NAME}"
  mkdir -p "$STAGE_DIR"
  echo "  ${GOOS}/${GOARCH}"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -ldflags "$LDFLAGS" -o "${STAGE_DIR}/zcom" ./cmd/zqk-community
  [ -f "${REPO_ROOT}/LICENSE" ] && cp "${REPO_ROOT}/LICENSE" "$STAGE_DIR/"
  [ -f "${REPO_ROOT}/README.md" ] && cp "${REPO_ROOT}/README.md" "$STAGE_DIR/"
  (cd "$DIST_DIR" && tar -czf "${ARCHIVE_NAME}.tar.gz" "$ARCHIVE_NAME")
  rm -rf "$STAGE_DIR"
done

(cd "$DIST_DIR" && shasum -a 256 ./*.tar.gz > checksums.txt)
(cd "$DIST_DIR" && shasum -a 256 -c checksums.txt)

echo ""
echo "Local artifacts in ${DIST_DIR}/"
ls -lh "$DIST_DIR/"
cat "$DIST_DIR/checksums.txt"

if [ "$DRY_RUN" != "--dry" ]; then
  echo ""
  echo "REFUSE: GitHub/Homebrew publish is held for this SKU."
  echo "Artifacts are local only. Re-run with --dry for the same local build."
  exit 2
fi
