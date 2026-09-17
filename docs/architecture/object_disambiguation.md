# Object Disambiguation: Backlog Item vs Agent Task

To ensure agents process context correctly and eliminate ontology confusion, we must strictly disambiguate the roles of a `backlog_item` and an `agent_task`.

## `backlog_item` (The "What" and "Why")
*   **Definition:** A `backlog_item` represents **human-readable value delivery**. It is the feature, the bug fix, or the technical debt resolution. 
*   **Ownership:** It is owned by the project. 
*   **Context Level:** It cares about the *outcome*. It contains `acceptance_criteria` (what must be true when it's done), `goal_refs` (why we are doing it), and `milestone_refs` (when it needs to be done).
*   **Analogy:** The blueprint for a house.

## `agent_task` (The "Who" and "How")
*   **Definition:** An `agent_task` is an **execution envelope**. It is a purely mechanical, transient routing object used to assign a specific piece of work to a specific AI persona.
*   **Ownership:** It is owned by the active Swarm/Pipeline.
*   **Context Level:** It cares about the *execution*. It contains `assignee_persona_ref` (who is doing it), `inputs` (what files/context they need), and `outputs` (what artifacts they must produce).
*   **Analogy:** The daily work order handed to the plumber.

## The Meta Layer Relationship
A single `backlog_item` (e.g., "Implement User Authentication") is almost never completed by a single agent doing a single thing. It requires a pipeline. 

Therefore, a `backlog_item` is the **parent**, and it spawns multiple **child** `agent_task` objects:

1.  **Task 1 (TPM Persona):** Read the `backlog_item` and break it into an execution plan.
2.  **Task 2 (Coder Persona):** Read the plan and write the backend Go code.
3.  **Task 3 (QA Persona):** Read the code diff and run the verification checks against the `backlog_item`'s acceptance criteria.

**Prompting Implication:**
When an agent receives an `agent_task`, the MCP Server uses the task to filter context. The Coder receives only the inputs needed for code, while the QA Auditor receives the code and the parent Backlog Item's acceptance criteria.
