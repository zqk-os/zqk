# Autonomous Single-Command Execution Pipeline (`zqk do`)

## Overview
`zqk do` (aliased as `zqk auto-exec` and `zqk auto-do`) provides a single unified entry point that autonomously executes task workflows without operator friction:

1. **Target Discovery / Resolution**: Automatically resolves the lead shovel-ready backlog item from active priority plans or accepts an explicit `BLI-...` / `ATK-...` identifier.
2. **Atomic Work Claiming**: Atomically sets claimant identity (`--by`), transitions status to `in_progress`, and primes effort metrics.
3. **Context Assembly**: Interrogates governing criteria (`CRIT-...`), requirements (`REQ-...`), and bound skills without polluting object bodies.
4. **TDD Verification & Latching**: Executes test cases linked to the criteria, records verification outcomes, and automatically promotes satisfied criteria and backlog items to `complete`.

## Implementation
- Command: `cmd/zqk/do/do.go`
- Core Engine: `pkg/cli/auto_exec.go`
