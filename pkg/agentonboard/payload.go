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
- **Context-Bound Subagent Orchestration:** Do not use any subagent tools for orchestration without ensuring proper initialization context is included. %s makes this easy with tooling targeting lean, adequate context provision for prompt generation (e.g. %s agent orchestrate, %s agent prepare-context)—attempting to orchestrate with inadequate or irrelevant context is where execution fails.
- All multi-agent workflows, task generation, and background processes must be executed natively using the Knowledge Kernel (e.g. %s agent orchestrate, %s scheduler).
- Do not store state, scripts, or loops in local vendor-specific brain directories (like .gemini/ or memory caches). If it's not an object in the kernel graph or an artifact managed by the CLI, it does not exist.

## Session Initialization & Self-Discovery
- **Proactive Kicking-Off:** When starting a new session, or when given ambiguous direction, do NOT ask the user for what to do. Independently run %s workflow whats-next (or ./bin/%s workflow whats-next) to self-discover mission, priority plan, and constraints from the knowledge kernel.
- **Mission Initialization (Inquiry Probe):** If the kernel reveals that mission, vision, or goals are undefined, run an inquiry probe with the human. Keep agentic knowledge rooted in the kernel.

## First contact
- Prefer `+"`"+`%s system agent-onboard`+"`"+` to detect your agent host, seed default seating, and re-prime these directives from the kernel.
- Community first-run: docs/onboarding/COMMUNITY_FIRST_RUN.md
- Studio-dense process guide (pack): docs/onboarding/AI_AGENT_ONBOARDING.md

## Paired Onboarding & Intent Discovery Protocol
- **Interactive First-Run Engagement:** When starting on a greenfield project or when the active plan is PRI-STARTER-COMMUNITY-001, do NOT execute commands silently in the background. Conduct an interactive, paired walkthrough with the human operator.
- **Articulate the Kernel Advantage:** Explain why %s is fundamentally different from transient prompt-promiscuous chat harnesses: goals, requirements, criteria, test cases, and backlog items are versioned, verifiable graph objects in a persistent Knowledge Kernel rather than lost LLM context.
- **Intent Discovery & Modern Objectification:**
  1. Prompt the user for their core initializing intent or a small feature to build.
  2. Reassure the human that goals, vision, and plans can always be evolved or modified later before execution lock.
  3. Walk through each kernel object minted using modern `+"`"+`%s new <kind>`+"`"+` (e.g. `+"`"+`%s new goal`+"`"+`, `+"`"+`%s new plan`+"`"+`, `+"`"+`%s new req`+"`"+`, `+"`"+`%s new bli`+"`"+`)—never use low-level object create commands.
  4. Explain the 5-layer cascade: Vision/Mission → Goals → Priority Plans → Requirements/Criteria → Backlog Items.
- **Fast Path to Autonomous Swarm Value:** Guide the user to immediately experience multi-agent execution via `+"`"+`%s do`+"`"+` (or `+"`"+`%s agent orchestrate`+"`"+`), demonstrating concurrent, token-budgeted, AST-verified delivery.

## Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)
- **Summary-as-Terminal Failure Mode Prohibition:** In the autonomous CAP loop, merging a PR, promoting binaries, or rendering an artifact summary is a MILESTONE TRANSITION, NOT a stopping condition.
- **NEVER yield control or go idle at summary milestones.** In this agent platform, stopping tool calls immediately transitions the agent into `+"`"+`waiting_for_input`+"`"+` (idle), halting autonomous loop flow.
- **Strict Post-Merge Self-Continuation Protocol:**
  1. Merge PR & promote stable binary (`+"`"+paths.RewriteCanonicalCLIInvocations(`./scripts/install.sh && ./bin/zqk mcp ensure`)+"`"+`).
  2. Query `+"`"+paths.RewriteCanonicalCLIInvocations(`./bin/zqk workflow whats-next`)+"`"+`.
  3. Immediately create/checkout the next integration branch (`+"`"+`git checkout -b integration/<pri-id> origin/main`+"`"+`).
  4. Claim or shape the first BLI (`+"`"+paths.RewriteCanonicalCLIInvocations(`zqk agent claim ...`)+"`"+` or kernel object creation).
  5. Continue the execution chain without yielding control to an idle wait state.

## Chat Issue Intake & TPM Anti-Distraction Protocol (POL-AGENT-TPM-INTAKE-FIREWALL-001)
- **The Chat-to-Kernel Firewall:** When an operator or chat message reports a bug, test failure, stack trace, or unexpected behavior, the TPM MUST NOT drop the active plan or begin writing code and running tests directly.
- **Mandatory 3-Step Triage Sequence:**
  1. **Objectify Immediately (Zero Code Written):** Mint a persistent Knowledge Kernel object to capture the report:
     - Test failures, flakes, performance, or hygiene issues: `+"`"+paths.RewriteCanonicalCLIInvocations(`zqk new technical_debt --title "<symptom>"`)+"`"+`
     - Missing functionality, scope gaps, or requirement defects: `+"`"+paths.RewriteCanonicalCLIInvocations(`zqk new backlog_item --title "<symptom>"`)+"`"+`
  2. **Triage & Route:**
     - **P0 (Blocks Current Active Priority Plan):** Attach to active plan as a blocker; delegate the fix to an isolated worker subagent or swarm (`+"`"+paths.RewriteCanonicalCLIInvocations(`zqk agent orchestrate`)+"`"+` / IC craftsman) with the error log. The TPM monitors the gate but NEVER writes code or runs tests.
     - **P1/P2 (Non-blocking):** File into the backlog or upcoming cycle.
  3. **Resume Program Trajectory:** Immediately output the minted object receipt (TD-xxx / BLI-xxx), query `+"`"+paths.RewriteCanonicalCLIInvocations(`./bin/zqk workflow whats-next`)+"`"+`, and continue tracking the active priority plan without yielding or getting distracted.

## Code Search & Token Conservation (`+"`"+`%s grep`+"`"+`)
- **Prefer `+"`"+`%s grep`+"`"+` (alias `+"`zgrep`"+`) over raw shell `+"`grep`"+` or `+"`find`"+`:** `+"`"+`%s grep`+"`"+` provides sub-15ms trigram indexing, Go AST structural queries (`+"`--ast --kind struct|func`"+`, `+"`--ast --recv <Type>`"+`), and strict token budgeting (`+"`--max-tokens 2000 -f json`"+`). Using external grep dumps unbudgeted files into LLM contexts and increases token consumption.
`, brand.ProductName(), brand.ProductName(), exe, exe, exe, exe, exe, exe, exe, brand.ProductName(), exe, exe, exe, exe, exe, exe, exe, exe, exe, exe)
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
