#!/usr/bin/env python3
"""
setup_phase4_kernel_objects.py

Creates Requirements, Criteria, Test Cases, and Backlog Items for CEF Phase 4
Launch Remediation, wiring TPM Definition of Done lineage and priority plan membership.
"""

import json
import os
import sys
import subprocess

PHASE4_ITEMS = [
    {
        "req_id": "REQ-PHASE4-RACE-GOROUTINELABELS",
        "crit_id": "CRIT-PHASE4-RACE-GOROUTINELABELS",
        "tst_id": "TST-PHASE4-RACE-GOROUTINELABELS",
        "bli_id": "BLI-PHASE4-RACE-GOROUTINELABELS",
        "finding_id": "F-TST-RACE-DETECTOR-EXCLUSION",
        "title": "Eliminate Channel Send/Close Concurrency Data Race in Goroutinelabels Pool",
        "summary": "Synchronize goroutinelabels.Pool.Submit and Stop to prevent concurrent channel send and close operations detected by the Go race detector (F-TST-RACE-DETECTOR-EXCLUSION).",
        "test_cmd": "go test -race -short -timeout 30s ./pkg/goroutinelabels/...",
        "code_path": "pkg/goroutinelabels/pool.go",
    },
    {
        "req_id": "REQ-PHASE4-MCP-HEALTH-FASTPATH",
        "crit_id": "CRIT-PHASE4-MCP-HEALTH-FASTPATH",
        "tst_id": "TST-PHASE4-MCP-HEALTH-FASTPATH",
        "bli_id": "BLI-PHASE4-MCP-HEALTH-FASTPATH",
        "finding_id": "F-OBS-MCP-FAST-PATH-MASKING",
        "title": "Enrich MCP System Health Fast Path with Cached Integrity Status",
        "summary": "Read cached tier1 health data in getSystemHealthDataMCPFast to report genuine system health without latency penalties or false unknown masks (F-OBS-MCP-FAST-PATH-MASKING).",
        "test_cmd": "go test -v ./cmd/zqk/system -run TestMCPFastHealth",
        "code_path": "cmd/zqk/system/status_helpers.go",
    },
    {
        "req_id": "REQ-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "crit_id": "CRIT-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "tst_id": "TST-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "bli_id": "BLI-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "finding_id": "F-MNT-PANIC-IN-LIBRARIES",
        "title": "Eradicate Uncontrolled Library Panics in MCP and Kernel Compose Subsystems",
        "summary": "Refactor library functions in pkg/mcp and pkg/kernelcas/compose to return fail-closed errors or safe fallbacks instead of crashing daemons with panic (F-MNT-PANIC-IN-LIBRARIES).",
        "test_cmd": "go test -v ./pkg/mcp/... ./pkg/kernelcas/compose/...",
        "code_path": "pkg/mcp/permission_format_helpers.go",
    },
    {
        "req_id": "REQ-PHASE4-CLONES-DUAL-TREE",
        "crit_id": "CRIT-PHASE4-CLONES-DUAL-TREE",
        "tst_id": "TST-PHASE4-CLONES-DUAL-TREE",
        "bli_id": "BLI-PHASE4-CLONES-DUAL-TREE",
        "finding_id": "F-MNT-CLONES-DUAL-TREE",
        "title": "Prune Orphaned Command Builders Tree and Re-point Codegen Defaults",
        "summary": "Delete 211 unimported duplicate files in pkg/cli/command_builders and update generate-command-builders default output directory (F-MNT-CLONES-DUAL-TREE, F-ARCH-009).",
        "test_cmd": "go test -v ./pkg/zqkdev/...",
        "code_path": "pkg/zqkdev/generate_command_builders.go",
    },
    {
        "req_id": "REQ-PHASE4-METABOLISM-MUTATION-ERRORS",
        "crit_id": "CRIT-PHASE4-METABOLISM-MUTATION-ERRORS",
        "tst_id": "TST-PHASE4-METABOLISM-MUTATION-ERRORS",
        "bli_id": "BLI-PHASE4-METABOLISM-MUTATION-ERRORS",
        "finding_id": "F-MNT-ERR-BLANK-SUPPRESSION",
        "title": "Preserve and Propagate Kernel Mutation Errors in Metabolism Exhaust",
        "summary": "Check and return errors from RecordKernelMutation in pkg/swarm/metabolism/exhaust.go rather than silently discarding with blank identifier (F-MNT-ERR-BLANK-SUPPRESSION).",
        "test_cmd": "go test -v ./pkg/swarm/metabolism/...",
        "code_path": "pkg/swarm/metabolism/exhaust.go",
    },
]

