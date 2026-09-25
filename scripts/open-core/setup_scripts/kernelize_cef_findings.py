#!/usr/bin/env python3
"""
kernelize_cef_findings.py

Ingests all 65 CEF findings from docs/quality/cef-runs/2026-09-25-GEMINI_RUN/findings.jsonl,
converts them into first-class technical_debt objects in the Knowledge Kernel,
builds the strategic Launch Readiness Roadmap, and shapes Priority Plan Phase 4.
"""

import json
import os
import sys
import subprocess

RUN_DIR = "docs/quality/cef-runs/2026-09-25-GEMINI_RUN"
FINDINGS_FILE = os.path.join(RUN_DIR, "findings.jsonl")

# Findings already remediated and verified in Phases 1, 2, and 3
RESOLVED_FINDINGS = {
    # Phase 1
    "F-REL-TX-PARTIAL-APPLY-ATOMICTY-VIOLATION": {
        "status": "resolved",
        "notes": "Remediated in PR #198 (commit ff2dd153): Atomic rollback in FileObjectTransaction across multi-operation mutations.",
        "commit": "ff2dd153051fdd6f333efc4ce0f10f038b3c2f88"
    },
    "F-REL-WAL-DROP-AND-ADVANCE-DATA-LOSS": {
        "status": "resolved",
        "notes": "Remediated in PR #198 (commit ff2dd153) and verified in PR #200 (commit fe73ee8b): Write-behind retry and checkpoint safety under save queue saturation.",
        "commit": "fe73ee8b178530cb00fc4dae4d9a54f0a8fa25a4"
    },
    "F-SEC-SANDBOX-ALLOWLIST-ESCAPE": {
        "status": "resolved",
        "notes": "Remediated in PR #198 (commit ff2dd153): Quantum Sandbox allowlist hardened against process escape utilities (find -exec/-ok, go run).",
        "commit": "ff2dd153051fdd6f333efc4ce0f10f038b3c2f88"
    },
    "F-OBS-FALSE-GREEN-STATUS": {
        "status": "resolved",
        "notes": "Remediated in PR #198 (commit ff2dd153): Eliminated false-green status reporting; daemon and scheduler health correctly exposed.",
        "commit": "ff2dd153051fdd6f333efc4ce0f10f038b3c2f88"
    },
    # Phase 2
    "F-SEC-SCHEDULER-SHELL-INTERPOLATION": {
        "status": "resolved",
        "notes": "Remediated in PR #199 (commit 65725a9a): Direct execvp execution in scheduler eliminating /bin/sh shell command interpolation.",
        "commit": "65725a9a14704f08c3a105086d38e078ecbc5134"
    },
    "F-REL-IPC-WRITER-UNBOUNDED-HANG": {
        "status": "resolved",
        "notes": "Remediated in PR #199 (commit 65725a9a): Bound IPC Privileged Writer RPC invocations with context deadlines.",
        "commit": "65725a9a14704f08c3a105086d38e078ecbc5134"
    },
    "F-REL-DARWIN-CAS-SYNC-DROP-ON-EXIT": {
        "status": "resolved",
        "notes": "Remediated in PR #199 (commit 65725a9a): Flush and drain Darwin async fsync queue during CLI root teardown and daemon exit.",
        "commit": "65725a9a14704f08c3a105086d38e078ecbc5134"
    },
    "F-SEC-CPCP-MEMBRANE-COVERAGE-GAPS": {
        "status": "resolved",
        "notes": "Remediated in PR #199 (commit 65725a9a): Regex pattern-matched tool interceptor covering all daemon sockets and sensitive directories.",
        "commit": "65725a9a14704f08c3a105086d38e078ecbc5134"
    },
    # Phase 3
    "F-ARCH-004": {
        "status": "resolved",
        "notes": "Remediated in PR #200 (commit fe73ee8b): FileObjectStorage.GetNeighbors upgraded to O(1) reverse-reference index traversal.",
        "commit": "fe73ee8b178530cb00fc4dae4d9a54f0a8fa25a4"
    },
    "F-MNT-ERR-NILERR-DISCARD": {
        "status": "resolved",
        "notes": "Remediated in PR #200 (commit fe73ee8b): Eliminated 50 silent nilerr returns across cmd/ and enabled nilerr linter in .golangci.yml.",
        "commit": "fe73ee8b178530cb00fc4dae4d9a54f0a8fa25a4"
    },
    "F-OBS-TELEMETRY-SINK-DISCONNECTION": {
        "status": "resolved",
        "notes": "Remediated in PR #200 (commit fe73ee8b): Wired InMemoryHook ring buffer into GlobalManager and connected TelemetryDaemon aggregate sink.",
        "commit": "fe73ee8b178530cb00fc4dae4d9a54f0a8fa25a4"
    },
}

