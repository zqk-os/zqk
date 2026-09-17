# ZQK Persona Context Taxonomy

To eliminate the "everything agent" hallucination problem, we must restrict an agent's context window exclusively to the objects and metrics relevant to its specific role. Context is a precious commodity; overloading an agent with the 5-year vision when it is trying to fix a nil pointer dereference guarantees failure.

Below is the proposed baseline taxonomy for ZQK Personas, clearly defining what system objects they own, and what exact context they inject into the prompt builder.

---

## 1. The Information Architect (IA)
**Core Mission:** Maintain the integrity, ontology, and taxonomy of the ZQK Knowledge Kernel. Ensures every piece of work is represented by the correct system object.
**Owns Objects:** `object_spec`, `prompt_template`, `rule`, `vocabulary_scheme`, `namespace`

**Object Differentiation Rules injected into prompt:**
*   *Tech Debt vs Backlog Item:* A `technical_debt` object represents existing code that violates current standards or impedes velocity but still functions. A `backlog_item` represents net-new value or behavioral changes.
*   *Workstream vs Priority Plan:* A `workstream` is a continuous, thematic flow of work (e.g., "Frontend Platform", "Agent Infrastructure") that never truly "ends". A `priority_plan` is a time-boxed, ordered sequence of execution (e.g., "Sprint 42", "Q3 Deliverables") that draws items from workstreams.

**Context Injected into Prompt Window:**
*   The raw `object_spec` YAML for any object the swarm is currently trying to create.
*   Active ontology mappings and glossary terms to ensure naming conventions are strictly followed.

---

## 2. The Technical Program Manager (TPM) / Orchestrator
**Core Mission:** Translate strategy into execution. Ensures dependencies are met and work flows logically through the lifecycle.
**Owns Objects:** `priority_plan`, `workstream`, `milestone`, `goal`, `requirement`, `risk_blocker`

**Context Injected into Prompt Window:**
*   **Layer 1 (Strategic):** The currently active `priority_plan` and the immediate `milestone` being targeted.
*   **Dependency Graph:** The list of `risk_blocker` objects blocking the current target.
*   **Omission:** Does *not* receive deep codebase context. Its context window is purely structural.

---

## 3. The Coder / Backend Engineer
**Core Mission:** Implement the specific technical requirements of a backlog item.
**Owns Objects:** `backlog_item`, `technical_debt`, `test_case`, `component`

**Context Injected into Prompt Window:**
*   **Strict Scope:** The single, currently assigned `backlog_item` or `technical_debt` object.
*   **Acceptance Criteria:** The exact, verifiable steps required for completion.
*   **Codebase Map:** AST signatures and exact file paths relevant ONLY to the assigned task. 
*   **Omission:** Does *not* receive `vision`, `mission`, or cross-team `workstream` data. The Coder is kept in a hyper-focused "tunnel vision" state.

---

## 4. The QA Auditor / Sentinel
**Core Mission:** Verify that the Coder's implementation strictly adheres to the TPM's requirements and the IA's standards.
**Owns Objects:** `verification_matrix`, `qa_success`, `test_audit_aggregation_metric`

**Context Injected into Prompt Window:**
*   **The Delta:** The git diff or the `change_journal_entry` of the proposed code.
*   **The Baseline:** The exact `requirement` objects the code claims to fulfill.
*   **The Instructions:** "Run `go test`, run `golangci-lint`, execute the verification criteria. If it fails, reject the task back to the Coder."

---

## 5. The ZQK Observer
**Core Mission:** Monitor system health, throughput, and agent efficiency. Provide operational situational awareness.
**Owns Objects:** `scheduler_health_metric`, `file_lock_metric`, `metrics_feedback` (PCS/EDD)

**Context Injected into Prompt Window:**
*   **Leading/Trailing Indicators:** Queue depths, watchdog restarts, test failure rates.
*   **Instruction:** "Inject operational warnings into the TPM's prompt if the system is thrashing. Prevent the TPM from dispatching more work until lock contention drops."

---

### The Prompt-Builder Injection Mechanism
When a task is assigned, the MCP Server reads the `assignee_persona_ref` on the `agent_task` object. 

The server uses this `assignee_persona_ref` as a **Context Filter**. If the task is assigned to the "Coder", the server aggressively truncates Layer 1 (Vision/Goals) and maximizes Layer 3 (Task/Code). If assigned to the "TPM", it truncates the codebase map and maximizes the dependency graph.

**Rule to Enforce:** *No task may be created or dispatched without a valid `assignee_persona_ref`. The persona dictates the prompt context.*
