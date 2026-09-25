#!/usr/bin/env python3
"""
setup_phase11_kernel_objects.py
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

PHASE11_ITEMS = [
    {
        "req_id": "REQ-PHASE11-OPERABILITY-RUNBOOKS",
        "crit_id": "CRIT-PHASE11-OPERABILITY-RUNBOOKS",
        "tst_id": "TST-PHASE11-OPERABILITY-RUNBOOKS",
        "bli_id": "BLI-PHASE11-OPERABILITY-RUNBOOKS",
        "finding_id": "F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE",
        "title": "Establish Structured Operational Incident Runbooks and Triage Catalogs",
        "summary": "Create comprehensive operational runbooks in docs/runbooks/ for CAS corruption, WAL compaction failures, lock contention deadlocks, and scheduler triage (F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE).",
        "test_cmd": "go test -v ./docs -run TestRunbooksIntegrity",
        "code_path": "docs/runbooks_test.go",
    },
    {
        "req_id": "REQ-PHASE11-NATIVE-FUZZ-TESTING",
        "crit_id": "CRIT-PHASE11-NATIVE-FUZZ-TESTING",
        "tst_id": "TST-PHASE11-NATIVE-FUZZ-TESTING",
        "bli_id": "BLI-PHASE11-NATIVE-FUZZ-TESTING",
        "finding_id": "F-TST-ZERO-FUZZ-TESTING",
        "title": "Introduce Native Go Fuzz Tests for CAS, WAL, and Kernel Spec Deserializers",
        "summary": "Implement native Go fuzz testing (func FuzzXxx) across CAS frame encoding, WAL mutation parsers, and YAML deserialization pipelines (F-TST-ZERO-FUZZ-TESTING).",
        "test_cmd": "go test -v ./pkg/kernelcas -run Fuzz",
        "code_path": "pkg/kernelcas/fuzz_test.go",
    },
    {
        "req_id": "REQ-PHASE11-KERNEL-SNAP-REMEDY-CLEANUP",
        "crit_id": "CRIT-PHASE11-KERNEL-SNAP-REMEDY-CLEANUP",
        "tst_id": "TST-PHASE11-KERNEL-SNAP-REMEDY-CLEANUP",
        "bli_id": "BLI-PHASE11-KERNEL-SNAP-REMEDY-CLEANUP",
        "finding_id": "TDE-SNAP-REMEDIES",
        "title": "Resolve Lingering Kernel Snap-Remedies for Account, Workstream, and Execution Plan",
        "summary": "Verify and formally mark resolved the snap-remedy technical debt items for ACC-1785920548450214012-68b850c0, WS-CODE_EVAL, and PRI-CODE_EVAL.",
        "test_cmd": "go test -v ./pkg/objects -run TestSnapRemediesResolved",
        "code_path": "pkg/objects/snap_remedy_test.go",
    },
    {
        "req_id": "REQ-PHASE11-IDIOMATIC-CONTROL-FLOW",
        "crit_id": "CRIT-PHASE11-IDIOMATIC-CONTROL-FLOW",
        "tst_id": "TST-PHASE11-IDIOMATIC-CONTROL-FLOW",
        "bli_id": "BLI-PHASE11-IDIOMATIC-CONTROL-FLOW",
        "finding_id": "F-ARCH-008",
        "title": "Eliminate Anti-Idiomatic Functional DSLs in Scheduler in Favor of Standard Go Control Flow",
        "summary": "Refactor cmd/zqk/scheduler usages of pkg/functional fluent closures to canonical Go if/else and standard error returns (F-ARCH-008).",
        "test_cmd": "go test -v ./cmd/zqk/scheduler -run TestScheduler",
        "code_path": "cmd/zqk/scheduler/scheduler_test.go",
    }
]

def import_objects(kind, objects):
    import_path = f"/tmp/import_{kind}.json"
    with open(import_path, "w") as f:
        json.dump(objects, f, indent=2)
    print(f"Importing {len(objects)} {kind} objects into kernel...")
    import_cmd = ["./bin/zqk", "object", "import", "--file", import_path, "--mode", "upsert"]
    res = subprocess.run(import_cmd, capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Error importing {kind} objects: {res.stderr}\n{res.stdout}")
        sys.exit(1)
    if os.path.exists(import_path):
        os.remove(import_path)

def main():
    # 1. Milestone
    milestone = [{
        "id": "MIL-LAUNCH-REMEDIATION-P11",
        "kind": "milestone",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 11",
        "description": "Remediation milestone for Phase 11 covering operational runbooks, native fuzz testing, snap-remedy resolution, and idiomatic Go control flow.",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("milestone", milestone)

    # 2. Requirements
    req_objs = []
    for item in PHASE11_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P11"],
            "criteria_refs": [item["crit_id"]],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("requirement", req_objs)

    # 3. Criteria (kind: criteria)
    crit_objs = []
    for item in PHASE11_ITEMS:
        crit_objs.append({
            "id": item["crit_id"],
            "kind": "criteria",
            "title": f"Validation criteria for {item['req_id']}",
            "description": item["summary"],
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("criteria", crit_objs)

    # 4. Priority Plan
    pri_plan = {
        "id": "PRI-LAUNCH-REMEDIATION-PHASE11",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 11",
        "description": "Phase 11 execution plan targeting operational runbooks (F-OBS-STRANGER-OPERABILITY-RUNBOOK-ABSENCE), native fuzz testing (F-TST-ZERO-FUZZ-TESTING), snap-remedy debt clearance, and idiomatic Go control flow in scheduler (F-ARCH-008).",
        "status": "planned",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE11",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T15:43:00Z",
        "updated_at": "2026-09-25T15:43:00Z",
        "created_by": "ACC-1785920548450214012-68b850c0",
        "version_context": "default",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }
    content = yaml.dump(pri_plan, sort_keys=False)
    sha = hashlib.sha256(content.encode("utf-8")).hexdigest()
    os.makedirs(".zqk/process/priority_plans", exist_ok=True)
    with open(f".zqk/process/priority_plans/{sha}.yaml", "w") as f:
        f.write(content)
    with open(".zqk/process/priority_plans/.priority_plan.index", "a") as f:
        f.write(f"PRI-LAUNCH-REMEDIATION-PHASE11\t{sha}\n")

    # 5. Test Cases
    tst_objs = []
    for item in PHASE11_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P11"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE11_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE11",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P11"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("backlog_item", bli_objs)

    print("Phase 11 lineage setup complete!")

if __name__ == "__main__":
    main()
