#!/usr/bin/env python3
"""
setup_phase8_kernel_objects.py

Creates Milestone, Priority Plan, Requirements, Criteria, Test Cases, and Backlog Items
for CEF Phase 8 Launch Remediation, establishing full TPM DoD lineage.
"""

import json
import os
import sys
import subprocess

PHASE8_ITEMS = [
    {
        "req_id": "REQ-PHASE8-FULL-STREAM-READ-SAFETY",
        "crit_id": "CRIT-PHASE8-FULL-STREAM-READ-SAFETY",
        "tst_id": "TST-PHASE8-FULL-STREAM-READ-SAFETY",
        "bli_id": "BLI-PHASE8-FULL-STREAM-READ-SAFETY",
        "finding_id": "F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION",
        "title": "Enforce Full Stream Reads in Cache-Bypass Object File Reader",
        "summary": "Replace raw file.Read with io.ReadFull/io.ReadAll in pkg/storage/object_storage_file_helpers_read_impl.go to guarantee complete YAML file retrieval without truncation (F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION).",
        "test_cmd": "go test -race -v ./pkg/storage -run TestReadObjectFile_FullStreamRead",
        "code_path": "pkg/storage/full_stream_read_safety_test.go",
    },
    {
        "req_id": "REQ-PHASE8-WAL-CORRUPT-LINE-QUARANTINE",
        "crit_id": "CRIT-PHASE8-WAL-CORRUPT-LINE-QUARANTINE",
        "tst_id": "TST-PHASE8-WAL-CORRUPT-LINE-QUARANTINE",
        "bli_id": "BLI-PHASE8-WAL-CORRUPT-LINE-QUARANTINE",
        "finding_id": "F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE",
        "title": "Quarantine Corrupted WAL Entries During Compaction for Post-Mortem Recovery",
        "summary": "Archive unparseable or damaged records to a timestamped quarantine file during WAL compaction in pkg/storage/wal/object_wal_impl.go rather than silently purging them (F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE).",
        "test_cmd": "go test -race -v ./pkg/storage/wal/...",
        "code_path": "pkg/storage/wal/object_wal_impl.go",
    },
    {
        "req_id": "REQ-PHASE8-TELEMETRY-HISTOGRAM-EXPOSURE",
        "crit_id": "CRIT-PHASE8-TELEMETRY-HISTOGRAM-EXPOSURE",
        "tst_id": "TST-PHASE8-TELEMETRY-HISTOGRAM-EXPOSURE",
        "bli_id": "BLI-PHASE8-TELEMETRY-HISTOGRAM-EXPOSURE",
        "finding_id": "F-OBS-UNEXPOSED-HISTOGRAM-METRICS",
        "title": "Expose Histogram Snapshots and Latency Accessors on Telemetry Tracker",
        "summary": "Implement getter methods for IPC latencies, ghost drift MTTR, and telemetry histograms on Tracker/DefaultTracker in pkg/telemetry/telemetry.go (F-OBS-UNEXPOSED-HISTOGRAM-METRICS).",
        "test_cmd": "go test -race -v ./pkg/telemetry/...",
        "code_path": "pkg/telemetry/telemetry.go",
    },
    {
        "req_id": "REQ-PHASE8-HERMETIC-TEST-ENV",
        "crit_id": "CRIT-PHASE8-HERMETIC-TEST-ENV",
        "tst_id": "TST-PHASE8-HERMETIC-TEST-ENV",
        "bli_id": "BLI-PHASE8-HERMETIC-TEST-ENV",
        "finding_id": "F-TST-DATA-RACE-ENV-MUTATION",
        "title": "Eradicate Direct os.Setenv Mutators from Test Packages in Favor of t.Setenv",
        "summary": "Refactor remaining os.Setenv calls in test files to use t.Setenv for hermetic execution and race prevention (F-TST-DATA-RACE-ENV-MUTATION).",
        "test_cmd": "go test -race -v ./pkg/testservices/... ./pkg/specialization/... ./pkg/zqkenv/...",
        "code_path": "pkg/testservices/services_extended_test.go",
    },
]

def main():
    def import_and_promote(kind_name, objs):
        import_path = f"/tmp/phase8_{kind_name}_import.json"
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

    # 1. Milestone
    milestone_obj = [{
        "id": "MIL-LAUNCH-REMEDIATION-P8",
        "kind": "milestone",
        "title": "CEF Phase 8 Launch Remediation (Storage Stream Safety, WAL Quarantine & Observability)",
        "description": "Phase 8 remediations: full stream read safety in object reader (F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION), WAL corrupted line quarantine (F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE), telemetry histogram accessor exposure (F-OBS-UNEXPOSED-HISTOGRAM-METRICS), and hermetic t.Setenv test adoption (F-TST-DATA-RACE-ENV-MUTATION).",
        "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "status": "originated",
        "priority_tier": "P1",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_and_promote("milestone", milestone_obj)

    # 2. Priority Plan in grooming status so BLIs can attach
    plan_obj = [{
        "id": "PRI-LAUNCH-REMEDIATION-PHASE8",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 8",
        "description": "Phase 8 execution plan targeting full stream read safety (F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION), WAL corrupted line quarantine (F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE), telemetry histogram exposure (F-OBS-UNEXPOSED-HISTOGRAM-METRICS), and hermetic test environment hygiene (F-TST-DATA-RACE-ENV-MUTATION).",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE8",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "status": "grooming",
        "priority_tier": "P1",
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_and_promote("priority_plan", plan_obj)

    # 3. Criteria
    crit_objs = []
    for item in PHASE8_ITEMS:
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
    for item in PHASE8_ITEMS:
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "criteria_refs": [item["crit_id"]],
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P8"],
            "priority": "p1",
            "priority_tier": "P1",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("requirement", req_objs)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE8_ITEMS:
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P8"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE8_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P8"],
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE8",
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "estimated_effort": "2h",
            "priority": "high",
            "priority_tier": "P1",
            "status": "planned",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("backlog_item", bli_objs)

    # 7. Promote Priority Plan to active with active_order
    plan_update = {
        "active_order": 1
    }
    plan_path = "/tmp/phase8_plan_update.json"
    with open(plan_path, "w", encoding="utf-8") as f:
        json.dump(plan_update, f, indent=2)
    subprocess.run(["./bin/zqk", "object", "update", "PRI-LAUNCH-REMEDIATION-PHASE8", "--file", plan_path], check=False)
    if os.path.exists(plan_path):
        os.remove(plan_path)
    subprocess.run(["./bin/zqk", "object", "promote", "PRI-LAUNCH-REMEDIATION-PHASE8"], check=False)
    subprocess.run(["./bin/zqk", "object", "draft", "promote", "--all", "--dry-run=false"], check=False)

    print("✓ Successfully created and promoted all Phase 8 objects!")

if __name__ == "__main__":
    main()
