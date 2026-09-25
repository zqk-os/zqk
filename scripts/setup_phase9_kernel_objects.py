#!/usr/bin/env python3
"""
setup_phase9_kernel_objects.py

Creates Milestone, Priority Plan, Requirements, Criteria, Test Cases, and Backlog Items
for CEF Phase 9 Launch Remediation, establishing full TPM DoD lineage.
"""

import json
import os
import sys
import subprocess

PHASE9_ITEMS = [
    {
        "req_id": "REQ-PHASE9-ASYNC-ASSERTION-INTEGRITY",
        "crit_id": "CRIT-PHASE9-ASYNC-ASSERTION-INTEGRITY",
        "tst_id": "TST-PHASE9-ASYNC-ASSERTION-INTEGRITY",
        "bli_id": "BLI-PHASE9-ASYNC-ASSERTION-INTEGRITY",
        "finding_id": "F-TST-MISSING-ASSERTIONS-LOG-ONLY",
        "title": "Enforce Deterministic Assertions Over Passive Logging in Async Validation Tests",
        "summary": "Refactor asynchronous check tests in cmd/zqk/system/async_check_test.go to enforce deterministic assertions instead of passive t.Log on queue exhaustion and progress delivery (F-TST-MISSING-ASSERTIONS-LOG-ONLY).",
        "test_cmd": "go test -race -v ./cmd/zqk/system -run TestAsyncValidation_MaxRetriesExceeded",
        "code_path": "cmd/zqk/system/async_check_test.go",
    },
    {
        "req_id": "REQ-PHASE9-DETERMINISTIC-ASYNC-WAITING",
        "crit_id": "CRIT-PHASE9-DETERMINISTIC-ASYNC-WAITING",
        "tst_id": "TST-PHASE9-DETERMINISTIC-ASYNC-WAITING",
        "bli_id": "BLI-PHASE9-DETERMINISTIC-ASYNC-WAITING",
        "finding_id": "F-TST-ARBITRARY-SLEEPS-FLAKE-RISK",
        "title": "Replace Arbitrary Sleep Delays with Deterministic Condition Waiting in Async Tests",
        "summary": "Replace arbitrary multi-second time.Sleep delays in cmd/zqk/utility/scenario_builder_id_stream_test.go with deterministic condition-based waiting via require.Eventually (F-TST-ARBITRARY-SLEEPS-FLAKE-RISK).",
        "test_cmd": "go test -race -v ./cmd/zqk/utility -run TestConfigFileWatcher_RegisterAndDetectChanges",
        "code_path": "cmd/zqk/utility/scenario_builder_id_stream_test.go",
    },
    {
        "req_id": "REQ-PHASE9-CENTRALIZED-PATH-PERM-LITERALS",
        "crit_id": "CRIT-PHASE9-CENTRALIZED-PATH-PERM-LITERALS",
        "tst_id": "TST-PHASE9-CENTRALIZED-PATH-PERM-LITERALS",
        "bli_id": "BLI-PHASE9-CENTRALIZED-PATH-PERM-LITERALS",
        "finding_id": "F-MNT-LITERALS-PATHS-PERMS",
        "title": "Eradicate Raw Path Literals in Favor of Centralized Paths Constants in CEF Pack Builder",
        "summary": "Eliminate hardcoded .zqk path strings in pkg/quality/cef_pack_builder.go in favor of centralized paths.ProjectDataDir and file permission constants (F-MNT-LITERALS-PATHS-PERMS).",
        "test_cmd": "go test -race -v ./pkg/quality -run TestBuildCanonicalCEFPack_4WaveDAGAndSeal",
        "code_path": "pkg/quality/cef_pack_builder_test.go",
    },
    {
        "req_id": "REQ-PHASE9-W3C-TRACE-CHILD-PROPAGATION",
        "crit_id": "CRIT-PHASE9-W3C-TRACE-CHILD-PROPAGATION",
        "tst_id": "TST-PHASE9-W3C-TRACE-CHILD-PROPAGATION",
        "bli_id": "BLI-PHASE9-W3C-TRACE-CHILD-PROPAGATION",
        "finding_id": "F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED",
        "title": "Implement W3C TraceContext ChildSpan Generation for Distributed Correlation",
        "summary": "Implement ChildSpan method on TraceContext in pkg/telemetry/tracer.go to enable W3C trace correlation propagation across async queues and IPC (F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED).",
        "test_cmd": "go test -race -v ./pkg/telemetry -run TestChildSpan",
        "code_path": "pkg/telemetry/tracer.go",
    },
]

def run_cmd(args, check=True):
    res = subprocess.run(args, capture_output=True, text=True)
    if check and res.returncode != 0:
        print(f"Command failed: {' '.join(args)}\nError: {res.stderr}")
        sys.exit(1)
    return res

def import_and_promote(kind, objects):
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

    print(f"Promoting {kind} draft plane objects to CAS...")
    promote_cmd = ["./bin/zqk", "object", "draft", "promote", "--all", "--dry-run=false"]
    res = subprocess.run(promote_cmd, capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Error promoting {kind} objects: {res.stderr}")
        sys.exit(1)
    print(res.stdout.strip())

def main():
    # 1. Milestone
    milestone_obj = [{
        "id": "MIL-LAUNCH-REMEDIATION-P9",
        "kind": "milestone",
        "title": "CEF Phase 9 Launch Readiness Remediation",
        "description": "Remediation milestone covering async test assertion integrity, deterministic async condition waiting, centralized path literals in quality packs, and W3C distributed trace child span propagation.",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_and_promote("milestone", milestone_obj)

    # 2. Priority Plan
    plan_obj = [{
        "id": "PRI-LAUNCH-REMEDIATION-PHASE9",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 9",
        "description": "Phase 9 execution plan targeting async test assertion integrity (F-TST-MISSING-ASSERTIONS-LOG-ONLY), deterministic condition waiting (F-TST-ARBITRARY-SLEEPS-FLAKE-RISK), centralized path literals (F-MNT-LITERALS-PATHS-PERMS), and W3C trace child propagation (F-OBS-DISTRIBUTED-TRACING-UNPROPAGATED).",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE9",
        "active_order": 1,
        "status": "grooming",
        "priority_tier": "P1",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_and_promote("priority_plan", plan_obj)

    # 3. Criteria
    crit_objs = []
    for item in PHASE9_ITEMS:
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

    # 4. Requirements
    req_objs = []
    for item in PHASE9_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "criteria_refs": [item["crit_id"]],
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P9"],
            "priority": "p1",
            "priority_tier": "P1",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("requirement", req_objs)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE9_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P9"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE9_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE9",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P9"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("backlog_item", bli_objs)

    print("Phase 9 kernel lineage setup complete!")

if __name__ == "__main__":
    main()
