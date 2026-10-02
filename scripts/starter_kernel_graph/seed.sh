#!/usr/bin/env bash
# Seed the starter kernel graph: organization → mission → vision → goal →
# workstream → priority_plan, then gen-trace-pipeline on a requirement.
#
# This is the stranger/agent spine. Powered by declarative ZQL transactions.
#
# Usage (community):
#   unset ZQK_PROJECT_ROOT ZQK_TEST_ROOT
#   ZCOM_PROJECT_ROOT=/path/to/project sh scripts/starter_kernel_graph/seed.sh
#   (Do not 'export ZCOM_PROJECT_ROOT'; an exported root hijacks CWD for later inits)
#
# Existing originated objects of each kind are reused (idempotent).

set -eu

ROOT="${ZCOM_PROJECT_ROOT:-${ZQK_PROJECT_ROOT:-$(pwd)}}"
BIN="${ZQK_STARTER_BIN:-}"
if [ -z "$BIN" ]; then
  if [ -n "${ZCOM_PROJECT_ROOT:-}" ] && [ -x "$ROOT/bin/zcom" ]; then
    BIN="$ROOT/bin/zcom"
  elif [ -x "$ROOT/bin/zqk" ]; then
    BIN="$ROOT/bin/zqk"
  else
    BIN="zqk"
  fi
fi

cd "$ROOT"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ZQL_FILE="$SCRIPT_DIR/starter_graph.zql"

# Check if organization already exists
existing_org="$("$BIN" object list organization --format json --timeout 60s 2>/dev/null || true)"
org_count="$(python3 -c '
import json,sys
raw=sys.stdin.read().strip()
if not raw:
    print(0)
    sys.exit(0)
try:
    d=json.loads(raw)
    print(len(d.get("objects") or []))
except Exception:
    print(0)
' <<<"$existing_org")"

if [ "$org_count" -eq 0 ]; then
  echo "🌱 Seeding starter kernel graph via declarative ZQL transaction..."
  "$BIN" mutate -f "$ZQL_FILE" --format json --timeout 120s >/dev/null
  echo "✓ Starter kernel graph committed atomically."
else
  echo "ℹ️  Existing organization found; skipping ZQL seed to preserve graph."
fi

# Locate the starter requirement and generate trace pipeline if not yet traced
req_id="$(python3 -c '
import json,sys,subprocess
res = subprocess.run(["'"$BIN"'", "object", "list", "requirement", "--format", "json"], capture_output=True, text=True)
if res.returncode == 0 and res.stdout.strip():
    try:
        objs = json.loads(res.stdout).get("objects") or []
        for o in objs:
            if "launch-ready" in o.get("title", "").lower() or "documentation vetting" in o.get("title", "").lower():
                print(o.get("id", ""))
                sys.exit(0)
        if objs:
            print(objs[0].get("id", ""))
    except Exception:
        pass
')"

if [ -n "$req_id" ]; then
  echo "Tracing requirement $req_id..."
  "$BIN" workflow gen-trace-pipeline "$req_id" --timeout 120s >/dev/null 2>&1 || true
fi

echo "starter_kernel_graph seeded successfully."
