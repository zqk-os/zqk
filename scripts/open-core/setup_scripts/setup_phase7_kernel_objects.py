#!/usr/bin/env python3
"""
setup_phase7_kernel_objects.py

Creates Milestone, Priority Plan, Requirements, Criteria, Test Cases, and Backlog Items
for CEF Phase 7 Launch Remediation, establishing full TPM DoD lineage.
"""

import json
import os
import sys
import subprocess

PHASE7_ITEMS = [
    {
        "req_id": "REQ-PHASE7-GOROUTINE-POOL-PANIC-SAFETY",
        "crit_id": "CRIT-PHASE7-GOROUTINE-POOL-PANIC-SAFETY",
        "tst_id": "TST-PHASE7-GOROUTINE-POOL-PANIC-SAFETY",
        "bli_id": "BLI-PHASE7-GOROUTINE-POOL-PANIC-SAFETY",
        "finding_id": "F-REL-GOROUTINE-POOL-PANIC-DRAIN",
        "title": "Recover Safely from Task Panics in Goroutinelabels Pool Workers",
        "summary": "Wrap individual task execution in an inner defer/recover block in pkg/goroutinelabels/pool.go so worker goroutines survive task panics and do not starve the pool (F-REL-GOROUTINE-POOL-PANIC-DRAIN).",
        "test_cmd": "go test -race -short -timeout 30s ./pkg/goroutinelabels/...",
        "code_path": "pkg/goroutinelabels/pool_test.go",
    },
    {
        "req_id": "REQ-PHASE7-RELAY-CLEANUP-LIFECYCLE",
        "crit_id": "CRIT-PHASE7-RELAY-CLEANUP-LIFECYCLE",
        "tst_id": "TST-PHASE7-RELAY-CLEANUP-LIFECYCLE",
        "bli_id": "BLI-PHASE7-RELAY-CLEANUP-LIFECYCLE",
        "finding_id": "F-REL-RELAY-CLEANUP-GOROUTINE-LEAK",
        "title": "Add Lifecycle Termination and Queue Bounds to Relay Server Cleanup",
        "summary": "Add context cancellation and Close() method to RelayServer in pkg/relay/server.go, bound per-client payload buffers to prevent unbounded memory growth, and verify termination (F-REL-RELAY-CLEANUP-GOROUTINE-LEAK).",
        "test_cmd": "go test -race -v ./pkg/relay/...",
        "code_path": "pkg/relay/server_test.go",
    },
    {
        "req_id": "REQ-PHASE7-CONTEXT-AWARE-HYGIENE",
        "crit_id": "CRIT-PHASE7-CONTEXT-AWARE-HYGIENE",
        "tst_id": "TST-PHASE7-CONTEXT-AWARE-HYGIENE",
        "bli_id": "BLI-PHASE7-CONTEXT-AWARE-HYGIENE",
        "finding_id": "F-MNT-CONTEXT-SLEEP-HYGIENE",
        "title": "Enforce Context-Aware Interruptible Sleep and Request Context in Listeners",
        "summary": "Replace uninterruptible time.Sleep with context select in pkg/mcp/proxy.go and propagate req.Context() in pkg/mesh/webhook_listener.go (F-MNT-CONTEXT-SLEEP-HYGIENE).",
        "test_cmd": "go test -v ./pkg/mesh/...",
        "code_path": "pkg/mesh/webhook_listener.go",
    },
    {
        "req_id": "REQ-PHASE7-ERROR-UNWRAP-CHAIN",
        "crit_id": "CRIT-PHASE7-ERROR-UNWRAP-CHAIN",
        "tst_id": "TST-PHASE7-ERROR-UNWRAP-CHAIN",
        "bli_id": "BLI-PHASE7-ERROR-UNWRAP-CHAIN",
        "finding_id": "F-MNT-ERR-CHAIN-SEVERANCE",
        "title": "Restore Error Unwrap Chains across CLI Middleware and Operations",
        "summary": "Replace %v and %s formatting verbs with %w error wrapping in auth middleware, git operations, demote/promote commands, and CLI path references (F-MNT-ERR-CHAIN-SEVERANCE).",
        "test_cmd": "go test -v ./cmd/zqk/utility -run TestReadAndParseYAMLFile_ErrorWrapping",
        "code_path": "cmd/zqk/utility/fix_hashes_test.go",
    },
    {
        "req_id": "REQ-PHASE7-SUPPLY-NOTICE-ATTRIBUTION",
        "crit_id": "CRIT-PHASE7-SUPPLY-NOTICE-ATTRIBUTION",
        "tst_id": "TST-PHASE7-SUPPLY-NOTICE-ATTRIBUTION",
        "bli_id": "BLI-PHASE7-SUPPLY-NOTICE-ATTRIBUTION",
        "finding_id": "F-SUPPLY-RELEASE-005",
        "title": "Complete Third-Party Open Source Attribution in NOTICE",
        "summary": "Reconcile go.mod direct dependencies against root NOTICE file, documenting licenses and copyrights for google/go-cmp, joho/godotenv, mitchellh/go-ps, and go.uber.org/goleak (F-SUPPLY-RELEASE-005).",
        "test_cmd": "grep -E '(go-cmp|godotenv|go-ps|goleak)' NOTICE",
        "code_path": "NOTICE",
    },
    {
        "req_id": "REQ-PHASE7-SUPPLY-REPRODUCIBLE-BUILDS",
        "crit_id": "CRIT-PHASE7-SUPPLY-REPRODUCIBLE-BUILDS",
        "tst_id": "TST-PHASE7-SUPPLY-REPRODUCIBLE-BUILDS",
        "bli_id": "BLI-PHASE7-SUPPLY-REPRODUCIBLE-BUILDS",
        "finding_id": "F-SUPPLY-RELEASE-007",
        "title": "Add -trimpath and SOURCE_DATE_EPOCH for Deterministic Reproducible Builds",
        "summary": "Add -trimpath to compilation commands in Makefile and .goreleaser.yaml and support SOURCE_DATE_EPOCH in buildDate injection for reproducible builds (F-SUPPLY-RELEASE-007).",
        "test_cmd": "make -n compile-bin | grep trimpath",
        "code_path": "Makefile",
    },
]

