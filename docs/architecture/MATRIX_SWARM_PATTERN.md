# Architecture Decision Record: Matrix Swarm Pattern

**Last Verified:** 2026-08-31


## Context and Problem
As ZQK scales out multi-agent execution, we face the "Blackboard Problem." A strategic pod breaks down a large `priority_plan` into many smaller criteria. The swarm needs a shared way to log state, and the overall system needs a way to view progress across many active pods. Interrupting agents to ask for status reduces throughput, and having multiple agents simultaneously update a single file causes severe file lock/merge conflicts (write contention).

## Solution: The Matrix Swarm Pattern
We synthesize the existing `verification_matrix` (specifically with `matrix_role: agent_coordination`) and the `agent_chat_channel.jsonl` bus to create a massively parallel, lock-free checklist system.

1. **The Blueprint (Grooming Stage):** 
   When the Strategic Pod (`cap_stage_grooming`) provisions a complex `priority_plan`, it also creates a `verification_matrix` object and its backing CSV. This represents the explicit breakdown of sub-tasks.
   
2. **The Ledger (Execution Stage):** 
   The execution Pod is dispatched with the `verification_matrix` ID in its ambient context. The matrix serves as the unquestionable source of truth for completion.

3. **Lock-Free Updates (The Matrix Steward):**
   Worker personas (e.g., Engineer, QA) **never edit the CSV directly**. Instead, when they complete a criteria, they emit a lightweight JSON event to `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` (e.g. `{"event": "task_complete", "matrix_row": 4}`).
   
   The Pod's TPM persona acts as the **Matrix Steward**, tailing this event bus to serialize updates deterministically into the matrix CSV. This eliminates write contention.

4. **Continuous Swarm Aggregation:**
   A background system scheduler job continually aggregates the states of all active `verification_matrix` objects, rolling up progress dynamically to organizational dashboards without interrupting agent execution flow.

## Consequences
- **Positive:** Agents run at max velocity without git/file locks. Progress is strictly objective and verifiable. Seamless dashboarding.
- **Negative:** Increased reliance on the `agent_chat_channel.jsonl` bus remaining highly available and properly formatted.
