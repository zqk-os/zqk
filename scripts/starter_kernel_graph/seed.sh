#!/usr/bin/env bash
# Seed the starter kernel graph: organization → mission → vision → goal →
# workstream → priority_plan, then gen-trace-pipeline on a requirement.
#
# This is the stranger/agent spine. Do not tell people to `object create goal
# --field title=...` on an empty kernel.
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

mint_json() {
  kind="$1"
  title="$2"
  shift 2
  existing="$("$BIN" object list "$kind" --format json --timeout 60s 2>/dev/null || true)"
  id="$(python3 -c '
import json,sys
raw=sys.stdin.read().strip()
if not raw:
    raise SystemExit(0)
try:
    d=json.loads(raw)
except Exception:
    raise SystemExit(0)
objs=d.get("objects") or []
print(objs[0]["id"] if objs else "")
' <<<"$existing")"
  if [ -n "$id" ]; then
    printf '%s\n' "$id"
    return 0
  fi
  extra=""
  if [ "$kind" = "goal" ] || [ "$kind" = "requirement" ] || [ "$kind" = "milestone" ]; then
    extra="--skip-trace-pipeline"
  fi
  out="$("$BIN" new object "$kind" --title "$title" --format json --timeout 120s $extra)"
  python3 -c '
import json,sys
d=json.loads(sys.argv[1])
print(d.get("id") or d.get("object",{}).get("id") or "")
' "$out"
}

apply_file() {
  id="$1"
  file="$2"
  "$BIN" object update "$id" --file "$file" --format yaml --timeout 90s >/dev/null
}

promote_id() {
  id="$1"
  "$BIN" object promote "$id" --format yaml --timeout 90s >/dev/null || true
}

create_org() {
  existing="$("$BIN" object list organization --format json --timeout 60s 2>/dev/null || true)"
  id="$(python3 -c '
import json,sys
raw=sys.stdin.read().strip()
if not raw:
    raise SystemExit(0)
try:
    d=json.loads(raw)
except Exception:
    raise SystemExit(0)
objs=d.get("objects") or []
print(objs[0]["id"] if objs else "")
' <<<"$existing")"
  if [ -n "$id" ]; then
    printf '%s\n' "$id"
    return 0
  fi
  cat > "$WORKDIR/org-create.yaml" <<'EOF'
kind: organization
title: ZQK Open-Core Community
organization_name: ZQK Open-Core Community
domain: custom
spec_context_broker: org_broker
spec_interpreter: org_interpreter
description: "Public open-core kernel for strangers and agents. This organization owns the starter graph that init must materialize so first-run is not an empty Gantt."
EOF
  out="$("$BIN" object create organization --file "$WORKDIR/org-create.yaml" --format json --timeout 120s)"
  python3 -c '
import json,sys
d=json.loads(sys.argv[1])
print(d.get("id") or d.get("object",{}).get("id") or "")
' "$out"
}

