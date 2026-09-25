#!/usr/bin/env python3
"""
setup_phase12_kernel_objects.py
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

PHASE12_ITEMS = [
    {
        "req_id": "REQ-PHASE12-SUPPLY-PROVENANCE",
        "crit_id": "CRIT-PHASE12-SUPPLY-PROVENANCE",
        "tst_id": "TST-PHASE12-SUPPLY-PROVENANCE",
        "bli_id": "BLI-PHASE12-SUPPLY-PROVENANCE",
        "finding_id": "TDE-F-SUPPLY-RELEASE-009",
        "title": "Release Pipeline Provenance, Cosign Signatures, and SPDX SBOM Verification",
        "summary": "Add Cosign keyless signatures and SPDX SBOM generation to GoReleaser and release workflow, and enforce signature validation in install scripts (F-SUPPLY-RELEASE-009).",
        "test_cmd": "go test -v ./pkg/supply -run TestSupplyChainReleaseIntegrity",
        "code_path": "pkg/supply/supply_test.go",
    },
    {
        "req_id": "REQ-PHASE12-PACKAGE-STUTTER",
        "crit_id": "CRIT-PHASE12-PACKAGE-STUTTER",
        "tst_id": "TST-PHASE12-PACKAGE-STUTTER",
        "bli_id": "BLI-PHASE12-PACKAGE-STUTTER",
        "finding_id": "TDE-F-MNT-NAMING-PACKAGE-STUTTER",
        "title": "Eliminate Core Domain Package Name Stutter in Pipeline Package",
        "summary": "Refactor repetitive type names in pkg/pipeline (PipelineRouter, PipelinePlugin, PipelineStorageProvider) to idiomatic Go aliases (PlanRouter, Plugin, StorageProvider) (F-MNT-NAMING-PACKAGE-STUTTER).",
        "test_cmd": "go test -v ./pkg/pipeline -run TestPackageStutterElimination",
        "code_path": "pkg/pipeline/stutter_test.go",
    },
    {
        "req_id": "REQ-PHASE12-LOCK-TAXONOMY",
        "crit_id": "CRIT-PHASE12-LOCK-TAXONOMY",
        "tst_id": "TST-PHASE12-LOCK-TAXONOMY",
        "bli_id": "BLI-PHASE12-LOCK-TAXONOMY",
        "finding_id": "TDE-F-ARCH-010",
        "title": "Establish Explicit Lock Registry Scope Taxonomy and Mutex Hierarchy Guidelines",
        "summary": "Formalize lock scope prefixes, documentation of lock domains, and architectural boundary guidelines distinguishing storage coordination from local struct-level sync.Mutexes (F-ARCH-010).",
        "test_cmd": "go test -v ./pkg/storage/locknames -run TestLockHierarchyDocumentation",
        "code_path": "pkg/storage/locknames/locknames_test.go",
    },
    {
        "req_id": "REQ-PHASE12-STORAGE-CONTRACT-HARMONY",
        "crit_id": "CRIT-PHASE12-STORAGE-CONTRACT-HARMONY",
        "tst_id": "TST-PHASE12-STORAGE-CONTRACT-HARMONY",
        "bli_id": "BLI-PHASE12-STORAGE-CONTRACT-HARMONY",
        "finding_id": "TDE-F-ARCH-003",
        "title": "Clarify Storage Layer ObjectStorageProvider Contract Boundaries and Error Uniformity",
        "summary": "Harmonize ObjectStorageProvider interface expectations between pkg/storage/filecas and pkg/storage, specifying ErrObjectNotFound conventions and CAS byte-level vs domain-level responsibilities (F-ARCH-003).",
        "test_cmd": "go test -v ./pkg/storage/filecas -run TestErrObjectNotFound",
        "code_path": "pkg/storage/filecas/interfaces_test.go",
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
        "id": "MIL-LAUNCH-REMEDIATION-P12",
        "kind": "milestone",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 12",
        "description": "Remediation milestone for Phase 12 covering release provenance signatures, pipeline package stutter elimination, lock registry taxonomy, and storage contract harmony.",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("milestone", milestone)

    # 2. Criteria (kind: criteria)
    crit_objs = []
    for item in PHASE12_ITEMS:
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
    for item in PHASE12_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P12"],
            "criteria_refs": [item["crit_id"]],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("requirement", req_objs)

    # 4. Priority Plan
    pri_plan = {
        "id": "PRI-LAUNCH-REMEDIATION-PHASE12",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 12",
        "description": "Phase 12 execution plan targeting release provenance (F-SUPPLY-RELEASE-009), pipeline package stutter (F-MNT-NAMING-PACKAGE-STUTTER), lock registry taxonomy (F-ARCH-010), and storage contract harmony (F-ARCH-003).",
        "status": "planned",
        "priority_tier": "P1",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE12",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T16:10:00Z",
        "updated_at": "2026-09-25T16:10:00Z",
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
    index_data.setdefault("mappings", {})["PRI-LAUNCH-REMEDIATION-PHASE12"] = sha
    with open(index_path, "w") as f:
        json.dump(index_data, f)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE12_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P12"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE12_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE12",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P12"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("backlog_item", bli_objs)

    print("Phase 12 lineage setup complete!")

if __name__ == "__main__":
    main()
