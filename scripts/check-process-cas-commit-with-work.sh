#!/bin/sh
#
# Fail closed: CAS instance rename pairs under .zqk/process/ must be staged
# with the commit that ships the work.
#
# Completing/updating a kernel object changes bytes, so the filename hash
# changes. Git still tracks the old hash until the delete+add pair is committed.
# Leaving that pair untracked, then `git reset --hard origin/main`, restores
# the old status (e.g. planned) and looks like completed work "demoted".
#
# Pair = tracked .zqk/process/<kind>/<64-hex>.yaml deleted in the worktree
#         + untracked (or newly added) file in the same directory whose YAML
#         `id:` matches the deleted object's id.
#
# Unpaired delete = staged D of a CAS instance with no staged A of the same
# `id:`. That is object removal, not a rename. It mints GhostRefs. Blocked
# unless ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1 (this commit only) after
# `zqk object delete --unlink-references` AND remaining process YAML no longer
# names those IDs in *_refs (inbound scan still runs under break-glass).
#
# Break-glass (human only, this commit):
#   ZQK_ALLOW_UNSTAGED_PROCESS_CAS=1
#   ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1
#
# See: .cursor/rules/cas-cache-git-hygiene.mdc
# TRACK: after S6 complete CAS was wiped by post-#1644 reset --hard origin/main
# TRACK: BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001 — inbound *_refs still runs under break-glass.
#

set -e

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT" || exit 1

if [ -n "${ZQK_ALLOW_UNSTAGED_PROCESS_CAS:-}" ]; then
	echo "⚠️  ZQK_ALLOW_UNSTAGED_PROCESS_CAS set — skipping CAS-with-work commit gate" >&2
	exit 0
fi

if ! git diff --cached --quiet 2>/dev/null; then
	:
else
	# Nothing staged — not a commit in flight.
	exit 0
fi

if [ -x "$REPO_ROOT/scripts/check-process-internal-guard.sh" ]; then
	"$REPO_ROOT/scripts/check-process-internal-guard.sh"
fi

python3 - "$REPO_ROOT" <<'PY'
import os
import re
import subprocess
import sys

repo = sys.argv[1]
os.chdir(repo)

CAS_NAME = re.compile(r"^[0-9a-f]{64}\.yaml$")
ID_RE = re.compile(r"^id:\s*[\"']?([^\"'\s]+)")


def yaml_id(text: str):
    for line in text.splitlines():
        m = ID_RE.match(line)
        if m:
            return m.group(1)
    return None


def is_instance_cas(path: str) -> bool:
    if not path.startswith(".zqk/process/") or not path.endswith(".yaml"):
        return False
    if "/_internal/" in path:
        return False
    return bool(CAS_NAME.match(os.path.basename(path)))


def git_output(*args) -> str:
    return subprocess.check_output(["git", *args], text=True, stderr=subprocess.DEVNULL)


porcelain = git_output("status", "--porcelain", "-uall", "--", ".zqk/process")
cached = git_output("diff", "--cached", "--name-status", "--", ".zqk/process")

staged_deleted = set()
staged_added = set()
for line in cached.splitlines():
    if not line.strip():
        continue
    status, path = line.split("\t", 1) if "\t" in line else (line[:1], line[1:].strip())
    # name-status: D\tpath / A\tpath / R100\told\tnew
    parts = line.split("\t")
    code = parts[0]
    if code.startswith("R") and len(parts) >= 3:
        staged_deleted.add(parts[1])
        staged_added.add(parts[2])
        continue
    if len(parts) < 2:
        continue
    path = parts[1]
    if code.startswith("D"):
        staged_deleted.add(path)
    elif code.startswith("A") or code.startswith("M") or code.startswith("C"):
        staged_added.add(path)

deleted_wt = []  # (path, oid)
untracked = []  # (path, oid)

for line in porcelain.splitlines():
    if len(line) < 4:
        continue
    xy, path = line[:2], line[3:]
    if "/_internal/" in path:
        continue
    if not is_instance_cas(path):
        continue
    # unstaged or untracked delete
    if xy[1] == "D" or xy == " D" or xy == "D ":
        try:
            blob = git_output("show", f"HEAD:{path}")
        except subprocess.CalledProcessError:
            continue
        oid = yaml_id(blob)
        if oid:
            deleted_wt.append((path, oid))
    elif xy == "??":
        try:
            with open(path, encoding="utf-8") as fh:
                oid = yaml_id(fh.read())
        except OSError:
            continue
        if oid:
            untracked.append((path, oid))

# Also treat index-deleted (staged) as a deleted side of the pair for matching,
# so we can see untracked partners that still need to be added.
for path in staged_deleted:
    if not is_instance_cas(path):
        continue
    try:
        blob = git_output("show", f"HEAD:{path}")
    except subprocess.CalledProcessError:
        continue
    oid = yaml_id(blob)
    if oid and (path, oid) not in deleted_wt:
        deleted_wt.append((path, oid))

untracked_by_id = {}
for path, oid in untracked:
    untracked_by_id.setdefault(oid, []).append(path)

missing = []
for old_path, oid in deleted_wt:
    news = untracked_by_id.get(oid, [])
    if not news:
        continue
    delete_ok = old_path in staged_deleted
    added_ok = any(p in staged_added for p in news)
    if delete_ok and added_ok:
        continue
    missing.append((oid, old_path, news, delete_ok, added_ok))

