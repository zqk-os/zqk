package agentonboard

import (
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
)

// BootPayload returns the regenerable directive written into vendor config files.
// Brand-aware; studio-dense rules belong in packs, not this string.
func BootPayload() string {
	exe := brand.ExecutableName()
	return fmt.Sprintf(`# %s Agent Boot Protocol

## Orchestration & State Management
- **NEVER use vendor-specific subagent tools for orchestration.**
- All multi-agent workflows, task generation, and background processes must be executed natively using the Knowledge Kernel (e.g. %s agent orchestrate, %s scheduler).
- Do not store state, scripts, or loops in local vendor-specific brain directories (like .gemini/ or memory caches). If it's not an object in the kernel graph or an artifact managed by the CLI, it does not exist.

## Session Initialization & Self-Discovery
- **Proactive Kicking-Off:** When starting a new session, or when given ambiguous direction, do NOT ask the user for what to do. Independently run %s workflow whats-next (or ./bin/%s workflow whats-next) to self-discover mission, priority plan, and constraints from the knowledge kernel.
- **Mission Initialization (Inquiry Probe):** If the kernel reveals that mission, vision, or goals are undefined, run an inquiry probe with the human. Keep agentic knowledge rooted in the kernel.

## First contact
- Prefer `+"`"+`%s system agent-onboard`+"`"+` to detect your agent host, seed default seating, and re-prime these directives from the kernel.
- Community first-run: docs/onboarding/COMMUNITY_FIRST_RUN.md
- Studio-dense process guide (pack): docs/onboarding/AI_AGENT_ONBOARDING.md

## Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)
- **Summary-as-Terminal Failure Mode Prohibition:** In the autonomous CAP loop, merging a PR, promoting binaries, or rendering an artifact summary is a MILESTONE TRANSITION, NOT a stopping condition.
- **NEVER yield control or go idle at summary milestones.** In this agent platform, stopping tool calls immediately transitions the agent into `+"`"+`waiting_for_input`+"`"+` (idle), halting autonomous loop flow.
- **Strict Post-Merge Self-Continuation Protocol:**
  1. Merge PR & promote stable binary (`+"`"+paths.RewriteCanonicalCLIInvocations(`./scripts/install.sh && ./bin/zqk mcp restart`)+"`"+`).
  2. Query `+"`"+paths.RewriteCanonicalCLIInvocations(`./bin/zqk workflow whats-next`)+"`"+`.
  3. Immediately create/checkout the next integration branch (`+"`"+`git checkout -b integration/<pri-id> origin/main`+"`"+`).
  4. Claim or shape the first BLI (`+"`"+paths.RewriteCanonicalCLIInvocations(`zqk agent claim-work ...`)+"`"+` or kernel object creation).
  5. Continue the execution chain without yielding control to an idle wait state.
`, brand.ProductName(), exe, exe, exe, exe, exe)
}

// SyncReportRelPath is the workspace→kernel sync artifact (lite file under .zqk/agent-runtime).
var SyncReportRelPath = filepath.ToSlash(paths.AgentRuntimeRel(paths.AgentWorkspaceSyncFile))

// SyncReportSchema identifies the sync report JSON shape.
const SyncReportSchema = "zqk_agent_workspace_sync_v1"

// ResultSchema identifies the agent-onboard command result payload.
const ResultSchema = "zqk_agent_onboard_result_v1"

// Stage names for Result.Stages (stable JSON keys).
const (
	StageDetect         = "detect"
	StageAuth           = "auth"
	StageSeat           = "seat"
	StagePrimeWorkspace = "prime_workspace"
	StagePrimeKernel    = "prime_kernel"
	StageSmoke          = "smoke"
)

// Stage status values.
const (
	StageOK      = "ok"
	StageWarn    = "warn"
	StageSkipped = "skipped"
	StageFailed  = "failed"
)

// ResultStatus values for Result.Status.
const (
	ResultSuccess = "success"
	ResultBlocked = "blocked"
	ResultFailed  = "failed"
)
