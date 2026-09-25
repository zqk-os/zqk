#!/usr/bin/env python3
"""
setup_phase5_kernel_objects.py

Manages and verifies Requirements, Criteria, Test Cases, and Backlog Items for
CEF Phase 5 Launch Remediation (PRI-LAUNCH-REMEDIATION-PHASE5), wiring TPM
Definition of Done lineage and priority plan membership.
"""

import json
import os
import sys
import subprocess

PHASE5_ITEMS = [
    {
        "req_id": "REQ-PHASE4-RACE-GOROUTINELABELS",
        "crit_id": "CRIT-PHASE4-RACE-GOROUTINELABELS",
        "tst_id": "TST-PHASE4-RACE-GOROUTINELABELS",
        "bli_id": "BLI-PHASE4-RACE-GOROUTINELABELS",
        "tde_id": "TDE-F-TST-RACE-DETECTOR-EXCLUSION",
        "finding_id": "F-TST-RACE-DETECTOR-EXCLUSION",
        "title": "Eliminate Channel Send/Close Concurrency Data Race in Goroutinelabels Pool",
        "summary": "Synchronize goroutinelabels.Pool.Submit and Stop to prevent concurrent channel send and close operations detected by the Go race detector (F-TST-RACE-DETECTOR-EXCLUSION).",
        "test_cmd": "go test -race -short -timeout 30s ./pkg/goroutinelabels/...",
        "code_path": "pkg/goroutinelabels/pool.go",
    },
    {
        "req_id": "REQ-PHASE4-MCP-HEALTH-FASTPATH",
        "crit_id": "CRIT-PHASE4-MCP-HEALTH-FASTPATH",
        "tst_id": "TST-PHASE4-MCP-HEALTH-FASTPATH",
        "bli_id": "BLI-PHASE4-MCP-HEALTH-FASTPATH",
        "tde_id": "TDE-F-OBS-MCP-FAST-PATH-MASKING",
        "finding_id": "F-OBS-MCP-FAST-PATH-MASKING",
        "title": "Enrich MCP System Health Fast Path with Cached Integrity Status",
        "summary": "Read cached tier1 health data in getSystemHealthDataMCPFast to report genuine system health without latency penalties or false unknown masks (F-OBS-MCP-FAST-PATH-MASKING).",
        "test_cmd": "go test -v ./cmd/zqk/system -run '^(TestGetSystemHealthDataMCPFast_WithCachedSummary|TestGetSystemHealthDataMCPFast_WithoutCache)'",
        "code_path": "cmd/zqk/system/mcp_fast_health_test.go",
    },
    {
        "req_id": "REQ-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "crit_id": "CRIT-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "tst_id": "TST-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "bli_id": "BLI-PHASE4-LIBRARY-PANIC-ELIMINATION",
        "tde_id": "TDE-F-MNT-PANIC-IN-LIBRARIES",
        "finding_id": "F-MNT-PANIC-IN-LIBRARIES",
        "title": "Eradicate Uncontrolled Library Panics in MCP and Kernel Compose Subsystems",
        "summary": "Refactor library functions in pkg/mcp and pkg/kernelcas/compose to return fail-closed errors or safe fallbacks instead of crashing daemons with panic (F-MNT-PANIC-IN-LIBRARIES).",
        "test_cmd": "go test -v ./pkg/mcp/...",
        "code_path": "pkg/mcp/permission_format_helpers.go",
    },
    {
        "req_id": "REQ-PHASE4-CLONES-DUAL-TREE",
        "crit_id": "CRIT-PHASE4-CLONES-DUAL-TREE",
        "tst_id": "TST-PHASE4-CLONES-DUAL-TREE",
        "bli_id": "BLI-PHASE4-CLONES-DUAL-TREE",
        "tde_id": "TDE-F-MNT-CLONES-DUAL-TREE",
        "finding_id": "F-MNT-CLONES-DUAL-TREE",
        "title": "Prune Orphaned Command Builders Tree and Re-point Codegen Defaults",
        "summary": "Delete 211 unimported duplicate files in pkg/cli/command_builders and update generate-command-builders default output directory (F-MNT-CLONES-DUAL-TREE, F-ARCH-009).",
        "test_cmd": "go test -v ./pkg/zqkdev/...",
        "code_path": "pkg/zqkdev/generate_command_builders.go",
    },
    {
        "req_id": "REQ-PHASE4-METABOLISM-MUTATION-ERRORS",
        "crit_id": "CRIT-PHASE4-METABOLISM-MUTATION-ERRORS",
        "tst_id": "TST-PHASE4-METABOLISM-MUTATION-ERRORS",
        "bli_id": "BLI-PHASE4-METABOLISM-MUTATION-ERRORS",
        "tde_id": "TDE-F-MNT-ERR-BLANK-SUPPRESSION",
        "finding_id": "F-MNT-ERR-BLANK-SUPPRESSION",
        "title": "Preserve and Propagate Kernel Mutation Errors in Metabolism Exhaust",
        "summary": "Check and return errors from RecordKernelMutation in pkg/swarm/metabolism/exhaust.go rather than silently discarding with blank identifier (F-MNT-ERR-BLANK-SUPPRESSION).",
        "test_cmd": "go test -v ./pkg/swarm/metabolism/...",
        "code_path": "pkg/swarm/metabolism/exhaust.go",
    },
]

def main():
    print(f"Verifying {len(PHASE5_ITEMS)} Phase 5 items in Knowledge Kernel...")
    for item in PHASE5_ITEMS:
        print(f"Checking {item['bli_id']} -> {item['req_id']} -> {item['crit_id']} -> {item['tst_id']}")
        res = subprocess.run(["./bin/zqk", "test", "run", item["tst_id"]], capture_output=True, text=True)
        if res.returncode != 0:
            print(f"FAILED test {item['tst_id']}:\n{res.stdout}\n{res.stderr}", file=sys.stderr)
            sys.exit(1)
        print(f"✓ {item['tst_id']} PASSED")
    print("✓ All Phase 5 test cases verified successfully!")

if __name__ == "__main__":
    main()