create_workstream() {
  existing="$("$BIN" object list workstream --format json --timeout 60s 2>/dev/null || true)"
  id="$(python3 -c '
import json,sys
raw=sys.stdin.read().strip()
if not raw:
    raise SystemExit(0)
try:
    d=json.loads(raw)
except Exception:
    raise SystemExit(0)
objs=[o for o in (d.get("objects") or []) if str(o.get("status")) not in ("conceptual","")]
print(objs[0]["id"] if objs else "")
' <<<"$existing")"
  if [ -n "$id" ]; then
    printf '%s\n' "$id"
    return 0
  fi
  cat > "$WORKDIR/ws-create.yaml" <<'EOF'
kind: workstream
title: Community launch and first-run kernel
description: "Execution lane for community launch vetting: isolation, documentation graph, starter kernel objects, and installer/quickstart."
category: feature
entry_point: scripts/starter_kernel_graph/seed.sh
owner_ref: ACC-SYSTEM
EOF
  out="$("$BIN" object create workstream --file "$WORKDIR/ws-create.yaml" --format json --timeout 120s)"
  python3 -c '
import json,sys
d=json.loads(sys.argv[1])
print(d.get("id") or d.get("object",{}).get("id") or "")
' "$out"
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

ORG_ID="$(create_org)"
promote_id "$ORG_ID"

MIS_ID="$(mint_json mission "Ship a kernel strangers can init and operate")"
cat > "$WORKDIR/mis.yaml" <<'EOF'
mission_statement: "Give every operator a local, content-addressed knowledge kernel they can initialize, query, and extend without reconstructing the Gantt matrix from tribal knowledge."
problem_statement: "Init currently leaves organization, mission, vision, goal, and workstream empty. start-here then tells agents to list goals and create one with a title field. New users and agents cannot assemble the pipeline."
description: "Community launch mission: a first-run kernel that already has the strategic spine and one traced requirement."
EOF
apply_file "$MIS_ID" "$WORKDIR/mis.yaml"
promote_id "$MIS_ID"

VIS_ID="$(mint_json vision "First-run produces a complete executable graph")"
cat > "$WORKDIR/vis.yaml" <<'EOF'
narrative: "A stranger runs system init and immediately has organization, mission, vision, goal, workstream, priority_plan, and a requirement with criteria/tests/backlog — seated as the system account, not a test harness."
description: "Desired end state for community first-run: whats-next has a real plan because the starter graph exists."
EOF
apply_file "$VIS_ID" "$WORKDIR/vis.yaml"
promote_id "$VIS_ID"

WS_ID="$(create_workstream)"
promote_id "$WS_ID"

GOAL_ID="$(mint_json goal "Community kernel is launch-ready for strangers and agents")"
cat > "$WORKDIR/goal.yaml" <<'EOF'
description: "Measurable outcome: zcom in this checkout lists a linked org/mission/vision/goal/workstream/priority_plan, live docs have doc_entry rows, archive copies are gone, and identity is the system account."
EOF
apply_file "$GOAL_ID" "$WORKDIR/goal.yaml"
promote_id "$GOAL_ID"

PRI_ID="$(mint_json priority_plan "Community launch testing and vetting")"
cat > "$WORKDIR/pri.yaml" <<'EOF'
description: "Execution column for proving the community kernel is launch-ready: isolation, starter graph, documentation vetting, and first-run smoke."
EOF
apply_file "$PRI_ID" "$WORKDIR/pri.yaml"
promote_id "$PRI_ID"

REQ_ID="$(mint_json requirement "Community kernel must prove launch-ready testing and documentation vetting")"
cat > "$WORKDIR/req.yaml" <<'EOF'
description: "The community checkout must be operable as its own kernel: objects resolve here, the starter graph is present, live architecture/best-practices/onboarding have doc_entry rows, and archive markdown is not shipped."
priority: p0
EOF
apply_file "$REQ_ID" "$WORKDIR/req.yaml"
"$BIN" workflow gen-trace-pipeline "$REQ_ID" --timeout 120s >/dev/null || true
promote_id "$REQ_ID"

"$BIN" object ref add "$VIS_ID" "$MIS_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$VIS_ID" "$GOAL_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$MIS_ID" "$GOAL_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$MIS_ID" "$WS_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$GOAL_ID" "$WS_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$PRI_ID" "$WS_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$ORG_ID" "$GOAL_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$REQ_ID" "$GOAL_ID" --timeout 60s >/dev/null || true
"$BIN" object ref add "$REQ_ID" "$PRI_ID" --timeout 60s >/dev/null || true

python3 - <<PY
print("starter_kernel_graph")
print("organization=$ORG_ID")
print("mission=$MIS_ID")
print("vision=$VIS_ID")
print("workstream=$WS_ID")
print("goal=$GOAL_ID")
print("priority_plan=$PRI_ID")
print("requirement=$REQ_ID")
PY