def main():
    def import_and_promote(kind_name, objs):
        import_path = f"/tmp/phase4_{kind_name}_import.json"
        with open(import_path, "w", encoding="utf-8") as f:
            json.dump(objs, f, indent=2)
        print(f"Importing {len(objs)} {kind_name} objects into kernel...")
        res = subprocess.run(["./bin/zqk", "object", "import", "--file", import_path, "--mode", "upsert"], capture_output=True, text=True)
        if res.returncode != 0:
            print(f"Error importing {kind_name} objects: {res.stderr}\n{res.stdout}", file=sys.stderr)
            sys.exit(res.returncode)
        if os.path.exists(import_path):
            os.remove(import_path)
        print(f"Promoting {kind_name} draft plane objects to CAS...")
        subprocess.run(["./bin/zqk", "object", "draft", "promote", "--all", "--dry-run=false"], check=False)

    # 1. Criteria
    crit_objs = []
    for item in PHASE4_ITEMS:
        crit_objs.append({
            "id": item["crit_id"],
            "kind": "criteria",
            "title": f"Validation criteria for {item['req_id']}",
            "description": item["summary"],
            "category": "functional",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("criteria", crit_objs)

    # 2. Requirements
    req_objs = []
    for item in PHASE4_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "criteria_refs": [item["crit_id"]],
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P4"],
            "priority": "p1",
            "priority_tier": "P1",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("requirement", req_objs)

    # 3. Test Cases
    tst_objs = []
    for item in PHASE4_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P4"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("test_case", tst_objs)

    # 4. Backlog Items
    bli_objs = []
    for item in PHASE4_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P4"],
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE4",
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "estimated_effort": "2h",
            "priority": "high",
            "priority_tier": "P1",
            "status": "planned",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("backlog_item", bli_objs)

    # 5. Update Priority Plan
    plan_data = {
        "title": "Kernel and Supply Launch Readiness Remediation - Phase 4 (Concurrency, Observability & Hygiene)",
        "description": "Execution plan for CEF Phase 4 remediations: goroutinelabels channel race remediation (F-TST-RACE-DETECTOR-EXCLUSION), MCP health fast-path cached status reporting (F-OBS-MCP-FAST-PATH-MASKING), library panic eradication in MCP and kernel compose (F-MNT-PANIC-IN-LIBRARIES), CLI command builder dual-tree clone pruning (F-MNT-CLONES-DUAL-TREE, F-ARCH-009), and metabolism mutation error preservation (F-MNT-ERR-BLANK-SUPPRESSION).",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE4",
        "status": "in_progress",
        "priority_tier": "P1"
    }
    plan_path = "/tmp/phase4_plan_update.json"
    with open(plan_path, "w", encoding="utf-8") as f:
        json.dump(plan_data, f, indent=2)
    subprocess.run(["./bin/zqk", "object", "update", "PRI-LAUNCH-REMEDIATION-PHASE4", "--file", plan_path], check=False)
    if os.path.exists(plan_path):
        os.remove(plan_path)

    print("✓ Successfully imported and promoted all Phase 4 objects in proper dependency order!")

if __name__ == "__main__":
    main()
