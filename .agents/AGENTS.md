# ZQK Agent Boot Protocol

## Orchestration & State Management
- **Context-Bound Subagent Orchestration:** Do not use any subagent tools for orchestration without ensuring proper initialization context is included. ZQK makes this easy with tooling targeting lean, adequate context provision for prompt generation (e.g. zqk agent orchestrate, zqk agent prepare-context)—attempting to orchestrate with inadequate or irrelevant context is where execution fails.
- All multi-agent workflows, task generation, and background processes must be executed natively using the Knowledge Kernel (e.g. zqk agent orchestrate, zqk scheduler).
- Do not store state, scripts, or loops in local vendor-specific brain directories (like .gemini/ or memory caches). If it's not an object in the kernel graph or an artifact managed by the CLI, it does not exist.

## Session Initialization & Self-Discovery
- **Proactive Kicking-Off:** When starting a new session, or when given ambiguous direction, do NOT ask the user for what to do. Independently run zqk workflow whats-next (or ./bin/zqk workflow whats-next) to self-discover mission, priority plan, and constraints from the knowledge kernel.
- **Mission Initialization (Inquiry Probe):** If the kernel reveals that mission, vision, or goals are undefined, run an inquiry probe with the human. Keep agentic knowledge rooted in the kernel.

## First contact
- Prefer `zqk system agent-onboard` to detect your agent host, seed default seating, and re-prime these directives from the kernel.
- Community first-run: docs/onboarding/COMMUNITY_FIRST_RUN.md
- Studio-dense process guide (pack): docs/onboarding/AI_AGENT_ONBOARDING.md

## Paired Onboarding & Intent Discovery Protocol
- **Interactive First-Run Engagement:** When starting on a greenfield project or when the active plan is PRI-STARTER-COMMUNITY-001, do NOT execute commands silently in the background. Conduct an interactive, paired walkthrough with the human operator.
- **Articulate the Kernel Advantage:** Explain why ZQK is fundamentally different from transient prompt-promiscuous chat harnesses (Cursor Composer, Claude Code, Devin): goals, requirements, criteria, test cases, and backlog items are versioned, verifiable graph objects in a persistent Knowledge Kernel rather than lost LLM context.
- **Intent Discovery & Modern Objectification:**
  1. Prompt the user for their core initializing intent or a small feature to build.
  2. Reassure the human that goals, vision, and plans can always be evolved or modified later before execution lock.
  3. Walk through each kernel object minted using modern `zqk new <kind>` (e.g. `zqk new goal`, `zqk new plan`, `zqk new req`, `zqk new bli`)—never use low-level object create commands.
  4. Explain the 5-layer cascade: Vision/Mission → Goals → Priority Plans → Requirements/Criteria → Backlog Items.
- **Fast Path to Autonomous Swarm Value:** Guide the user to immediately experience multi-agent execution via `zqk do` (or `zqk agent orchestrate`), demonstrating concurrent, token-budgeted, AST-verified delivery.

## Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)
- **Summary-as-Terminal Failure Mode Prohibition:** In the autonomous CAP loop, merging a PR, promoting binaries, or rendering an artifact summary is a MILESTONE TRANSITION, NOT a stopping condition.
- **NEVER yield control or go idle at summary milestones.** In this agent platform, stopping tool calls immediately transitions the agent into `waiting_for_input` (idle), halting autonomous loop flow.
- **Strict Post-Merge Self-Continuation Protocol:**
  1. Merge PR & promote stable binary (`./scripts/install.sh && ./bin/zqk mcp ensure`).
  2. Query `./bin/zqk workflow whats-next`.
  3. Immediately create/checkout the next integration branch (`git checkout -b integration/<pri-id> origin/main`).
  4. Claim or shape the first BLI (`zqk agent claim ...` or kernel object creation).
  5. Continue the execution chain without yielding control to an idle wait state.

