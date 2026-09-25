#!/usr/bin/env python3
"""
setup_phase13_kernel_objects.py
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

PHASE13_ITEMS = [
    {
        "req_id": "REQ-PHASE13-INPROCESS-EXECUTOR",
        "crit_id": "CRIT-PHASE13-INPROCESS-EXECUTOR",
        "tst_id": "TST-PHASE13-INPROCESS-EXECUTOR",
        "bli_id": "BLI-PHASE13-INPROCESS-EXECUTOR",
        "finding_id": "TDE-F-ARCH-001",
        "title": "Establish In-Process Command Dispatch Architecture to Eliminate Process Fork Indirection",
        "summary": "Introduce InProcessExecutor and in-process execution handlers in scheduler command execution to eliminate process fork latency, memory footprint, and subprocess cancellation failure modes (F-ARCH-001).",
        "test_cmd": "go test -v ./pkg/scheduler -run TestInProcessExecutor",
        "code_path": "pkg/scheduler/executor_test.go",
    },
    {
        "req_id": "REQ-PHASE13-STORAGE-DECOUPLING",
        "crit_id": "CRIT-PHASE13-STORAGE-DECOUPLING",
        "tst_id": "TST-PHASE13-STORAGE-DECOUPLING",
        "bli_id": "BLI-PHASE13-STORAGE-DECOUPLING",
        "finding_id": "TDE-F-ARCH-002",
        "title": "Decouple Storage Layer Auxiliary Facilities into Dedicated Concurrency Primitives",
        "summary": "Extract WaitGroupManager and WaitGroupObserver from pkg/storage into canonical pkg/concurrency package with full test coverage and lifecycle observation (F-ARCH-002).",
        "test_cmd": "go test -v ./pkg/concurrency -run TestWaitGroupManager",
        "code_path": "pkg/concurrency/waitgroup_manager_test.go",
    },
    {
        "req_id": "REQ-PHASE13-ORCHESTRATION-TAXONOMY",
        "crit_id": "CRIT-PHASE13-ORCHESTRATION-TAXONOMY",
        "tst_id": "TST-PHASE13-ORCHESTRATION-TAXONOMY",
        "bli_id": "BLI-PHASE13-ORCHESTRATION-TAXONOMY",
        "finding_id": "TDE-F-ARCH-007",
        "title": "Harmonize Orchestration Runtimes and Deprecate Skeletal Stub Packages",
        "summary": "Establish canonical orchestration taxonomy documentation and mark legacy skeletal stubs in pkg/agentorch as deprecated shims routing to pkg/orchestration and pkg/primaryorch (F-ARCH-007).",
        "test_cmd": "go test -v ./pkg/agentorch -run TestOrchestrationEngine",
        "code_path": "pkg/agentorch/orchestrate_test.go",
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
        "id": "MIL-LAUNCH-REMEDIATION-P13",
        "kind": "milestone",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 13",
        "description": "Remediation milestone for Phase 13 covering in-process command execution (F-ARCH-001), storage auxiliary decoupling into concurrency (F-ARCH-002), and orchestration taxonomy consolidation (F-ARCH-007).",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("milestone", milestone)

    # 2. Criteria (kind: criteria) - must be imported BEFORE requirements (Tier 2 ref check)
    crit_objs = []
    for item in PHASE13_ITEMS:
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

    # 3. Requirements
    req_objs = []
    for item in PHASE13_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P13"],
            "criteria_refs": [item["crit_id"]],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("requirement", req_objs)

    # 4. Priority Plan
    pri_plan = {
        "id": "PRI-LAUNCH-REMEDIATION-PHASE13",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 13",
        "description": "Phase 13 execution plan targeting in-process command dispatch (F-ARCH-001), storage decoupling into concurrency (F-ARCH-002), and orchestration taxonomy unification (F-ARCH-007).",
        "status": "planned",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE13",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T16:25:00Z",
        "updated_at": "2026-09-25T16:25:00Z",
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

    index_path = ".zqk/process/priority_plans/.priority_plan.index"
    index_data = {"version": "1", "kind": "priority_plan", "mappings": {}}
    if os.path.exists(index_path):
        try:
            with open(index_path, "r") as f:
                index_data = json.load(f)
        except Exception:
            pass
    index_data.setdefault("mappings", {})["PRI-LAUNCH-REMEDIATION-PHASE13"] = sha
    with open(index_path, "w") as f:
        json.dump(index_data, f)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE13_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P13"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE13_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE13",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P13"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("backlog_item", bli_objs)

    print("Phase 13 lineage setup complete!")

if __name__ == "__main__":
    main()
