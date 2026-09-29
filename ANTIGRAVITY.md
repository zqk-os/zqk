# ZQK Agent Boot Protocol

## Orchestration & State Management
- **NEVER use vendor-specific subagent tools for orchestration.**
- All multi-agent workflows, task generation, and background processes must be executed natively using the Knowledge Kernel (e.g. zqk agent orchestrate, zqk scheduler).
- Do not store state, scripts, or loops in local vendor-specific brain directories (like .gemini/ or memory caches). If it's not an object in the kernel graph or an artifact managed by the CLI, it does not exist.

## Session Initialization & Self-Discovery
- **Proactive Kicking-Off:** When starting a new session, or when given ambiguous direction, do NOT ask the user for what to do. Independently run zqk workflow whats-next (or ./bin/zqk workflow whats-next) to self-discover mission, priority plan, and constraints from the knowledge kernel.
- **Mission Initialization (Inquiry Probe):** If the kernel reveals that mission, vision, or goals are undefined, run an inquiry probe with the human. Keep agentic knowledge rooted in the kernel.

## First contact
- Prefer `zqk system agent-onboard` to detect your agent host, seed default seating, and re-prime these directives from the kernel.
- Community first-run: docs/onboarding/COMMUNITY_FIRST_RUN.md
- Studio-dense process guide (pack): docs/onboarding/AI_AGENT_ONBOARDING.md

## Skill Configuration (Mandatory)
- **Equip the `zqk-expert` skill**: Read and follow instructions in `.agent/skills/zqk-expert/SKILL.md` (or `.zqk/skills/zqk-expert/SKILL.md`).
- Use `zqk-expert` for all Knowledge Kernel operations, VDS done-gates, single-command execution loops (`zqk do`), and fail-closed CLI discipline.

## Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)
- **Summary-as-Terminal Failure Mode Prohibition:** In the autonomous CAP loop, merging a PR, promoting binaries, or rendering an artifact summary is a MILESTONE TRANSITION, NOT a stopping condition.
- **NEVER yield control or go idle at summary milestones.** In this agent platform, stopping tool calls immediately transitions the agent into `waiting_for_input` (idle), halting autonomous loop flow.
- **Strict Post-Merge Self-Continuation Protocol:**
  1. Merge PR & promote stable binary (`./scripts/install.sh && ./bin/zqk mcp restart`).
  2. Query `./bin/zqk workflow whats-next`.
  3. Immediately create/checkout the next integration branch (`git checkout -b integration/<pri-id> origin/main`).
  4. Claim or shape the first BLI (`zqk agent claim-work ...` or kernel object creation).
  5. Continue the execution chain without yielding control to an idle wait state.

## Ambient Signal Interpretation & Action Rubric (Anti-Thrashing Protocol)
- Follow the strict precedence hierarchy: P0 Blockers → P1 Mesh Sync → P2 Active Tasks → P3 Plan Delivery → P4 Runway Replenishment → P5 Hygiene.
- Execute mandated CLI commands and verify with objective validation gates before transitioning tasks. See `docs/architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.md`.

## Universal AGENTS.md
- This file is the headless-safe directive surface (Vector B). IDE rule forests are optional packs.

## Vendor: Agent
- Prefer kernel seating + feed notify for multi-agent wake over vendor-only chat paste.
