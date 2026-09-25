#!/usr/bin/env python3
"""
setup_phase10_kernel_objects.py
"""

import json
import os
import sys
import subprocess

PHASE10_ITEMS = [
    {
        "req_id": "REQ-PHASE10-TDD-GENUINE-ASSERTIONS",
        "crit_id": "CRIT-PHASE10-TDD-GENUINE-ASSERTIONS",
        "tst_id": "TST-PHASE10-TDD-GENUINE-ASSERTIONS",
        "bli_id": "BLI-PHASE10-TDD-GENUINE-ASSERTIONS",
        "finding_id": "F-TST-TDD-VANITY-STUBS-GAMING",
        "title": "Replace Zero-Assertion TDD Dummy Tests with Genuine Behavioral Assertions",
        "summary": "Replace zero-assertion test stubs and vanity placeholders in cmd/zqk/mesh/mesh_test.go, pkg/swarm/executor_test.go, and pkg/objects/field_keys_test.go with genuine behavioral assertions (F-TST-TDD-VANITY-STUBS-GAMING).",
        "test_cmd": "go test -race -v ./cmd/zqk/mesh -run TestMesh",
        "code_path": "cmd/zqk/mesh/mesh_test.go",
    },
    {
        "req_id": "REQ-PHASE10-SPDX-HEADER-COMPLIANCE",
        "crit_id": "CRIT-PHASE10-SPDX-HEADER-COMPLIANCE",
        "tst_id": "TST-PHASE10-SPDX-HEADER-COMPLIANCE",
        "bli_id": "BLI-PHASE10-SPDX-HEADER-COMPLIANCE",
        "finding_id": "F-SUPPLY-RELEASE-006",
        "title": "Establish SPDX-License-Identifier and Apache-2.0 License Header Compliance",
        "summary": "Add standard SPDX-License-Identifier and Apache-2.0 copyright headers to core package files starting with pkg/bufferpool and add automated header verification (F-SUPPLY-RELEASE-006).",
        "test_cmd": "go test -race -v ./pkg/bufferpool -run TestLicenseHeaders",
        "code_path": "pkg/bufferpool/pool_test.go",
    },
    {
        "req_id": "REQ-PHASE10-INTEGRATION-TEST-WIRING",
        "crit_id": "CRIT-PHASE10-INTEGRATION-TEST-WIRING",
        "tst_id": "TST-PHASE10-INTEGRATION-TEST-WIRING",
        "bli_id": "BLI-PHASE10-INTEGRATION-TEST-WIRING",
        "finding_id": "F-TST-ORPHANED-INTEGRATION-BUILD-TAGS",
        "title": "Reconcile Concurrency Windowing and Wire Tagged Integration Tests into Build Targets",
        "summary": "Fix window overlap collision in pkg/storage/adversarial_concurrency_test.go and wire Go integration test targets into Makefile to prevent orphaned build-tagged tests (F-TST-ORPHANED-INTEGRATION-BUILD-TAGS).",
        "test_cmd": "go test -race -v -tags integration ./pkg/storage -run TestAdversarialConcurrency",
        "code_path": "pkg/storage/adversarial_concurrency_test.go",
    },
    {
        "req_id": "REQ-PHASE10-WAL-SPECIFIC-ERROR-ASSERTIONS",
        "crit_id": "CRIT-PHASE10-WAL-SPECIFIC-ERROR-ASSERTIONS",
        "tst_id": "TST-PHASE10-WAL-SPECIFIC-ERROR-ASSERTIONS",
        "bli_id": "BLI-PHASE10-WAL-SPECIFIC-ERROR-ASSERTIONS",
        "finding_id": "F-TST-GENERIC-ERROR-ASSERTIONS",
        "title": "Enforce Specific Error Type and Value Assertions in WAL Tests",
        "summary": "Assert exact error types and sentinel error values across pkg/storage/wal/extra_coverage_test.go with zero generic err != nil checks to eliminate spurious negative test passes (F-TST-GENERIC-ERROR-ASSERTIONS).",
        "test_cmd": "go test -race -v ./pkg/storage/wal -run TestExtraCoverage_WALSpecificErrors",
        "code_path": "pkg/storage/wal/extra_coverage_test.go",
    },
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
    # Test Cases
    tst_objs = []
    for item in PHASE10_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P10"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("test_case", tst_objs)

    # Backlog Items
    bli_objs = []
    for item in PHASE10_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2h",
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE10",
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P10"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_objects("backlog_item", bli_objs)

    print("Phase 10 test_case and backlog_item setup complete!")

if __name__ == "__main__":
    main()