LENS_TO_DEBT_TYPE = {
    "L-ARCHITECTURE": "complexity",
    "L-SECURITY": "security",
    "L-RELIABILITY": "infrastructure",
    "L-OBSERVABILITY": "observability",
    "L-TESTING": "testing",
    "L-CODE-QUALITY": "maintainability",
    "L-SUPPLY-RELEASE": "tooling",
}

SEV_TO_IMPACT = {
    "critical": "critical",
    "high": "high",
    "moderate": "medium",
    "low": "low",
}

SEV_TO_TARGET_DATE = {
    "critical": "2026-10-15",
    "high": "2026-10-31",
    "moderate": "2026-11-15",
    "low": "2026-12-01",
}

def main():
    if not os.path.exists(FINDINGS_FILE):
        print(f"Error: {FINDINGS_FILE} not found", file=sys.stderr)
        sys.exit(1)

    with open(FINDINGS_FILE, "r", encoding="utf-8") as f:
        findings = [json.loads(line) for line in f if line.strip()]

    print(f"Ingesting {len(findings)} findings from {FINDINGS_FILE}...")

    tde_objects = []
    skipped_info = 0

    for item in findings:
        sev = item.get("severity", "moderate")
        if sev == "info":
            skipped_info += 1
            continue

        fid = item["finding_id"]
        tde_id = f"TDE-{fid}"
        lens = item.get("lens_id") or item.get("lens") or "L-CODE-QUALITY"
        debt_type = LENS_TO_DEBT_TYPE.get(lens, "maintainability")
        impact = SEV_TO_IMPACT.get(sev, "medium")
        target_date = SEV_TO_TARGET_DATE.get(sev, "2026-11-15")

        title = item.get("title", fid)
        summary = item.get("summary") or item.get("description", "")
        # Enforce max description length 2000 chars as per spec
        if len(summary) > 1950:
            summary = summary[:1950] + "..."

        obj = {
            "id": tde_id,
            "kind": "technical_debt",
            "title": title[:200],
            "description": summary,
            "debt_type": debt_type,
            "impact_assessment": impact,
            "priority": sev if sev in ["low", "medium", "high", "critical"] else "medium",
            "target_resolution_date": target_date,
            "tags": ["cef", lens.lower(), sev, f"finding:{fid}"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0",
        }

        # Check evidence for primary file path
        evidence = item.get("evidence", [])
        for ev in evidence:
            if isinstance(ev, dict) and "path" in ev and ev["path"]:
                obj["file_path"] = ev["path"]
                break

        if fid in RESOLVED_FINDINGS:
            res_info = RESOLVED_FINDINGS[fid]
            obj["status"] = res_info["status"]
            obj["resolution_notes"] = res_info["notes"]
            obj["completed_at"] = "2026-09-25T12:35:00Z"
        else:
            obj["status"] = "identified"

        tde_objects.append(obj)

    print(f"Generated {len(tde_objects)} technical_debt objects ({len(RESOLVED_FINDINGS)} resolved, {len(tde_objects) - len(RESOLVED_FINDINGS)} open/identified, {skipped_info} info skipped).")

    # Write import file
    import_path = "/tmp/cef_technical_debt_import.json"
    with open(import_path, "w", encoding="utf-8") as f:
        json.dump(tde_objects, f, indent=2)

    print(f"Importing {len(tde_objects)} objects into kernel via 'zqk object import'...")
    cmd = ["./bin/zqk", "object", "import", "--file", import_path, "--mode", "upsert"]
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Error importing objects: {res.stderr}\n{res.stdout}", file=sys.stderr)
        sys.exit(res.returncode)

    print(f"✓ Successfully imported {len(tde_objects)} technical_debt objects into CAS!")
    os.remove(import_path)

if __name__ == "__main__":
    main()