def main():
    def import_and_promote(kind_name, objs):
        import_path = f"/tmp/phase7_{kind_name}_import.json"
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
        "id": "MIL-LAUNCH-REMEDIATION-P7",
        "kind": "milestone",
        "title": "CEF Phase 7 Launch Remediation (Worker Panic Safety, Relay Lifecycle & Error Chains)",
        "description": "Phase 7 remediations: recover from task panics in goroutinelabels worker pool (F-REL-GOROUTINE-POOL-PANIC-DRAIN), terminate relay cleanup loop and bound buffers (F-REL-RELAY-CLEANUP-GOROUTINE-LEAK), context-aware sleep in MCP proxy and webhook listener (F-MNT-CONTEXT-SLEEP-HYGIENE), and restore error wrapping verbs (F-MNT-ERR-CHAIN-SEVERANCE).",
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
        "id": "PRI-LAUNCH-REMEDIATION-PHASE7",
        "kind": "priority_plan",
        "title": "Kernel and Reliability Launch Readiness Remediation - Phase 7",
        "description": "Phase 7 execution plan targeting worker panic safety (F-REL-GOROUTINE-POOL-PANIC-DRAIN), relay server cleanup lifecycle and memory bounds (F-REL-RELAY-CLEANUP-GOROUTINE-LEAK), context-aware interruptible sleep (F-MNT-CONTEXT-SLEEP-HYGIENE), and error wrapping preservation (F-MNT-ERR-CHAIN-SEVERANCE).",
        "branch_name": "integration/PRI-LAUNCH-REMEDIATION-PHASE7",
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
    for item in PHASE7_ITEMS:
        cur_res = subprocess.run(["./bin/zqk", "object", "get", item["crit_id"]], capture_output=True, text=True)
        if cur_res.returncode == 0 and "id: " + item["crit_id"] in cur_res.stdout:
            print(f"Criteria {item['crit_id']} already exists, skipping import.")
            continue
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
    if crit_objs:
        import_and_promote("criteria", crit_objs)

    # 4. Requirements
    req_objs = []
    for item in PHASE7_ITEMS:
        cur_res = subprocess.run(["./bin/zqk", "object", "get", item["req_id"]], capture_output=True, text=True)
        if cur_res.returncode == 0 and "id: " + item["req_id"] in cur_res.stdout:
            print(f"Requirement {item['req_id']} already exists, skipping import.")
            continue
        req_objs.append({
            "id": item["req_id"],
            "kind": "requirement",
            "title": item["title"],
            "description": item["summary"],
            "criteria_refs": [item["crit_id"]],
            "goal_refs": ["GOAL-STARTER-COMMUNITY-001"],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P7"],
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    if req_objs:
        import_and_promote("requirement", req_objs)

    # 5. Test Cases
    tst_objs = []
    for item in PHASE7_ITEMS:
        cur_res = subprocess.run(["./bin/zqk", "object", "get", item["tst_id"]], capture_output=True, text=True)
        if cur_res.returncode == 0 and "id: " + item["tst_id"] in cur_res.stdout:
            print(f"Test case {item['tst_id']} already exists, skipping import.")
            continue
        tst_objs.append({
            "id": item["tst_id"],
            "kind": "test_case",
            "title": f"Automated test for {item['crit_id']}",
            "description": f"Verification execution: {item['test_cmd']}",
            "criteria_refs": [item["crit_id"]],
            "requirement_refs": [item["req_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P7"],
            "path_or_id": item["code_path"],
            "scope": "unit",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    if tst_objs:
        import_and_promote("test_case", tst_objs)

    # 6. Backlog Items
    bli_objs = []
    for item in PHASE7_ITEMS:
        bli_objs.append({
            "id": item["bli_id"],
            "kind": "backlog_item",
            "title": item["title"],
            "description": item["summary"],
            "requirement_refs": [item["req_id"]],
            "criteria_refs": [item["crit_id"]],
            "test_case_refs": [item["tst_id"]],
            "milestone_refs": ["MIL-LAUNCH-REMEDIATION-P7"],
            "priority_plan_ref": "PRI-LAUNCH-REMEDIATION-PHASE7",
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "estimated_effort": "2h",
            "priority": "high",
            "priority_tier": "P1",
            "status": "planned",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        })
    import_and_promote("backlog_item", bli_objs)

    # 7. Promote Priority Plan to in_progress
    plan_update = {
        "status": "in_progress",
        "started_at": "2026-09-25T14:00:00Z"
    }
    plan_path = "/tmp/phase7_plan_update.json"
    with open(plan_path, "w", encoding="utf-8") as f:
        json.dump(plan_update, f, indent=2)
    subprocess.run(["./bin/zqk", "object", "update", "PRI-LAUNCH-REMEDIATION-PHASE7", "--file", plan_path], check=False)
    if os.path.exists(plan_path):
        os.remove(plan_path)
    subprocess.run(["./bin/zqk", "object", "draft", "promote", "--all", "--dry-run=false"], check=False)

    print("✓ Successfully created and promoted all Phase 7 objects!")

if __name__ == "__main__":
    main()
