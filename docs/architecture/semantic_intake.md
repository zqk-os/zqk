# Architecture Review: Semantic Ingestion Pipeline (Intent Capture)

## 1. Executive Summary
The CLI currently operates as a rigid database entry tool, expecting humans (and agents) to synthesize perfectly compliant schema payloads (`title`, `description`, `refs`). This rigidity leads to boilerplate duplication ("ocean of bullshit") or outright rejection. 

The **Semantic Ingestion Pipeline** shifts the CLI's role from *schema validation* to *intent capture*. Running synchronously within the CLI session, users submit raw, semi-structured key-value pairs or conversational context. The active CLI session intercepts these signals, directly invokes the Semantic Engine (LLM) to synthesize compliant target objects, and instantiates them.

Crucially, this pipeline natively supports **bulk operations**. A single contextual brain-dump can be semantically parsed into multiple discrete objects. To govern this dynamic ingestion, the architecture introduces the **Policy Enforcement Neuron**—a DSL-driven shockwave that dynamically validates semantic groups as they ripple through the graph.

---

## 2. Architecture & Data Flow (Synchronous Model)

1. **Intake Interface (CLI):** 
   - A new command (`zqk intake`) captures raw context. The CLI accepts single intents or massive contextual text dumps (e.g., piping a Markdown file).
2. **Synchronous Synthesis & Bulk Parsing:**
   - The CLI initiates a direct call to the ambient Semantic Engine.
   - The LLM parses the raw input into an *array* of identified objects (e.g., extracting three `requirement` objects and one `workstream` from a single prompt).
3. **Clarification Loop (In-Session):**
   - If the intent is too vague, the semantic engine immediately kicks back a clarification prompt to the terminal.
4. **Target Instantiation (The Bulk Commit):**
   - The CLI executes the underlying `object create` logic for *each* parsed object in the array, staging them as `draft` or `planned`.

---

## 3. The Policy Enforcement Neuron (Shockwave Validation)

Static schema validation (e.g., checking if a string is present) is insufficient for dynamic semantic ingestion. To govern the bulk creation of objects, we use a **Validation Overlay** driven by `policy` objects.

1. **The Policy DSL as a Neuron:**
   Each `policy` object contains a declarative Validation DSL defining the semantic constraints for a specific group of objects. When a bulk intake occurs, these policies act as enforcement neurons.
2. **The Shockwave Effect:**
   When an object is synthesized and instantiated, it triggers a "shockwave." The event strikes the relevant policy neuron, which evaluates the Validation DSL. 
3. **Graph Rippling (Downstream Triggers):**
   If the policy dictates further checks (e.g., "A requirement must trace to an active strategic plan"), the shockwave ripples across the graph relationships. It automatically triggers downstream events, notifications, and cascading validations.
4. **Guards and Leaf Nodes:**
   The shockwave continues rippling through the graph until it safely dissipates at a leaf node (validation passes completely) or slams into a strict Guard (e.g., triggering an `ElectrocutionError` and halting the intake process because a mandatory strategic boundary was violated).

---

## 4. Concurrency Review (Multi-Instance Support)

- **Session Isolation:** The architecture supports dozens of concurrent `zqk intake` sessions simultaneously. 
- **No Global Locks:** Because the inference and intent-capture happen entirely in-memory within the isolated CLI process, there are zero global locks during the "thinking" phase.
- **Write-Time Concurrency:** When the synthesized objects are finally ready for instantiation, it leverages atomic file locks. The Shockwave Validation operates entirely on read-state graph traversal until the final atomic write.

---

## 5. Security & Entitlements Review

- **Prompt Injection & Execution Bounds:** The LLM prompt must wrap raw input in strict delimiter boundaries. The CLI enforces bounded output parsing to ensure the LLM only returns the requested schema parameters.
- **Partial Failure Rollbacks:** Because a single shockwave could trigger an electrocution halfway through a bulk bulk parse, the ingestion pipeline must utilize a transactional boundary (or staged drafting). If one object hits a Guard, the entire bulk payload is rolled back to prevent torn graph states.
- **Session Identity Preservation:** Target objects and their cascading shockwaves are executed using the exact RBAC entitlements of the active session. If an unauthorized community user tries to synthesize a restricted structure, the policy shockwave will immediately hit a permission guard and electrocute the session.

---

## 6. Governance Review

- **Traceability Chain:** Every object created by the intake process must inject metadata linking it back to the original raw intent (e.g., an `intake_context` field storing the human's exact words). For bulk operations, all generated objects will share the same `intake_context` hash, ensuring auditors can trace the exact shockwave origin.
- **The "BZZZT" Prevention:** By initially generating objects in a `planned` state, the CLI ensures that missing mandatory fields do not immediately trigger a terminal shockwave, allowing for human review before activating the objects.

---

## 7. Hater Critical Review (The Antagonist's Take: Part 4)

*“Ah, the 'Shockwave Validation Neuron.' It sounds like a sci-fi weapon, which is fitting because you are about to blow your own legs off.”*

1. **The Infinite Ripple (Cascading Failures):**
   You just introduced an event-driven graph traversal that triggers downstream validations automatically. Have you ever written a recursive function without a base case? The moment two policies have overlapping, slightly conflicting DSL definitions, your 'shockwave' is going to bounce back and forth between them in an infinite loop until the CLI runs out of memory and violently crashes. 

2. **The UX of a Nuclear Blast:** 
   A user pastes a meeting summary. The LLM hallucinates 5 objects. The ingestion starts, and Object 3 triggers a policy shockwave that ripples through 14 connected graph nodes, eventually hitting a Guard 3 levels deep because of an obscure strategic plan constraint. The entire transaction rolls back. 
   What is the error message the user sees? Are you going to print a 50-line graph traversal stack trace? "Failed to create workstream because Requirement B's shockwave hit Policy 12's guard on Strategic Plan Alpha." The user will have absolutely zero idea how to fix their original prompt.

**The Antagonist's Verdict:** Moving policy enforcement to a dynamic DSL is the correct architectural choice, but the 'shockwave' is a loose cannon. You MUST implement a strict 'Max Depth' limit on the ripple, and you need a dedicated 'Shockwave Interpreter' that can translate a 3rd-degree graph rejection back into a human-readable prompt that the LLM can use to self-correct in the background before bothering the human.
