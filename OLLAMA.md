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

## Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)
- **Summary-as-Terminal Failure Mode Prohibition:** In the autonomous CAP loop, merging a PR, promoting binaries, or rendering an artifact summary is a MILESTONE TRANSITION, NOT a stopping condition.
- **NEVER yield control or go idle at summary milestones.** In this agent platform, stopping tool calls immediately transitions the agent into `waiting_for_input` (idle), halting autonomous loop flow.
- **Strict Post-Merge Self-Continuation Protocol:**
  1. Merge PR & promote stable binary (`./scripts/install.sh && ./bin/zqk mcp ensure`).
  2. Query `./bin/zqk workflow whats-next`.
  3. Immediately create/checkout the next integration branch (`git checkout -b integration/<pri-id> origin/main`).
  4. Claim or shape the first BLI (`zqk agent claim ...` or kernel object creation).
  5. Continue the execution chain without yielding control to an idle wait state.

## Code Search & Token Conservation (`zqk grep`)
- **Prefer `zqk grep` (alias `zgrep`) over raw shell `grep` or `find`:** `zqk grep` provides sub-15ms trigram indexing, Go AST structural queries (`--ast --kind struct|func`, `--ast --recv <Type>`), and strict token budgeting (`--max-tokens 2000 -f json`). Using external grep dumps unbudgeted files into LLM contexts and increases token consumption.

## Vendor: Ollama (Air-Gapped Local LLM)
- Operates 100% offline and sovereign without external network access.
- Local model coordination via Ollama API at 127.0.0.1:11434 (Qwen 2.5 Coder, Llama 3, DeepSeek Coder).
- Execute workflows directly via zqk workflow whats-next and zqk agent orchestrate.
- All session directives, tests, and criteria remain strictly grounded in the Knowledge Kernel.
