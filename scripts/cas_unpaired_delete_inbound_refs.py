#!/usr/bin/env python3
"""Inbound *_refs scan for unpaired .zqk/process CAS deletes.

ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1 only means "this commit is an
intentional object removal." It must still fail if deleted IDs remain in
on-disk reference fields (that mint GhostRefs / strip archived CRIT lineage).

TRACK: BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001 — inbound *_refs scan on unpaired CAS delete.
"""

from __future__ import annotations

import os
import re
from collections import defaultdict
from typing import Dict, Iterable, List, Optional, Set

ID_RE = re.compile(r"^id:\s*[\"']?([^\"'\s]+)")
KEY_RE = re.compile(r"^([ \t]*)([A-Za-z0-9_]+)\s*:")
ITEM_RE = re.compile(r"^([ \t]*)-\s*")
PROCESS_PREFIX = ".zqk/process/"
INTERNAL_PREFIX = ".zqk/specs/"


def _is_ref_key(key: str) -> bool:
    k = key.lower()
    return k.endswith("_ref") or k.endswith("_refs")


def mentions_in_ref_fields(text: str, oid: str) -> bool:
    """True when oid appears in a *_ref / *_refs scalar or sequence.

    Narrative fields (scheduler_job command/description, commit messages)
    mentioning the id are not GhostRefs.
    """
    if oid not in text:
        return False
    in_refs_indent: Optional[int] = None
    for raw in text.splitlines():
        km = KEY_RE.match(raw)
        if km:
            indent = len(km.group(1))
            key = km.group(2)
            if _is_ref_key(key):
                rest = raw.split(":", 1)[1].strip()
                if oid in rest:
                    return True
                in_refs_indent = indent
                continue
            if in_refs_indent is not None and indent <= in_refs_indent:
                in_refs_indent = None
        if in_refs_indent is None:
            continue
        im = ITEM_RE.match(raw)
        if im and len(im.group(1)) > in_refs_indent and oid in raw:
            return True
    return False


def yaml_id(text: str) -> Optional[str]:
    for line in text.splitlines():
        m = ID_RE.match(line)
        if m:
            return m.group(1)
    return None


def is_instance_yaml(rel: str) -> bool:
    rel = rel.replace("\\", "/")
    if not (rel.startswith(".zqk/process/") or rel.startswith(".zqk/process/")) or not rel.endswith(".yaml"):
        return False
    if "/_internal/" in rel:
        return False
    return True


def inbound_refs_for_deleted_ids(
    repo: str,
    deleted_ids: Iterable[str],
    skip_paths: Optional[Set[str]] = None,
) -> Dict[str, List[str]]:
    """Return deleted_id -> remaining files that still mention it (excluding own id)."""
    wanted = {oid for oid in deleted_ids if oid}
    if not wanted:
        return {}
    skip_paths = {p.replace("\\", "/") for p in (skip_paths or set())}
    hits: Dict[str, List[str]] = defaultdict(list)
    process_root = os.path.join(repo, ".zqk", "process")
    if not os.path.isdir(process_root):
        return {}
    for dirpath, dirnames, filenames in os.walk(process_root):
        dirnames[:] = [d for d in dirnames if d != "_internal"]
        for name in filenames:
            if not name.endswith(".yaml"):
                continue
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, repo).replace("\\", "/")
            if rel in skip_paths or not is_instance_yaml(rel):
                continue
            try:
                with open(full, encoding="utf-8") as fh:
                    text = fh.read()
            except OSError:
                continue
            own = yaml_id(text)
            present = [
                oid
                for oid in wanted
                if oid != own and mentions_in_ref_fields(text, oid)
            ]
            if not present:
                continue
            for oid in present:
                hits[oid].append(rel)
    return dict(hits)


def ids_surviving_in_tree(
    repo: str,
    deleted_ids: Iterable[str],
    skip_paths: Optional[Set[str]] = None,
) -> Set[str]:
    """Return deleted ids that another surviving instance YAML still declares as its own id.

    A staged delete whose id is absent from staged additions is not necessarily an
    object removal: consolidating branches can commit two blobs for one id, so the
    object survives in a file this commit does not touch. Callers must treat those
    as CAS dedupe, not as deletion, or the gate reports ghosts for live objects.
    """
    wanted = {oid for oid in deleted_ids if oid}
    if not wanted:
        return set()
    skip_paths = {p.replace("\\", "/") for p in (skip_paths or set())}
    surviving: Set[str] = set()
    process_root = os.path.join(repo, ".zqk", "process")
    if not os.path.isdir(process_root):
        return set()
    for dirpath, dirnames, filenames in os.walk(process_root):
        dirnames[:] = [d for d in dirnames if d != "_internal"]
        for name in filenames:
            if not name.endswith(".yaml"):
                continue
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, repo).replace("\\", "/")
            if rel in skip_paths or not is_instance_yaml(rel):
                continue
            try:
                with open(full, encoding="utf-8") as fh:
                    text = fh.read()
            except OSError:
                continue
            own = yaml_id(text)
            if own in wanted:
                surviving.add(own)
        if surviving == wanted:
            return surviving
    return surviving


def format_gate_failure(found: Dict[str, List[str]], limit: int = 20) -> str:
    lines = [
        "❌ PROCESS CAS UNPAIRED-DELETE GATE: deleted IDs still appear in remaining *_refs",
        "   ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1 does not authorize GhostRefs.",
        "   Restore the CAS blobs or run zqk object delete --unlink-references first",
        "   (unlink refuses archived CRIT lineage under complete BLIs — restore those).",
        "",
        f"   inbound leftovers: {len(found)} id(s)",
    ]
    n = 0
    for oid, paths in sorted(found.items()):
        n += 1
        if n > limit:
            lines.append(f"     … {len(found) - limit} more ids")
            break
        sample = ", ".join(paths[:3])
        extra = f" (+{len(paths) - 3})" if len(paths) > 3 else ""
        lines.append(f"     {oid}  in {sample}{extra}")
    return "\n".join(lines)
