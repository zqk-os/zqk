#!/bin/sh
set -eu

# scan-secrets.sh: Lightweight scanner for credential and API token leaks.
# Used by release gates and launch pre-flight checks.

TARGET="${1:-.}"

# Exclude self and test files from secret pattern matching
if [ -f "$TARGET" ]; then
	case "$(basename "$TARGET")" in
		scan-secrets.sh|*secret*|*_test.go)
			exit 0
			;;
	esac
fi

# Secret patterns constructed dynamically to avoid false-positive self-matching
P_GHP="gh""p_[a-zA-Z0-9]{36}"
P_GHO="gh""o_[a-zA-Z0-9]{36}"
P_PAT="github""_pat_[a-zA-Z0-9_]{82}"
P_AWS="AK""IA[0-9A-Z]{16}"
P_KEY="-----BEGIN ".*"PRIVATE KEY-----"
P_SLACK="xo""x[baprs]-[0-9]{12}-[0-9]{12}-[a-zA-Z0-9]{24}"

PATTERN="($P_GHP|$P_GHO|$P_PAT|$P_AWS|$P_KEY|$P_SLACK)"

if [ -d "$TARGET" ]; then
	if [ -e "$TARGET/.git" ] && command -v git >/dev/null 2>&1; then
		MATCHES=$(git -C "$TARGET" grep -E -n "$PATTERN" -- . \
			':!scripts/scan-secrets.sh' \
			':!pkg/systemcheck/policy/secrets.go' \
			':!pkg/systemcheck/policy/secrets_test.go' \
			':!pkg/systemcheck/policy/policy_test.go' \
			':!pkg/security/release_security.go' \
			':!pkg/security/release_security_test.go' \
			':!pkg/osslaunch/launch_prep_test.go' \
			':!config/gates.yaml' 2>/dev/null || true)
	else
		MATCHES=$(grep -r -E -n "$PATTERN" "$TARGET" \
			--exclude-dir=".git" \
			--exclude-dir=".zqk" \
			--exclude-dir="bin" \
			--exclude-dir="build" \
			--exclude-dir="dist" \
			--exclude-dir="testdata" \
			--exclude="scan-secrets.sh" \
			--exclude="secrets.go" \
			--exclude="secrets_test.go" \
			--exclude="policy_test.go" \
			--exclude="release_security.go" \
			--exclude="release_security_test.go" \
			--exclude="launch_prep_test.go" \
			--exclude="gates.yaml" 2>/dev/null || true)
	fi
else
	MATCHES=$(grep -E -n "$PATTERN" "$TARGET" 2>/dev/null || true)
fi

if [ -n "$MATCHES" ]; then
	echo "ERROR: Potential secrets detected:" >&2
	echo "$MATCHES" >&2
	exit 1
fi

echo "Zero secrets detected."
exit 0
