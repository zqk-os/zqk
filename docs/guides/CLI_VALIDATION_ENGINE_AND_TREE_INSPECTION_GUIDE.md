# CLI Validation Engine & Object Tree Inspection Guide

## Overview
The ZQK Validation Engine (`cmd/zqk/validate`) provides comprehensive static, architectural, workflow, and relational tree inspection across the knowledge kernel and repository workspace. It ensures adherence to:
1. **Prescriptive Workflow & Git State (`zqk validate workflow`)**: Branch naming conventions, active item linkage to the current priority plan, clean git working trees, and origin/main base freshness.
2. **Object Tree Inspection (`zqk validate tree`)**: Hierarchical decomposition of Epics -> Priority Plans -> Goals -> Requirements -> Backlog Items -> Test Cases -> Criteria.
3. **AST Command Constructor Integrity (`zqk validate`)**: Enforcement that leaf CLI commands do not wrap and patch other commands, preventing flag mutations and broken CLI taxonomy.
4. **Programmatic Vetting Matrix (`zqk validate matrix`)**: Verification of continuous verification dimensions (HCODE, EFFPERF, ERRHYG, CONCURR, SECOBS, DOCSIG) and tamper-proof matrix ledgers.
5. **Object Machine Hints (`zqk validate object`)**: Verification that kernel objects contain valid machine hints and executable commands.

---

## Architectural Taxonomy

### 1. Object Tree Resolution (`tree.go`)
`zqk validate tree` resolves the graph starting from an active, in-progress, or explicitly supplied priority plan (`--priority-plan <PRI-ID>`).
- **Resolver**: Traverses the relational links (`priority_plan_ref`, `goal_refs`, `requirement_refs`, `test_case_refs`, `criteria_refs`).
- **Safe Type Conversion**: Uses resilient `toStringSlice` conversions to handle both scalar string IDs, string slices (`[]string`), and heterogeneous YAML/JSON interface slices (`[]any`), preventing AST panic regressions during deserialization.
- **Tree Visualization**: Renders structured ASCII trees with configurable depth (`--depth`), displaying statuses, IDs, and titles.

### 2. Workflow Adherence Engine (`workflow.go`)
`zqk validate workflow` scores repository state against five canonical gates:
- **Clean Git Working Tree**: Confirms no unstaged or uncommitted artifacts taint the local workspace (`score -20` if dirty).
- **Plan-Scoped Branch Naming**: Enforces that feature/integration branches reference valid kernel item IDs (`feat/<ID>`, `integration/<PRI-ID>`).
- **Active Priority Plan Membership**: Verifies the claimed item is an explicit child in the currently active priority plan.
- **Sequential In-Plan Ordering**: Flags out-of-order execution if preceding items in the cycle remain open.
- **Main Base Freshness**: Verifies `git merge-base HEAD main` matches the tip of `main`.

### 3. Vetting Matrix & Codebase AST Sensors (`agent.go`, `command_ctor_ast.go`)
- **AST Violations**: AST sensors inspect modified packages for swallowed errors (`_ = ...`), naked prints (`fmt.Print*`), and command builder antipatterns.
- **Verification Gates**: Runs tests with strict coverage invariants, generating tamper-proof cryptographic matrix ledgers.

---

## Command Reference

### `zqk validate tree`
Displays hierarchical tree of priority plans, goals, requirements, backlogs, and criteria.
```bash
# Auto-resolve currently active or in-progress priority plan
zqk validate tree

# Inspect specific priority plan with custom depth
zqk validate tree --priority-plan PRI-1792008359177241000-141f7d25 --depth 4
```

### `zqk validate workflow`
Validates developer and agent adherence to the prescriptive workflow.
```bash
zqk validate workflow
```

### `zqk validate object`
Validates schema conformance and machine hints for a specific object.
```bash
zqk validate object BLI-1792008339820626500-98423dfa
```

### `zqk validate matrix`
Validates vetting matrix ledgers across the repository.
```bash
zqk validate matrix
```

---

## Test Verification & Quality Gates
All validation engine commands are guarded by unit and integration tests under `cmd/zqk/validate`:
- Statement test coverage: **>=80.0%** (achieved: **81.9%**).
- Hermetic filesystem isolation via `testkit.PrepareIsolatedTempProject`.
- Memory workflow stores for sub-second deterministic relational graph resolution.
