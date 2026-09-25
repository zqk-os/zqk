#!/usr/bin/env python3
"""
setup_phase14_kernel_objects.py
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

PHASE14_ITEMS = [
    {
        "req_id": "REQ-PHASE14-TYPE-ASSERT-REMEDY",
        "crit_id": "CRIT-PHASE14-TYPE-ASSERT-REMEDY",
        "tst_id": "TST-PHASE14-TYPE-ASSERT-REMEDY",
        "bli_id": "BLI-PHASE14-TYPE-ASSERT-REMEDY",
        "finding_id": "TDE-F-ARCH-005",
        "title": "Type-Safe Domain Access Veneer to Prevent Dynamic Type Cast Panics",
        "summary": "Deploy KOI (Kernel Object Inspector) type safe accessors to eliminate unsafe runtime .(string) type assertions and nil-dereference risks across domain maps (F-ARCH-005).",
        "test_cmd": "go test -v ./pkg/objects/koi -run TestTypeAssertRemediation",
        "code_path": "pkg/objects/koi/type_assert_remedy_test.go",
    },
    {
        "req_id": "REQ-PHASE14-PACKAGE-CONSOLIDATION",
        "crit_id": "CRIT-PHASE14-PACKAGE-CONSOLIDATION",
        "tst_id": "TST-PHASE14-PACKAGE-CONSOLIDATION",
        "bli_id": "BLI-PHASE14-PACKAGE-CONSOLIDATION",
        "finding_id": "TDE-F-ARCH-006",
        "title": "Package Consolidation Governance and Anti-Atomization Directives",
        "summary": "Establish package consolidation policies and automated 3-tier architectural layering validation in pkg/architecture to prevent micro-package sprawl (F-ARCH-006).",
        "test_cmd": "go test -v ./pkg/architecture -run TestArchitectureLayeringClassification",
        "code_path": "pkg/architecture/governance_test.go",
    },
    {
        "req_id": "REQ-PHASE14-CODEBASE-COMPLEXITY-BUDGET",
        "crit_id": "CRIT-PHASE14-CODEBASE-COMPLEXITY-BUDGET",
        "tst_id": "TST-PHASE14-CODEBASE-COMPLEXITY-BUDGET",
        "bli_id": "BLI-PHASE14-CODEBASE-COMPLEXITY-BUDGET",
        "finding_id": "TDE-F-MNT-MONOLITH-PACKAGE-OUTLIERS",
        "title": "Codebase Complexity Limits and Monolithic Package Outlier Guardrails",
        "summary": "Enforce single-file and package complexity limits and automated line budget tests to prevent monolithic sprawl and file size outliers (F-MNT-MONOLITH-PACKAGE-OUTLIERS).",
        "test_cmd": "go test -v ./pkg/architecture -run TestFileComplexityBudget",
        "code_path": "pkg/architecture/governance_test.go",
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
        "id": "MIL-LAUNCH-REMEDIATION-P14",
        "kind": "milestone",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 14 (Final)",
        "description": "Final launch remediation milestone covering type-safe accessors (F-ARCH-005), package sprawl consolidation governance (F-ARCH-006), and monolithic complexity budgets (F-MNT-MONOLITH-PACKAGE-OUTLIERS).",
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
    for item in PHASE14_ITEMS:
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
    for item in PHASE14_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P14"],
            "criteria_refs": [item["crit_id"]],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("requirement", req_objs)

    # 4. Priority Plan
    pri_plan = {
        "id": "PRI-LAUNCH-REMEDIATION-PHASE14",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 14 (Final)",
        "description": "Phase 14 execution plan targeting KOI type-safe accessors (F-ARCH-005), package consolidation (F-ARCH-006), and complexity budgeting (F-MNT-MONOLITH-PACKAGE-OUTLIERS).",
        "status": "planned",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE14",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T16:35:00Z",
        "updated_at": "2026-09-25T16:35:00Z",
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
    index_data.setdefault("mappings", {})["PRI-LAUNCH-REMEDIATION-PHASE14"] = sha
    with open(index_path, "w") as f:
        json.dump(index_data, f)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE14_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P14"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE14_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE14",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P14"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("backlog_item", bli_objs)

    print("Phase 14 lineage setup complete!")

if __name__ == "__main__":
    main()
