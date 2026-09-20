#!/bin/sh
# Fail-closed source-tree gate. Passing does not authorize a public push.
set -eu

ROOT=${1:-}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0

say() { printf '%s\n' "$*"; }
fail() {
	say "BLOCK: $*"
	FAIL=1
}

say "# public-release payload check"
say "root=$ROOT"

for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md \
	go.mod config/zqk.yaml cmd/zqk/main.go docs/INDEX.md docs/onboarding/COMMUNITY_FIRST_RUN.md \
	scripts/open-core/police-community-tree.sh; do
	if [ ! -f "$ROOT/$path" ]; then
		fail "required public artifact missing: $path"
	fi
done

for path in docs/_archive cmd/zqk-admin cmd/codegen_runner \
	cmd/pattern-cli pkg/billing \
	pkg/community/container_helm_test.go docs/commercial docs/marketing docs/launch; do
	if [ -e "$ROOT/$path" ]; then
		fail "forbidden or unconfigured public artifact present: $path"
	fi
done

if ! grep -Fqx 'module github.com/zqk-os/zqk' "$ROOT/go.mod"; then
	fail "go.mod does not declare module github.com/zqk-os/zqk"
fi

if git -C "$ROOT" grep -n -F 'github.com/lanceman/zqk' -- '*.go' go.mod >/dev/null 2>&1; then
	fail "private module path remains in Go source or go.mod"
fi

# Packaging/operator scripts must not advertise the private module or GitHub identity.
# rewrite-community-module-path.sh is the one-shot migrator and may name the old path.
if git -C "$ROOT" grep -n -F 'github.com/lanceman/zqk' -- \
	'scripts' \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' >/dev/null 2>&1; then
	fail "private module path remains in scripts"
fi
if git -C "$ROOT" grep -n -F 'lanceman/zqk' -- \
	'scripts' \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' >/dev/null 2>&1; then
	fail "private GitHub identity remains in scripts"
fi

if git -C "$ROOT" grep -n -i 'lanceman' -- \
	. \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' >/dev/null 2>&1; then
	fail "private owner/namespace reference (lanceman) remains in tracked repo files"
fi

# Studio technical_debt instance IDs (TDE-<nanos>-<hex>) belong in kernel CAS,
# not in public overlay/packaging comments.
if git -C "$ROOT" grep -n -E 'TDE-[0-9]{15,}-[0-9a-fA-F]{8}' -- \
	'scripts' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh'; then
	fail "studio technical_debt instance id remains in scripts"
fi

# TRACK comments must not leak studio kernel object ids (BLI/REQ/CRIT/PRI/TDE/ATK/CAP/CVS).
# cmd/zqk-shim previously shipped BLI-CEF-* / REQ-CEF-* TRACK lines while this gate
# only scanned TDE-nanos-hex under scripts/.
if git -C "$ROOT" grep -n -E 'TRACK:.*(BLI|REQ|CRIT|PRI|TDE|ATK|CAP|CVS)-' -- \
	'cmd' 'scripts' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh'; then
	fail "studio TRACK comment with kernel object id remains in cmd or scripts"
fi

if grep -E 'APPENDIX A|Proprietary|Commercial Enterprise|OPEN_CORE_PROPRIETARY_SPLIT' \
	"$ROOT/LICENSE" "$ROOT/NOTICE" >/dev/null 2>&1; then
	fail "LICENSE or NOTICE still carries the studio monorepo carve-out"
fi

if git -C "$ROOT" grep -n -E '\bzcom\b|zqk-community' -- \
	README.md CONTRIBUTING.md ZQK_GETTING_STARTED.md \
	'docs/onboarding/*.md' 'docs/architecture/*.md' \
	'scripts/open-core/sku-overlay/*.md' >/dev/null 2>&1; then
	fail "public first-run text contains a local executable or compile-unit name"
fi

sensitive=$(git -C "$ROOT" ls-files \
	'.zqk/process/**' 'docs/process/**' \
	'.zqk/keystore/**' '.zqk/state/**' 'config/zqk-local.yaml' \
	'*.pem' '*.key' '*.p12' '*.pfx' '*.test')
if [ -n "$sensitive" ]; then
	fail "tracked private/runtime material found"
	say "$sensitive"
fi

if git -C "$ROOT" grep -l -E \
	'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|xox[baprs]-[A-Za-z0-9-]{10,}' \
	-- . ':!scripts/open-core/check-public-release-payload.sh' >/dev/null 2>&1; then
	fail "tracked files contain an obvious credential or private-key marker"
fi

if ! sh "$ROOT/scripts/open-core/police-community-tree.sh" "$ROOT"; then
	fail "community tree police failed"
fi

if [ "$FAIL" -ne 0 ]; then
	say "RESULT=FAIL (do not publish)"
	exit 1
fi

say "RESULT=PASS (payload only; human public-push acknowledgment is still required)"
