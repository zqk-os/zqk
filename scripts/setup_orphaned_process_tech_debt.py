#!/usr/bin/env python3
"""
setup_orphaned_process_tech_debt.py

Objectify, kernelize, and prioritize technical debt item TDE-F-TST-ORPHANED-PROCESS-LEAK
along with its full DoD-traceable lineage (Milestone, Requirement, Criterion, Priority Plan,
Test Case, and Backlog Item).
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

def import_objects(kind, objects):
    import_path = f"/tmp/import_{kind}.json"
    with open(import_path, "w", encoding="utf-8") as f:
        json.dump(objects, f, indent=2)
    print(f"Importing {len(objects)} {kind} objects into kernel...")
    import_cmd = ["./bin/zqk", "object", "import", "--file", import_path, "--mode", "upsert"]
    res = subprocess.run(import_cmd, capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Error importing {kind} objects: {res.stderr}\n{res.stdout}", file=sys.stderr)
        sys.exit(1)
    if os.path.exists(import_path):
        os.remove(import_path)

def main():
    print("Kernelizing TDE-F-TST-ORPHANED-PROCESS-LEAK and lineage...")

    # 1. Technical Debt Object
    tde_obj = [{
        "id": "TDE-F-TST-ORPHANED-PROCESS-LEAK",
        "kind": "technical_debt",
        "title": "Test Harness and Child Subprocess Leak Causes Runaway CPU and System Exhaustion",
        "description": "Test suites invoking subprocesses, background daemons, and archive builds leak orphan child processes when tests time out or terminate. Root causes include lack of process-group pgid scoping, missing t.Cleanup process tree termination, and unbounded recursive compilation in test suites, causing severe CPU spikes and antivirus thrashing.",
        "debt_type": "testability",
        "impact_assessment": "high",
        "priority": "high",
        "target_resolution_date": "2026-10-15",
        "status": "identified",
        "tags": ["cef", "tst", "testability", "high", "finding:F-TST-ORPHANED-PROCESS-LEAK"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("technical_debt", tde_obj)

    # 2. Milestone
    milestone = [{
        "id": "MIL-TEST-HARNESS-ROBUSTNESS",
        "kind": "milestone",
        "title": "Test Harness Robustness and Subprocess Lifecycle Management",
        "description": "Troubleshoot and eradicate root causes of orphaned test processes, background daemon leaks, and CPU exhaustion during test execution.",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("milestone", milestone)

    # 3. Criteria (kind: criteria)
    crit_objs = [{
        "id": "CRIT-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "kind": "criteria",
        "title": "Validation criteria for REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "description": "All test suites spawning subprocesses or background daemons must bind execution to context cancellation, setpgid process groups, and t.Cleanup termination hooks to prevent orphan process leaks.",
        "status": "originated",
        "category": "functional",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("criteria", crit_objs)

    # 4. Requirement
    req_objs = [{
        "id": "REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "kind": "requirement",
        "title": "Deterministic Subprocess Scoping and Process Reaping Across Test Suites",
        "description": "Audit and enforce strict subprocess lifecycle management across testkit, scheduler, and CLI integration tests so that any spawned process trees are guaranteed to terminate upon test completion, failure, or timeout.",
        "priority": "high",
        "priority_tier": "P1",
        "status": "originated",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
        "criteria_refs": ["CRIT-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("requirement", req_objs)

    # 5. Priority Plan
    pri_plan = {
        "id": "PRI-TST-SUBPROCESS-HYGIENE",
        "kind": "priority_plan",
        "title": "Troubleshoot and Eradicate Orphaned Test Subprocess Leaks",
        "description": "Priority plan targeting TDE-F-TST-ORPHANED-PROCESS-LEAK: investigate subprocess invocation patterns in test files, implement process group reaping utilities, and verify zero orphan leaks under test aborts.",
        "status": "originated",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-TST-SUBPROCESS-HYGIENE",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T16:58:00Z",
        "updated_at": "2026-09-25T16:58:00Z",
        "created_by": "ACC-1785920548450214012-68b850c0",
        "updated_by": "ACC-1785920548450214012-68b850c0",
        "version_context": "default",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }
    content = yaml.dump(pri_plan, sort_keys=False)
    sha = hashlib.sha256(content.encode("utf-8")).hexdigest()
    os.makedirs(".zqk/process/priority_plans", exist_ok=True)
    with open(f".zqk/process/priority_plans/{sha}.yaml", "w", encoding="utf-8") as f:
        f.write(content)

    index_path = ".zqk/process/priority_plans/.priority_plan.index"
    index_data = {"version": "1", "kind": "priority_plan", "mappings": {}}
    if os.path.exists(index_path):
        try:
            with open(index_path, "r", encoding="utf-8") as f:
                index_data = json.load(f)
        except Exception:
            pass
    index_data.setdefault("mappings", {})["PRI-TST-SUBPROCESS-HYGIENE"] = sha
    with open(index_path, "w", encoding="utf-8") as f:
        json.dump(index_data, f)

    # 6. Test Case
    tst_objs = [{
        "id": "TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "kind": "test_case",
        "title": "Automated verification of test subprocess lifecycle and process group teardown",
        "description": "Verification execution: go test -v ./pkg/testkit -run TestSubprocessCleanup",
        "criteria_refs": ["CRIT-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
        "path_or_id": "pkg/testkit/subprocess_cleanup_test.go",
        "scope": "unit",
        "status": "originated",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("test_case", tst_objs)

    # 7. Backlog Item
    bli_objs = [{
        "id": "BLI-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "kind": "backlog_item",
        "title": "Audit and harden test subprocess execution with process group reaping and strict teardown",
        "description": "Root cause and resolve TDE-F-TST-ORPHANED-PROCESS-LEAK by providing testkit process management helpers that guarantee child process tree termination via setpgid / SIGKILL on t.Cleanup.",
        "status": "planned",
        "priority": "high",
        "priority_tier": "P1",
        "estimated_effort": "3h",
        "priority_plan_ref": "PRI-TST-SUBPROCESS-HYGIENE",
        "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
        "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "criteria_refs": ["CRIT-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "test_case_refs": ["TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("backlog_item", bli_objs)

    print("✓ Successfully objectified TDE-F-TST-ORPHANED-PROCESS-LEAK and established complete DoD lineage!")

if __name__ == "__main__":
    main()
