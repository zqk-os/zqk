#!/usr/bin/env python3
"""
setup_kernel_steward_loop.py

Mints and imports the complete ontological chain for PRI-KERNEL-STEWARD-LOOP:
- Goal: GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH
- Milestone: MIL-KERNEL-STEWARD-LOOP
- 4 Requirements (REQ:TST = 1:1, CRIT:TST = 3:1):
  1. REQ-KERNEL-INTAKE-CLUSTERING
  2. REQ-KERNEL-EXECUTION-ORGANIZER-MODEL
  3. REQ-KERNEL-SHOCKWAVE-STATE-RESTRICTION
  4. REQ-KERNEL-RUNWAY-REPLENISHMENT-LOOP
- 12 Criteria (3 per requirement: Static Floor, Operational Proof, Negative Invariant)
- 4 Execution Organizer Test Cases (each organizing 3 criteria into verification topologies)
- Priority Plan: PRI-KERNEL-STEWARD-LOOP
- 4 Decoupled Shovel-Ready Backlog Items
"""

import json
import os
import sys
import subprocess
import yaml
import hashlib

def import_objects(kind, objects):
    import_path = f"/tmp/import_steward_{kind}.json"
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
    print("Initiating Ontological Cascade for PRI-KERNEL-STEWARD-LOOP...")

    # 1. Goal
    goal_objs = [{
        "id": "GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH",
        "kind": "goal",
        "title": "Autonomous Kernel Stewardship, Frictionless Intake Clustering, and Execution Pipeline Synthesis",
        "description": "Establish an autonomous, self-sustaining Knowledge Kernel stewardship loop that eradicates 1:1 blind chaining, enforces strict shockwave transition gates, optimizes object topology via composite execution organizers, and maintains at least a 2-plan shovel-ready roadmap buffer.",
        "status": "originated",
        "metric": "Kernel health score, 0 invalid state transitions, zero 1:1 blind chains, and >= 2 planned priority plans",
        "target": "100% health & 0 violations",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("goal", goal_objs)

    # 2. Milestone
    milestone_objs = [{
        "id": "MIL-KERNEL-STEWARD-LOOP",
        "kind": "milestone",
        "title": "Autonomous Kernel Steward Loop & Ontological Quality Framework",
        "description": "Architect and deploy the dedicated Kernel Steward agent loop, two-stage intake clustering membrane, composite execution organizer verification engine, and mutation pipeline shockwave restriction gates.",
        "status": "originated",
        "priority_tier": "P1",
        "goal_refs": ["GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH"],
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "namespace_id": "zqk:kernel",
        "schema_version": "2.0.0"
    }]
    import_objects("milestone", milestone_objs)

    # 3. 12 Criteria (3 per requirement: Static Floor, Operational Proof, Negative Invariant)
    crit_objs = [
        # Req 1: Intake Clustering
        {
            "id": "CRIT-INTAKE-STATIC-SEMANTIC-GROUPING",
            "kind": "criteria",
            "title": "Static Floor: Semantic similarity analyzer groups intake requests into cohesive clusters",
            "description": "Intake analysis engine clusters incoming requests sharing common domain contexts and targets into unified milestones and workstreams rather than isolated 1:1 chains.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-INTAKE-OPERATIONAL-OVERLAP-SCRUTINY",
            "kind": "criteria",
            "title": "Operational Proof: Dependency matrix scrutinizes file and lock overlap for sequence/concurrency",
            "description": "Intake pipeline constructs dependency graphs across target packages, sorting independent tasks into concurrent streams and conflicting mutations into ordered sequential pipelines.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-INTAKE-ADVERSARIAL-1TO1-CHAIN-REJECTION",
            "kind": "criteria",
            "title": "Negative Invariant: Intake membrane rejects redundant 1:1 object micro-chains",
            "description": "Intake validator detects and blocks blind 1:1:1:1:1 object instantiations when proposals duplicate or fragment existing workstreams, requiring structural consolidation.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },

        # Req 2: Execution Organizer Model
        {
            "id": "CRIT-EXEC-ORG-TOPOLOGY-METADATA",
            "kind": "criteria",
            "title": "Static Floor: Test Case schema supports verification topology groupings and multi-criteria binding",
            "description": "Test Case objects define verification topology metadata (sequential, concurrent, hybrid DAG) and bind multiple criteria (CRIT:TST >= 2:1, target 3:1).",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-EXEC-ORG-OPERATIONAL-DISPATCH",
            "kind": "criteria",
            "title": "Operational Proof: Execution organizer dispatches multi-stage verification pipelines",
            "description": "Test dispatcher executes composite test cases according to their topology (parallel verification for isolated criteria, sequential barrier stages for dependent invariants).",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-EXEC-ORG-ADVERSARIAL-ISOLATION-SAFETY",
            "kind": "criteria",
            "title": "Negative Invariant: Concurrent verification branches maintain fault isolation and atomic reporting",
            "description": "A failure, timeout, or panic in one concurrent criteria verification stage does not corrupt sibling execution runners and produces deterministic aggregated diagnostics.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },

        # Req 3: Shockwave State Restriction
        {
            "id": "CRIT-SHOCKWAVE-STATIC-LIFECYCLE-GATES",
            "kind": "criteria",
            "title": "Static Floor: Kernel mutation pipeline defines strict per-kind lifecycle state transition matrices",
            "description": "Object state machine definitions prohibit invalid status jumps (e.g. originated -> in_progress without planned, planned without shovel-ready prerequisites).",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-SHOCKWAVE-OPERATIONAL-RESTRICTION",
            "kind": "criteria",
            "title": "Operational Proof: Shockwave cascade rolls back invalid state transitions atomically",
            "description": "Kernel mutation pipeline intercepts invalid or non-compliant transition mutations, executes clean atomic rollback, and emits actionable error shockwave events.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-SHOCKWAVE-ADVERSARIAL-MUTATION-REJECTION",
            "kind": "criteria",
            "title": "Negative Invariant: Incomplete lineage objects fail closed against promotion gates",
            "description": "Attempting to promote objects with broken lineage (unbound criteria, missing workstream/milestone references) is rejected with fail-closed validation errors.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },

        # Req 4: Runway Replenishment Loop
        {
            "id": "CRIT-RUNWAY-STATIC-BUFFER-MEASUREMENT",
            "kind": "criteria",
            "title": "Static Floor: Kernel health telemetry measures active shovel-ready roadmap runway lead",
            "description": "Kernel health metrics and whats-next telemetry measure the count and depth of shovel-ready priority plans in queue, asserting a minimum buffer of 2 plans ahead.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-RUNWAY-OPERATIONAL-STEWARD-SWEEP",
            "kind": "criteria",
            "title": "Operational Proof: Dedicated kernel steward daemon executes continuous maintenance sweeps",
            "description": "Dedicated kernel steward background loop executes periodic hygiene sweeps (stale lock reaping, WAL compaction, object schema validation, runway grooming).",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "CRIT-RUNWAY-ADVERSARIAL-STARVATION-ALERT",
            "kind": "criteria",
            "title": "Negative Invariant: Low runway buffer triggers proactive ambient replenishment signal",
            "description": "When shovel-ready runway falls below 2 plans, the steward loop raises an ambient replenishment signal (P4 Runway Replenishment) rather than allowing the agent loop to stall.",
            "status": "originated",
            "category": "functional",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("criteria", crit_objs)

    # 4. 4 Requirements
    req_objs = [
        {
            "id": "REQ-KERNEL-INTAKE-CLUSTERING",
            "kind": "requirement",
            "title": "Two-Stage Intake Membrane: Semantic Clustering and Conflicting Overlap Scrutiny",
            "description": "Implement a two-stage intake membrane that analyzes incoming work items for semantic commonality and package-level overlap, grouping related requests into cohesive workstreams and sequencing concurrent vs serial execution pipelines instead of blind 1:1 micro-chaining.",
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "criteria_refs": [
                "CRIT-INTAKE-STATIC-SEMANTIC-GROUPING",
                "CRIT-INTAKE-OPERATIONAL-OVERLAP-SCRUTINY",
                "CRIT-INTAKE-ADVERSARIAL-1TO1-CHAIN-REJECTION"
            ],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "REQ-KERNEL-EXECUTION-ORGANIZER-MODEL",
            "kind": "requirement",
            "title": "Composite Execution Organizer Pattern for Criteria Verification Topologies",
            "description": "Structure test cases as composite Execution Organizers that define verification topologies (sequential, concurrent, hybrid DAG) and bind multiple criteria (CRIT:TST >= 2:1, target 3:1), eliminating fragmented single-verification test cases.",
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "criteria_refs": [
                "CRIT-EXEC-ORG-TOPOLOGY-METADATA",
                "CRIT-EXEC-ORG-OPERATIONAL-DISPATCH",
                "CRIT-EXEC-ORG-ADVERSARIAL-ISOLATION-SAFETY"
            ],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "REQ-KERNEL-SHOCKWAVE-STATE-RESTRICTION",
            "kind": "requirement",
            "title": "Strict Shockwave & Mutation Pipeline Invariant Enforcement on State Transitions",
            "description": "Enforce strict per-kind lifecycle state transition matrices in the kernel mutation pipeline, ensuring shockwave events and mutation validators prevent incomplete or improperly structured objects from advancing across lifecycle boundaries.",
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "criteria_refs": [
                "CRIT-SHOCKWAVE-STATIC-LIFECYCLE-GATES",
                "CRIT-SHOCKWAVE-OPERATIONAL-RESTRICTION",
                "CRIT-SHOCKWAVE-ADVERSARIAL-MUTATION-REJECTION"
            ],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "REQ-KERNEL-RUNWAY-REPLENISHMENT-LOOP",
            "kind": "requirement",
            "title": "Autonomous Dedicated Kernel Steward Loop and 2-Plan Runway Replenishment",
            "description": "Deploy a dedicated background agent process loop solely focused on kernel stewardship, hygiene sweeps, and continuous roadmap replenishment to maintain at least a 2-plan shovel-ready buffer to prevent pipeline starvation.",
            "priority": "p1",
            "priority_tier": "P1",
            "status": "originated",
            "goal_refs": ["GOAL-KERNEL-STEWARDSHIP-CONTINUOUS-HEALTH"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "criteria_refs": [
                "CRIT-RUNWAY-STATIC-BUFFER-MEASUREMENT",
                "CRIT-RUNWAY-OPERATIONAL-STEWARD-SWEEP",
                "CRIT-RUNWAY-ADVERSARIAL-STARVATION-ALERT"
            ],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("requirement", req_objs)

    # 5. 4 Execution Organizer Test Cases (REQ:TST = 1:1, CRIT:TST = 3:1)
    tst_objs = [
        {
            "id": "TST-KERNEL-INTAKE-CLUSTERING-ORGANIZER",
            "kind": "test_case",
            "title": "Execution Organizer: Intake Clustering & Overlap Scrutiny Test Suite",
            "description": "Composite test harness verifying: (1) static semantic grouping, (2) operational dependency matrix sorting (concurrent vs sequential), and (3) adversarial rejection of redundant 1:1 chains.",
            "criteria_refs": [
                "CRIT-INTAKE-STATIC-SEMANTIC-GROUPING",
                "CRIT-INTAKE-OPERATIONAL-OVERLAP-SCRUTINY",
                "CRIT-INTAKE-ADVERSARIAL-1TO1-CHAIN-REJECTION"
            ],
            "requirement_refs": ["REQ-KERNEL-INTAKE-CLUSTERING"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "path_or_id": "pkg/kernel/intake/intake_clustering_test.go",
            "scope": "integration",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "TST-KERNEL-EXECUTION-ORGANIZER-SUITE",
            "kind": "test_case",
            "title": "Execution Organizer: Verification Topology & Dispatch Harness",
            "description": "Composite test harness verifying: (1) schema topology validation, (2) multi-stage dispatch execution, and (3) adversarial isolation safety under partial branch failures.",
            "criteria_refs": [
                "CRIT-EXEC-ORG-TOPOLOGY-METADATA",
                "CRIT-EXEC-ORG-OPERATIONAL-DISPATCH",
                "CRIT-EXEC-ORG-ADVERSARIAL-ISOLATION-SAFETY"
            ],
            "requirement_refs": ["REQ-KERNEL-EXECUTION-ORGANIZER-MODEL"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "path_or_id": "pkg/kernel/verification/exec_organizer_test.go",
            "scope": "integration",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "TST-KERNEL-SHOCKWAVE-GATES-SUITE",
            "kind": "test_case",
            "title": "Execution Organizer: Mutation Pipeline Shockwave Gates Verification",
            "description": "Composite test harness verifying: (1) lifecycle state transition matrix validation, (2) operational atomic rollback on invalid mutations, and (3) adversarial fail-closed rejection of incomplete lineage objects.",
            "criteria_refs": [
                "CRIT-SHOCKWAVE-STATIC-LIFECYCLE-GATES",
                "CRIT-SHOCKWAVE-OPERATIONAL-RESTRICTION",
                "CRIT-SHOCKWAVE-ADVERSARIAL-MUTATION-REJECTION"
            ],
            "requirement_refs": ["REQ-KERNEL-SHOCKWAVE-STATE-RESTRICTION"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "path_or_id": "pkg/kernel/mutation/shockwave_gates_test.go",
            "scope": "integration",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "TST-KERNEL-RUNWAY-STEWARD-SUITE",
            "kind": "test_case",
            "title": "Execution Organizer: Dedicated Kernel Steward Loop & Runway Replenishment Suite",
            "description": "Composite test harness verifying: (1) runway depth telemetry assertion (>= 2 plans), (2) operational steward background sweep execution, and (3) adversarial low-buffer replenishment signaling.",
            "criteria_refs": [
                "CRIT-RUNWAY-STATIC-BUFFER-MEASUREMENT",
                "CRIT-RUNWAY-OPERATIONAL-STEWARD-SWEEP",
                "CRIT-RUNWAY-ADVERSARIAL-STARVATION-ALERT"
            ],
            "requirement_refs": ["REQ-KERNEL-RUNWAY-REPLENISHMENT-LOOP"],
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "path_or_id": "pkg/kernel/steward/runway_steward_test.go",
            "scope": "integration",
            "status": "originated",
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("test_case", tst_objs)

    # 6. Priority Plan (PRI-KERNEL-STEWARD-LOOP)
    pri_plan = {
        "id": "PRI-KERNEL-STEWARD-LOOP",
        "kind": "priority_plan",
        "title": "Dedicated Autonomous Kernel Steward Loop and Frictionless Ontological Pipeline",
        "description": "Deploy a dedicated agent-driven process loop solely dedicated to kernel stewardship, two-stage intake semantic clustering, composite execution organizers for verification topologies, and strict mutation pipeline shockwave state transition gates.",
        "status": "active",
        "active_order": 1,
        "priority_tier": "P1",
        "branch_name": "integration/PRI-KERNEL-STEWARD-LOOP",
        "workstream_refs": ["WS-STARTER-COMMUNITY-001"],
        "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
        "created_at": "2026-09-25T18:55:00Z",
        "updated_at": "2026-09-25T18:55:00Z",
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
    index_data.setdefault("mappings", {})["PRI-KERNEL-STEWARD-LOOP"] = sha
    with open(index_path, "w", encoding="utf-8") as f:
        json.dump(index_data, f)

    # 7. 4 Decoupled Shovel-Ready Backlog Items
    bli_objs = [
        {
            "id": "BLI-KERNEL-INTAKE-MEMBRANE",
            "kind": "backlog_item",
            "title": "Implement Two-Stage Intake Membrane with Semantic Clustering and Overlap Scrutiny",
            "description": "Construct the intake membrane module that performs semantic clustering of incoming work requests and package dependency overlap analysis to sequence tasks into concurrent vs serial tracks, eliminating blind 1:1 chaining.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "3h",
            "priority_plan_ref": "PRI-KERNEL-STEWARD-LOOP",
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "requirement_refs": ["REQ-KERNEL-INTAKE-CLUSTERING"],
            "criteria_refs": [
                "CRIT-INTAKE-STATIC-SEMANTIC-GROUPING",
                "CRIT-INTAKE-OPERATIONAL-OVERLAP-SCRUTINY",
                "CRIT-INTAKE-ADVERSARIAL-1TO1-CHAIN-REJECTION"
            ],
            "test_case_refs": ["TST-KERNEL-INTAKE-CLUSTERING-ORGANIZER"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-KERNEL-EXEC-ORGANIZER-ENGINE",
            "kind": "backlog_item",
            "title": "Implement Execution Organizer Schema and Verification Topology Engine",
            "description": "Enhance test case schema and verification dispatch runner to support composite execution topologies (sequential, concurrent, hybrid DAG), organizing multiple criteria verifications into a cohesive execution container.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "3h",
            "priority_plan_ref": "PRI-KERNEL-STEWARD-LOOP",
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "requirement_refs": ["REQ-KERNEL-EXECUTION-ORGANIZER-MODEL"],
            "criteria_refs": [
                "CRIT-EXEC-ORG-TOPOLOGY-METADATA",
                "CRIT-EXEC-ORG-OPERATIONAL-DISPATCH",
                "CRIT-EXEC-ORG-ADVERSARIAL-ISOLATION-SAFETY"
            ],
            "test_case_refs": ["TST-KERNEL-EXECUTION-ORGANIZER-SUITE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-KERNEL-SHOCKWAVE-MUTATION-GATES",
            "kind": "backlog_item",
            "title": "Implement Strict Shockwave Transition Gates & Mutation Pipeline Validators",
            "description": "Implement lifecycle transition enforcement in the kernel mutation pipeline with atomic rollback and diagnostic shockwave events when objects attempt illegal state transitions or have broken lineage.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2.5h",
            "priority_plan_ref": "PRI-KERNEL-STEWARD-LOOP",
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "requirement_refs": ["REQ-KERNEL-SHOCKWAVE-STATE-RESTRICTION"],
            "criteria_refs": [
                "CRIT-SHOCKWAVE-STATIC-LIFECYCLE-GATES",
                "CRIT-SHOCKWAVE-OPERATIONAL-RESTRICTION",
                "CRIT-SHOCKWAVE-ADVERSARIAL-MUTATION-REJECTION"
            ],
            "test_case_refs": ["TST-KERNEL-SHOCKWAVE-GATES-SUITE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        },
        {
            "id": "BLI-KERNEL-RUNWAY-STEWARD-DAEMON",
            "kind": "backlog_item",
            "title": "Deploy Dedicated Kernel Steward Background Daemon and Runway Buffer Monitor",
            "description": "Deploy a dedicated background agent daemon that executes continuous kernel hygiene sweeps, monitors roadmap runway lead (maintaining >= 2 planned priority plans), and raises ambient replenishment signals.",
            "status": "planned",
            "priority": "high",
            "priority_tier": "P1",
            "estimated_effort": "2.5h",
            "priority_plan_ref": "PRI-KERNEL-STEWARD-LOOP",
            "milestone_refs": ["MIL-KERNEL-STEWARD-LOOP"],
            "requirement_refs": ["REQ-KERNEL-RUNWAY-REPLENISHMENT-LOOP"],
            "criteria_refs": [
                "CRIT-RUNWAY-STATIC-BUFFER-MEASUREMENT",
                "CRIT-RUNWAY-OPERATIONAL-STEWARD-SWEEP",
                "CRIT-RUNWAY-ADVERSARIAL-STARVATION-ALERT"
            ],
            "test_case_refs": ["TST-KERNEL-RUNWAY-STEWARD-SUITE"],
            "persona_refs": ["PER-COMMUNITY-SOFTWARE-ENGINEER"],
            "namespace_id": "zqk:kernel",
            "schema_version": "2.0.0"
        }
    ]
    import_objects("backlog_item", bli_objs)

    print("✓ Successfully minted PRI-KERNEL-STEWARD-LOOP with complete intact lineage, REQ:TST=1:1 and CRIT:TST=3:1!")

if __name__ == "__main__":
    main()
