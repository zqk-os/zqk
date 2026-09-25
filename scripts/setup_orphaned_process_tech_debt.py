#!/usr/bin/env python3
"""
restructure_subprocess_hygiene_plan.py

Restructure PRI-TST-SUBPROCESS-HYGIENE incorporating user architectural overrides:
- REQ:TST ratio is 1:1 (or 2:1), NOT 1:1 per criterion
- CRIT:TST is many:1 (3 criteria -> 1 unified Test Case / Execution Organizer)
- The Test Case acts as the cohesive execution container defining the verification topology (sequential & concurrent stages)
- 4 decoupled Backlog Items mapped to the plan and unified test case
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

def import_objects(kind, objects):
    import_path = f"/tmp/import_restructure_{kind}.json"
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
    print("Restructuring PRI-TST-SUBPROCESS-HYGIENE with REQ:TST=1:1 and CRIT:TST=3:1...")

    # 1. Milestone
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

    # 2. Three-Fold Proof Criteria (3 criteria for the requirement)
    crit_objs = [
        {
            "id": "CRIT-TST-SUBPROCESS-STATIC-GUARD",
            "kind": "criteria",
            "title": "Static Floor: Static AST guardrail prohibits unmanaged exec.Command in test suites",
            "description": "Static analysis (via zqk-vet or custom AST visitor) verifies that test files do not instantiate naked exec.Command without context or testkit managed execution primitives.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-TST-SUBPROCESS-MANAGED-RUNNER",
            "kind": "criteria",
            "title": "Operational Proof: ManagedCommand binds process group pgid and t.Cleanup termination",
            "description": "testkit.ManagedCommand launches child commands in distinct POSIX process groups (Setpgid: true) and registers t.Cleanup hooks that kill the entire process tree on test completion.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL",
            "kind": "criteria",
            "title": "Adversarial Boundary: Subprocess leak detector detects and reaps orphan processes on test timeouts",
            "description": "testkit.VerifyNoSubprocessLeaks and adversarial timeout tests confirm that child processes (including compilers and daemons) are terminated upon context cancellation, leaving zero descendant processes alive.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("criteria", crit_objs)

    # 3. Requirement
    req_objs = [{
        "id": "REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
        "kind": "requirement",
        "title": "Deterministic Subprocess Scoping and Process Reaping Across Test Suites",
        "description": "Audit and enforce strict subprocess lifecycle management across testkit, scheduler, and CLI integration tests so that any spawned process trees are guaranteed to terminate upon test completion, failure, or timeout.",
        "priority": "p1",
        "priority_tier": "P1",
        "status": "originated",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
        "criteria_refs": [
            "CRIT-TST-SUBPROCESS-STATIC-GUARD",
            "CRIT-TST-SUBPROCESS-MANAGED-RUNNER",
            "CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL"
        ],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("requirement", req_objs)

    # 4. Single Unified Test Case (Execution Organizer / Pipeline Container)
    # Ratio REQ:TST = 1:1, CRIT:TST = 3:1
    tst_objs = [
        {
            "id": "TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE",
            "kind": "test_case",
            "title": "Execution Organizer: Subprocess lifecycle multi-stage verification suite",
            "description": "Unified test harness organizing multi-stage verifications: (1) static vet guardrail against unmanaged exec, (2) operational process group lifecycle under testkit.ManagedCommand, and (3) adversarial abort and leak detection assertions.",
            "criteria_refs": [
                "CRIT-TST-SUBPROCESS-STATIC-GUARD",
                "CRIT-TST-SUBPROCESS-MANAGED-RUNNER",
                "CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL"
            ],
            "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
            "path_or_id": "pkg/testkit/subprocess_lifecycle_test.go",
            "scope": "integration",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("test_case", tst_objs)

    # 5. Priority Plan
    pri_plan = {
        "id": "PRI-TST-SUBPROCESS-HYGIENE",
        "kind": "priority_plan",
        "title": "Troubleshoot and Eradicate Orphaned Test Subprocess Leaks",
        "description": "Ontological multi-item execution plan targeting TDE-F-TST-ORPHANED-PROCESS-LEAK across static linter guards, managed process group runners, adversarial leak detectors, and high-risk test call site refactoring.",
        "status": "planned",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-TST-SUBPROCESS-HYGIENE",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T17:15:00Z",
        "updated_at": "2026-09-25T17:15:00Z",
        "created_by": "ACC-1785920548450214012-68b850c0",
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

    # 6. Four Decoupled Backlog Items - all referencing the unified test case container
    bli_objs = [
        {
            "id": "BLI-TST-PROCESS-GROUP-PRIMITIVES",
            "kind": "backlog_item",
            "title": "Implement testkit.ManagedCommand with Setpgid and t.Cleanup process group reaping",
            "description": "Provide a first-class testkit.ManagedCommand runner that configures process group isolation and ensures child and grandchild processes are reaped on test termination.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-TST-SUBPROCESS-HYGIENE",
            "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
            "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "criteria_refs": ["CRIT-TST-SUBPROCESS-MANAGED-RUNNER"],
            "test_case_refs": ["TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-TST-SUBPROCESS-LEAK-DETECTOR",
            "kind": "backlog_item",
            "title": "Build testkit.VerifyNoSubprocessLeaks for adversarial leak assertions",
            "description": "Implement a test utility to inspect descending process trees before and after test execution to assert zero leaked descendant processes.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-TST-SUBPROCESS-HYGIENE",
            "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
            "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "criteria_refs": ["CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL"],
            "test_case_refs": ["TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-TST-REFACTOR-HIGH-RISK-CALLS",
            "kind": "backlog_item",
            "title": "Refactor high-risk unconstrained exec.Command call sites across test suites",
            "description": "Audit and migrate high-risk test call sites (such as pkg/osslaunch/launch_prep_test.go dry-compile and pkg/scheduler daemon tests) to use managed commands with explicit timeouts.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-TST-SUBPROCESS-HYGIENE",
            "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
            "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "criteria_refs": ["CRIT-TST-SUBPROCESS-MANAGED-RUNNER"],
            "test_case_refs": ["TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-TST-VET-LINTER-GUARDRAIL",
            "kind": "backlog_item",
            "title": "Add static analysis check in zqk-vet prohibiting unmanaged exec.Command in test files",
            "description": "Introduce an automated linter rule in zqk-vet that flags naked exec.Command invocations in *_test.go files to statically enforce process group hygiene.",
            "status": "planned",
            "priority": "medium",
            "priority_tier": "P1",
            "estimated_effort": "1.5h",
            "priority_plan_ref": "PRI-TST-SUBPROCESS-HYGIENE",
            "milestone_refs": ["MIL-TEST-HARNESS-ROBUSTNESS"],
            "requirement_refs": ["REQ-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "criteria_refs": ["CRIT-TST-SUBPROCESS-STATIC-GUARD"],
            "test_case_refs": ["TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("backlog_item", bli_objs)

    print("✓ Successfully restructured PRI-TST-SUBPROCESS-HYGIENE with REQ:TST=1:1 and CRIT:TST=3:1!")

if __name__ == "__main__":
    main()
