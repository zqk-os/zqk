# Autonomous Single-Command Execution Pipeline (`zqk do`)

## Overview
`zqk do` (aliased as `zqk auto-exec` and `zqk auto-do`) provides a single unified entry point that autonomously executes task workflows without operator friction:
1. **Target Discovery / Resolution**: Automatically resolves the lead shovel-ready backlog item from active priority plans or accepts an explicit `BLI-...` / `ATK-...` identifier.
2. **Atomic Work Claiming**: Atomically sets claimant identity (`--by`), transitions status to `in_progress`, and primes effort metrics.
3. **Context Assembly**: Interrogates governing criteria (`CRIT-...`), requirements (`REQ-...`), and bound skills without polluting object bodies.
4. **TDD Verification & Latching**: Executes test cases linked to the criteria, records verification outcomes, and automatically promotes satisfied criteria and backlog items to `complete`.

## Traceability
- **Priority Plan**: `PRI-ERGONOMICS-AUTOMATION`
- **Backlog Item**: `BLI-ERGONOMICS-AUTO-EXEC-001`
- **Requirement**: `REQ-ERGONOMICS-AUTO-EXEC-001`
- **Criteria**: `CRIT-ERGONOMICS-AUTO-EXEC-001`
- **Test Case**: `TST-ERGONOMICS-AUTO-EXEC-001`
- **Implementation**: [`cmd/zqk/do/do.go`](file:///Users/lanceettl/zqk-public-candidate/cmd/zqk/do/do.go), [`pkg/cli/auto_exec.go`](file:///Users/lanceettl/zqk-public-candidate/pkg/cli/auto_exec.go)
