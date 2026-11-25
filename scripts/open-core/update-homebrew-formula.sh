#!/usr/bin/env bash
set -euo pipefail

# Atomically updates zqk-os/homebrew-tap Formula/zqk.rb from a GoReleaser checksums file.
# Usage:
#   scripts/update-homebrew-formula.sh <tag> <checksums_file> [--dry-run]
#
# Environment:
#   GH_TOKEN        GitHub token with contents:write on zqk-os/homebrew-tap (required unless --dry-run)
#   HOMEBREW_TAP    Target repo slug (default: zqk-os/homebrew-tap)

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <tag> <checksums_file> [--dry-run]" >&2
  exit 1
fi

TAG="$1"
CHECKSUMS_FILE="$2"
DRY_RUN="false"
if [ "${3:-}" = "--dry-run" ]; then
  DRY_RUN="true"
fi

VERSION="${TAG#v}"
TAP_REPO="${HOMEBREW_TAP:-zqk-os/homebrew-tap}"

if [ ! -f "${CHECKSUMS_FILE}" ]; then
  echo "ERROR: checksums file not found: ${CHECKSUMS_FILE}" >&2
  exit 1
fi

SHA_DARWIN_ARM64=$(awk '$2 ~ /darwin_arm64\.tar\.gz$/ {print $1}' "${CHECKSUMS_FILE}")
SHA_DARWIN_AMD64=$(awk '$2 ~ /darwin_amd64\.tar\.gz$/ {print $1}' "${CHECKSUMS_FILE}")
SHA_LINUX_ARM64=$(awk '$2 ~ /linux_arm64\.tar\.gz$/ {print $1}' "${CHECKSUMS_FILE}")
SHA_LINUX_AMD64=$(awk '$2 ~ /linux_amd64\.tar\.gz$/ {print $1}' "${CHECKSUMS_FILE}")

for name in SHA_DARWIN_ARM64 SHA_DARWIN_AMD64 SHA_LINUX_ARM64 SHA_LINUX_AMD64; do
  if [ -z "${!name}" ]; then
    echo "ERROR: missing ${name} in ${CHECKSUMS_FILE}" >&2
    exit 1
  fi
done

FORMULA=$(cat <<EOF
class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://zqk.dev"
  version "${VERSION}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
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
)

if [ "${DRY_RUN}" = "true" ]; then
  printf '%s\n' "${FORMULA}"
  exit 0
fi

CURRENT_SHA=$(gh api "repos/${TAP_REPO}/contents/Formula/zqk.rb" --jq '.sha')
ENCODED=$(printf '%s\n' "${FORMULA}" | base64 | tr -d '\n')

gh api -X PUT "repos/${TAP_REPO}/contents/Formula/zqk.rb" \
  -f message="chore(formula): bump zqk to ${TAG}" \
  -f content="${ENCODED}" \
  -f sha="${CURRENT_SHA}" \
  --jq '{commit: .commit.sha, path: .content.path}'
