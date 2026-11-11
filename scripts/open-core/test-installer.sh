#!/bin/sh
# Test suite for scripts/install.sh (public candidate / community tree).
# Verifies syntax, public defaults, and no-token stranger path documentation.
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
INSTALLER="$REPO_ROOT/scripts/install.sh"

echo "# Testing scripts/install.sh"

# 1. Syntax check
sh -n "$INSTALLER"
echo "✓ Syntax valid"

# 2. Check repo defaults
if ! grep -F 'REPO="${ZQK_REPO:-zqk-os/zqk}"' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh does not default to zqk-os/zqk" >&2
  exit 1
fi
if ! grep -F 'MODULE="${ZQK_MODULE:-github.com/zqk-os/zqk}"' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh does not default to github.com/zqk-os/zqk" >&2
  exit 1
fi
echo "✓ Default repo and module are zqk-os/zqk"

# 3. Public / stranger path must not require GITHUB_TOKEN
if ! grep -F 'ZQK_INSTALL_UNAUTH' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh missing ZQK_INSTALL_UNAUTH stranger-path guard" >&2
  exit 1
fi
if ! grep -F 'ZQK_INSTALL_METHOD=source' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh does not document source install without token" >&2
  exit 1
fi
if ! grep -F 'ZQK_INSTALL_METHOD=goinstall' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh does not document goinstall without token" >&2
  exit 1
fi
if ! grep -E 'no token|NO GITHUB_TOKEN|no GITHUB_TOKEN' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh does not state that the public path needs no GITHUB_TOKEN" >&2
  exit 1
fi
echo "✓ Public/unauth install path documented (no GITHUB_TOKEN required)"

# 4. Auto fallback chain includes source
if ! grep -F 'building from source' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh auto path missing source fallback" >&2
  exit 1
fi
echo "✓ Auto path falls through to source without token"

# 5. Checksum helper present (fail-closed for binary path)
if ! grep -F '_verify_archive_sha256' "$INSTALLER" >/dev/null; then
  echo "FAIL: install.sh missing checksum verification helper" >&2
  exit 1
fi
echo "✓ Binary path checksum verification present"

echo "PASS: all installer tests passed"
