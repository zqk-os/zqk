#!/usr/bin/env bash
# apply-github-branch-protection.sh
# Hardens the 'main' branch on GitHub to prevent direct pushes, require PRs,
# enforce status checks, and prevent unauthorized modifications by maintainers.
#
# Usage:
#   ./scripts/open-core/apply-github-branch-protection.sh [OWNER/REPO]
# Example:
#   ./scripts/open-core/apply-github-branch-protection.sh zqk-os/zqk

set -euo pipefail

REPO="${1:-}"

if [ -z "$REPO" ]; then
  # Auto-detect from git remote
  REMOTE_URL="$(git config --get remote.origin.url || true)"
  if [[ "$REMOTE_URL" =~ github\.com[:/]([^/]+/[^/.]+)(\.git)?$ ]]; then
    REPO="${BASH_REMATCH[1]}"
  else
    echo "❌ Error: Could not determine GitHub repository. Please pass OWNER/REPO as argument." >&2
    exit 1
  fi
fi

echo "🔒 Configuring branch protection for: ${REPO} (branch: main)"

# Check gh CLI
if ! command -v gh >/dev/null 2>&1; then
  echo "❌ Error: GitHub CLI ('gh') is required but not installed." >&2
  exit 1
fi

# Check authentication
if ! gh auth status >/dev/null 2>&1; then
  echo "❌ Error: 'gh' CLI is not authenticated. Run 'gh auth login' first." >&2
  exit 1
fi

# Verify repo visibility
VISIBILITY="$(gh api "repos/${REPO}" --jq '.visibility' 2>/dev/null || true)"
if [ "$VISIBILITY" = "private" ]; then
  echo "⚠️ Warning: ${REPO} is currently PRIVATE."
  echo "   On standard/free GitHub organizations, Branch Protection is only permitted on PUBLIC repositories."
  echo "   If this command fails with HTTP 403, flip the repository to public first:"
  echo "     gh repo edit ${REPO} --visibility public"
  echo ""
fi

# Apply branch protection payload
echo "⚙️ Applying fail-closed protection rules to 'main'..."

PROTECTION_PAYLOAD=$(cat << 'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Lint & Format",
      "Unit Tests (storage)",
      "Unit Tests (cli)",
      "Unit Tests (system)"
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "required_linear_history": false,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "block_creations": false,
  "required_conversation_resolution": true,
  "lock_branch": false,
  "allow_fork_syncing": true
}
EOF
)

if gh api --method PUT "repos/${REPO}/branches/main/protection" \
  --input - <<< "$PROTECTION_PAYLOAD" >/dev/null; then
  echo "✅ Successfully locked down 'main' branch on ${REPO}:"
  echo "   - Direct pushes to 'main' are BLOCKED (PR required)."
  echo "   - Status checks (Lint & Format, Unit Tests) must pass before merge."
  echo "   - Branch must be strictly up-to-date with 'main' prior to merge."
  echo "   - Stale PR approvals are automatically dismissed on new commits."
  echo "   - Force pushes (--force) are completely DISABLED."
  echo "   - Branch deletion of 'main' is DISABLED."
  echo "   - Enforced for ALL users including repository administrators."
else
  STATUS=$?
  echo "❌ Failed to set branch protection (exit code: $STATUS)." >&2
  exit $STATUS
fi