## Process Administration & Management: Kernel Objects (Shift-Left Directive)
- **Shift-Left Upfront Discipline:** Do not wait for `zqk-vet` or git commit check-valves to catch bad slicing or scope creep. Agents must follow the canonical relational model out of the gate:
  - **Timeless Strategic & Functional Contracts:**
    - **`goal` (Timeless Strategic Compass):** Anchors high-level intent outside of any sprint, schedule, or calendar constraint. Decomposes into 3–5 `requirement` contracts (child `requirement.goal_refs`). **Strictly NO `criteria_refs` or `priority_plan_refs` on `goal`.**
    - **`requirement` (Timeless Whole Feature Contract):** Owns `criteria_refs`. MUST declare at least 3 criteria satisfying the Three-Fold Proof (Static Floor Invariant, Operational Dynamic Proof, Negative Adversarial Boundary). **Requirements exist outside of time and MUST NOT declare `priority_plan_ref` or `priority_plan_refs`.**
    - **`test_case` (Unified Verification Group):** Linked 1:1 to a `requirement`. Wraps and verifies all criteria for that requirement in a single execution (`zqk test run <tst_id>`).
  - **Chronological Anchors & Time-Bounded Execution:**
    - **`milestone` (Chronological Boundary Anchor):** Introduces the time constraint across delivered reality. Acts as a time-bounded subgoal or external release event that Priority Plans aim toward.
    - **`priority_plan` (Time-Bounded Execution Cycle):** Groups exactly **1 cycle of work** (sprint/kanban batch, 2–5 BLIs) aiming toward a `milestone`. Acts as the stackable column on the Gantt chart. Scope-locked upon entering `in_progress` to guarantee completion.
    - **`backlog_item` (Atomic Unit of Effort):** Satisfies 1 to 3 specific criteria. Never build an entire feature in one monolithic BLI. Minimum 2–5 BLIs per Priority Plan.
    - **`epic` (Thematic Multi-Plan Container & Semantic Shape):** Gives human-legible semantic shape to the overall initiative (e.g. 'Interior Home Remodel' vs confusing abstract 'Phase 2/4'), grouping multiple 1-cycle Priority Plans under a meaningful domain umbrella. Responds to shockwaves (first linked plan `in_progress` flips epic to `in_progress`; all linked plans `complete` flips epic to `completed`). Permissive by default (can add plans while in_progress as new work/phases are discovered, unless explicitly `execution_locked`).
  - **The Non-Functional Behavior Plane (`technical_debt`):**
    - **`technical_debt` (Tactical Entry Point into Non-Functional Reality):** Represents tactical technical work (code smells, test flakes, performance bottlenecks, storage leaks, negative boundary omissions) often supporting one or more goals, but not directly called out as a feature requirement.
    - **The Rollup & Categorization Principle:** Technical debt tells the story of the non-functional plane. The rollup and clustering of multiple tech debt items MUST be used to extract the underlying anti-pattern story and codify lasting architectural or coding best practices (minted as `policy` objects or verified invariants), preventing recurrence in future initiatives.
- **Anti-Hijacking Rule (No Scope-Stacking):** If human intent introduces a new UI, macro factor, or capability, NEVER append or mutate an in-flight or completed BLI. Mint a new BLI under the cycle or mint a new Priority Plan under the parent Epic.
- Reference: `docs/guides/KERNEL_OBJECT_ADMINISTRATION.md`

## Operator Handoff Protocol (Post-Milestone Completion Briefing)
- When completing a Goal, Milestone, or lead Priority Plan, do NOT simply output test pass rates and stop.
- Every major completion transition MUST provide an **Operator Handoff Briefing**:
  1. **Operational Surface:** Credentials required, environment variables, secret management (`.env.example`).
  2. **Execution Topology:** On-demand CLI vs. Scheduled Cron vs. Always-on Daemon with URLs and ports.
  3. **Documentation Manifest:** Verified links to architecture docs, runbooks, and diagrams.
  4. **Next Strategic Action:** Immediate next priority plan discovered via `zqk workflow whats-next`.

## Chat Issue Intake & TPM Anti-Distraction Protocol (POL-AGENT-TPM-INTAKE-FIREWALL-001)
- **The Chat-to-Kernel Firewall:** When an operator or chat message reports a bug, test failure, stack trace, or unexpected behavior, the TPM MUST NOT drop the active plan or begin writing code and running tests directly.
- **Mandatory 3-Step Triage Sequence:**
  1. **Objectify Immediately (Zero Code Written):** Mint a persistent Knowledge Kernel object to capture the report:
     - Test failures, flakes, performance, or hygiene issues: `zqk new technical_debt --title "<symptom>"`
     - Missing functionality, scope gaps, or requirement defects: `zqk new backlog_item --title "<symptom>"`
  2. **Triage & Route:**
     - **P0 (Blocks Current Active Priority Plan):** Attach to active plan as a blocker; delegate the fix to an isolated worker subagent or swarm (`zqk agent orchestrate` / IC craftsman) with the error log. The TPM monitors the gate but NEVER writes code or runs tests.
     - **P1/P2 (Non-blocking):** File into the backlog or upcoming cycle.
  3. **Resume Program Trajectory:** Immediately output the minted object receipt (TD-xxx / BLI-xxx), query `./bin/zqk workflow whats-next`, and continue tracking the active priority plan without yielding or getting distracted.

## Code Search & Token Conservation (`zqk grep`)
- **Prefer `zqk grep` (alias `zgrep`) over raw shell `grep` or `find`:** `zqk grep` provides sub-15ms trigram indexing, Go AST structural queries (`--ast --kind struct|func`, `--ast --recv <Type>`), and strict token budgeting (`--max-tokens 2000 -f json`). Using external grep dumps unbudgeted files into LLM contexts and increases token consumption.

## Universal AGENTS.md
- This file is the headless-safe directive surface (Vector B). IDE rule forests are optional packs.

