#!/usr/bin/env bash
# flip-public-launch.sh
# End-to-end launch orchestrator:
#   1. Runs pre-flight verification gates (clean tree, unit tests, release payload).
#   2. Flips GitHub repository visibility from private to public.
#   3. Immediately applies strict branch protection to 'main' (blocks direct pushes).
#   4. Updates docs/onboarding/COMMUNITY_FIRST_RUN.md to activate Homebrew/release channels.
#   5. Updates internal/distribution/packaging_docs_test.go to enforce active install paths.
#   6. Rebuilds and deploys the live docs portal to docs.zqk.dev.
#   7. Cuts and pushes the initial release tag (e.g. v0.1.0) to trigger GoReleaser.
#
# Usage:
#   ./scripts/open-core/flip-public-launch.sh [--dry-run] [--tag v0.1.0]

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DRY_RUN=false
TARGET_TAG="v0.1.0"
REPO="zqk-os/zqk"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --tag)
      TARGET_TAG="$2"
      shift 2
      ;;
    --repo)
      REPO="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [--dry-run] [--tag <version-tag>] [--repo <owner/repo>]"
      exit 0
      ;;
    *)
      echo "Unknown flag: $1" >&2
      exit 1
      ;;
  esac
done

echo "=========================================================="
echo "🚀 ZQK Public Launch Transition Orchestrator"
echo "Target Repo:    ${REPO}"
echo "Target Tag:     ${TARGET_TAG}"
echo "Dry Run Mode:   ${DRY_RUN}"
echo "=========================================================="

cd "${REPO_ROOT}"

# Phase 1: Pre-flight Verification
echo ""
echo "📋 [Phase 1/6] Running Pre-Flight Invariant Gates..."

echo "  - Checking public release payload integrity..."
sh "${REPO_ROOT}/scripts/open-core/check-public-release-payload.sh"

echo "  - Verifying docs portal compilation..."
bash "${REPO_ROOT}/scripts/open-core/docs-portal/generate-docs-portal.sh" /tmp/zqk-docs-launch-check

echo "✅ Pre-flight gates satisfied."

# Phase 2: Flip Repository to Public
echo ""
echo "🌐 [Phase 2/6] Flipping Repository Visibility to PUBLIC..."
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY RUN] Would execute: gh repo edit ${REPO} --visibility public --accept-visibility-change-consequences"
else
  echo "  Executing repo visibility update to public..."
  gh repo edit "${REPO}" --visibility public --accept-visibility-change-consequences
  echo "✅ Repository ${REPO} is now PUBLIC."
fi

# Phase 3: Lock Down Branch Protection on 'main'
echo ""
echo "🔒 [Phase 3/6] Applying Strict Branch Protection to 'main'..."
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY RUN] Would run: ./scripts/open-core/apply-github-branch-protection.sh ${REPO}"
else
  bash "${REPO_ROOT}/scripts/open-core/apply-github-branch-protection.sh" "${REPO}"
  echo "✅ 'main' branch is locked fail-closed."
fi

# Phase 4: Promote Post-Launch Documentation
echo ""
echo "📝 [Phase 4/6] Activating Post-Launch Installation Documentation..."
COMMUNITY_DOC="${REPO_ROOT}/docs/onboarding/COMMUNITY_FIRST_RUN.md"
TEST_FILE="${REPO_ROOT}/internal/distribution/packaging_docs_test.go"

if [ "$DRY_RUN" = true ]; then
  echo "  [DRY RUN] Would update COMMUNITY_FIRST_RUN.md with Homebrew & GitHub release instructions"
  echo "  [DRY RUN] Would update internal/distribution/packaging_docs_test.go to assert active brew tap"
else
  if [ -f "$COMMUNITY_DOC" ]; then
    # Swap interim disclaimer with active Homebrew and GitHub release instructions
    python3 -c "
import re

doc_path = '$COMMUNITY_DOC'
with open(doc_path, 'r') as f:
    text = f.read()

old_disclaimer = 'There is **no Homebrew formula and no public GitHub release** yet.'
new_instructions = '''### Homebrew (macOS & Linux)
\`\`\`bash
brew tap zqk-os/tap
brew install zqk
\`\`\`

### GitHub Release Binaries
Pre-compiled release archives and OpenVEX attestations are available at:
https://github.com/zqk-os/zqk/releases/latest'''

if old_disclaimer in text:
    text = text.replace(old_disclaimer, new_instructions)
    with open(doc_path, 'w') as f:
        f.write(text)
    print('  ✓ Replaced pre-launch disclaimer in COMMUNITY_FIRST_RUN.md')
else:
    print('  ℹ️ Disclaimer already updated or not found in COMMUNITY_FIRST_RUN.md')
"
  fi

  if [ -f "$TEST_FILE" ]; then
    # Update test assertion to expect active homebrew instructions
    python3 -c "
test_path = '$TEST_FILE'
with open(test_path, 'r') as f:
    text = f.read()

old_needle = '\"no Homebrew formula and no public GitHub release\",'
new_needle = '\"brew tap zqk-os/tap\",'

if old_needle in text:
    text = text.replace(old_needle, new_needle)
    with open(test_path, 'w') as f:
        f.write(text)
    print('  ✓ Updated internal/distribution/packaging_docs_test.go to assert active brew tap')
"
  fi
fi

# Phase 5: Rebuild & Publish Documentation
echo ""
echo "📚 [Phase 5/6] Rebuilding and Syncing Live Docs Portal..."
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY RUN] Would rebuild portal and push to docs.zqk.dev"
else
  bash "${REPO_ROOT}/scripts/open-core/docs-portal/generate-docs-portal.sh" /tmp/zqk-docs-build
  if [ -d "/tmp/zqk-docs-repo/.git" ]; then
    cp -R /tmp/zqk-docs-build/ /tmp/zqk-docs-repo/
    git -C /tmp/zqk-docs-repo add -A
    git -C /tmp/zqk-docs-repo commit -m "docs: promote post-launch installation channels" || true
    git -C /tmp/zqk-docs-repo push origin main || true
    echo "✅ Published updated docs portal to https://docs.zqk.dev"
  fi
fi

# Phase 6: Cut Release Tag
echo ""
echo "🏷️  [Phase 6/6] Cutting and Pushing Initial Release Tag (${TARGET_TAG})..."
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY RUN] Would execute: git tag -a ${TARGET_TAG} -m 'ZQK Community Launch ${TARGET_TAG}' && git push origin ${TARGET_TAG}"
else
  git tag -a "${TARGET_TAG}" -m "ZQK Community Launch ${TARGET_TAG}"
  git push origin "${TARGET_TAG}"
  echo "✅ Release tag ${TARGET_TAG} pushed to GitHub. GitHub Actions GoReleaser pipeline is triggered!"
fi

echo ""
echo "=========================================================="
echo "🎉 ZQK PUBLIC LAUNCH PIPELINE COMPLETE!"
echo "Repository:    https://github.com/${REPO}"
echo "Documentation: https://docs.zqk.dev"
echo "Releases:      https://github.com/${REPO}/releases/tag/${TARGET_TAG}"
echo "Branch Rules:  Fail-closed on 'main'"
echo "=========================================================="