# Unpaired instance deletes (D with no same-id A) mint GhostRefs. The rename-pair
# check above only fires when an untracked new hash exists; it allows committing
# hundreds of deletes with no replacement (eadbd756c7, f2bdc25572 — 2026-08-19
# CRIT massacre). True object removal: zqk object delete --unlink-references,
# then ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1 for that commit only — and only
# when remaining .zqk/process YAML no longer names those IDs in *_refs.
sys.path.insert(0, os.path.join(repo, "scripts"))
from cas_unpaired_delete_inbound_refs import (  # noqa: E402
    inbound_refs_for_deleted_ids,
    ids_surviving_in_tree,
    format_gate_failure,
)

allow_unpaired_delete = os.environ.get("ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE", "").strip()
unpaired_deletes = []
added_ids = {}
for path in staged_added:
    if not is_instance_cas(path):
        continue
    try:
        blob = git_output("show", f":{path}")
    except subprocess.CalledProcessError:
        try:
            with open(path, encoding="utf-8") as fh:
                blob = fh.read()
        except OSError:
            continue
    oid = yaml_id(blob)
    if oid:
        added_ids.setdefault(oid, []).append(path)
for path in staged_deleted:
    if not is_instance_cas(path):
        continue
    try:
        blob = git_output("show", f"HEAD:{path}")
    except subprocess.CalledProcessError:
        continue
    oid = yaml_id(blob)
    if not oid:
        continue
    if oid not in added_ids:
        unpaired_deletes.append((oid, path))

# An id absent from staged additions may still be provided by a file this commit
# does not touch (two blobs for one id after branch consolidation). Those deletes
# are dedupe, not removal, so they must not be reported as unpaired.
if unpaired_deletes:
    survivors = ids_surviving_in_tree(
        repo,
        [oid for oid, _ in unpaired_deletes],
        skip_paths={path for _, path in unpaired_deletes},
    )
    unpaired_deletes = [(oid, path) for oid, path in unpaired_deletes if oid not in survivors]

inbound_leftovers = {}
if unpaired_deletes and allow_unpaired_delete:
    inbound_leftovers = inbound_refs_for_deleted_ids(
        repo,
        [oid for oid, _ in unpaired_deletes],
        skip_paths={path for _, path in unpaired_deletes},
    )

if missing:
    print("❌ PROCESS CAS WITH-WORK GATE: kernel object CAS rename is not in this commit", file=sys.stderr)
    print("   Completing/updating an object writes a new hash-named YAML. Git still has the old hash.", file=sys.stderr)
    print("   If you commit only code, the next reset --hard origin/main restores the old status", file=sys.stderr)
    print("   (completed BLIs look planned again).", file=sys.stderr)
    print("", file=sys.stderr)
    print("Stage the delete+add pair (same id) in THIS commit — only those two files:", file=sys.stderr)
    print("  git add -- .zqk/process/<kind>/<old-hash>.yaml .zqk/process/<kind>/<new-hash>.yaml", file=sys.stderr)
    print("  # Do NOT git add -A .zqk/process/ — that scoops unrelated deletes and mints GhostRefs.", file=sys.stderr)
    print("", file=sys.stderr)
    for oid, old_path, news, delete_ok, added_ok in missing:
        print(f"  id: {oid}", file=sys.stderr)
        print(f"    deleted (HEAD): {old_path}  staged_delete={delete_ok}", file=sys.stderr)
        for p in news:
            print(f"    new:            {p}  staged_add={p in staged_added}", file=sys.stderr)
    print("", file=sys.stderr)
    print("Use zqk object promote|update|create (CLI) — do not hand-edit YAML.", file=sys.stderr)
    print("Break-glass THIS commit only: ZQK_ALLOW_UNSTAGED_PROCESS_CAS=1", file=sys.stderr)

if unpaired_deletes and not allow_unpaired_delete:
    print("❌ PROCESS CAS UNPAIRED-DELETE GATE: instance YAML deleted without a same-id replacement", file=sys.stderr)
    print("   That is an object deletion, not a CAS rename. It leaves referrers as GhostRef", file=sys.stderr)
    print("   (Exists false). Heal is restore or zqk object delete --unlink-references — never", file=sys.stderr)
    print("   git add -A .zqk/process/ while completing a single BLI.", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"   unpaired deletes: {len(unpaired_deletes)}", file=sys.stderr)
    for oid, path in unpaired_deletes[:20]:
        print(f"     {oid}  {path}", file=sys.stderr)
    if len(unpaired_deletes) > 20:
        print(f"     … {len(unpaired_deletes) - 20} more", file=sys.stderr)
    print("", file=sys.stderr)
    print("   Intentional delete this commit only: ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1", file=sys.stderr)
    print("   plus zqk object delete --unlink-references (inbound *_refs must already be closed).", file=sys.stderr)

if inbound_leftovers:
    print(format_gate_failure(inbound_leftovers), file=sys.stderr)

if missing or (unpaired_deletes and not allow_unpaired_delete) or inbound_leftovers:
    sys.exit(1)
sys.exit(0)
PY
