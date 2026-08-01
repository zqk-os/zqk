# Snapshot References and Template-Driven Growth

**Status:** Design  
**Purpose:** Capture a first-class pattern for placeholder references (SNAPSHOT), lifecycle-gated transitions, and opinionated templates so ideas can start simple and grow without becoming disconnected.

---

## 1. Generic SNAPSHOT reference

**Idea:** A reference field can hold either a **concrete** reference (e.g. `GOAL-001`) or a **snapshot/placeholder** reference (e.g. a well-known token or synthetic ID that means “to be filled in”).

- **On create or initial ref link:** Validation accepts any referenceable object *or* a designated snapshot placeholder. So you can create an object (e.g. requirement, backlog item) and set `goal_refs: [SNAPSHOT]` or `goal_refs: [REQ-PLACEHOLDER-001]` and the system allows the save/update.
- **Lifecycle validation:** Normal lifecycle rules still apply (status transitions, required fields, etc.). So as long as lifecycle validation passes, the object can be saved and updated even with snapshot refs.
- **Transition to final state:** For fields that are **required** and not optional, the system does **not** allow a transition to a designated final state (e.g. `complete`, `approved`) if the field still holds a snapshot/placeholder reference. It must be updated to a non-snapshot (concrete) reference first. Optional refs can remain snapshot until replaced or cleared, without blocking terminal state.

**Effect:** You can author bundles or objects early with “TBD” refs, iterate, and only lock down concrete refs when moving to a terminal state. The system stays consistent (required refs are real before completion) while allowing a fluid, draft-friendly workflow.

---

## 2. Templates with placeholders and light configuration

**Idea:** Provide templates (per object or as part of a bundle) that use **placeholder text** with hints and tokens for:

- **Computed** values (e.g. generated ID, derived title)
- **Derived** values (e.g. from another field or from context)
- **Default** values (e.g. status, category)

Placeholders act like **synonyms**: they can have light configuration for:

- **What** to overwrite with (e.g. a concrete ID, a value from a registry, or a prompt-derived value)
- **When** it applies (e.g. on create, on first edit, when transitioning to a given status, or when a “resolve placeholders” step runs)

This supports a more **opinionated** way to structure prompt instructions and integration: the bundle or structure captures the right **scope** and **integration level** in one place. Ideas can start simple (placeholders, snapshot refs) and naturally embrace growth and change, with the system supporting that in a first-class way.

---

## 3. First-class growth

**Idea:** The system should treat “start simple, grow over time” as a first-class path:

- **References:** Snapshot/placeholder refs are valid until a terminal state requires concrete refs; then the workflow forces (or guides) replacement.
- **Templates:** Placeholder text and tokens in templates give hints for what to fill in and when; tooling (CLI, agents, prompts) can use the same structure so scope and integration level stay aligned.
- **Bundles:** A bundle (or similar structure) can define both the object graph and the placeholder/template policy, so “appropriate scope and integration level” live together and stay less disconnected as things evolve.

---

## 4. Possible implementation directions

- **Semantic type or ref kind:** Define a semantic type (e.g. `snapshot_reference`) or a reserved ID pattern (e.g. `SNAPSHOT`, `PLACEHOLDER-*`) that validation treats as valid for ref fields on create/update but that lifecycle rules treat as “not resolved” for required refs when transitioning to a final state.
- **Lifecycle preconditions:** Extend lifecycle definitions with a precondition like “required ref fields must not be snapshot/placeholder” for transitions into terminal statuses (e.g. `complete`, `approved`).
- **Template format:** Extend bundle or object template format with placeholder syntax (e.g. `{{id}}`, `{{goal_refs}}`) and optional config (overwrite-with, when-applies) and document how prompt instructions and tooling should use it.
- **Resolve step:** A dedicated “resolve placeholders” or “replace snapshot refs” step (manual or automated) that replaces snapshot refs with concrete refs and fills template placeholders from context or user input.

---

## 5. Bundle processor: new addition vs legacy-linked work

**Idea:** The bundle processor should understand whether a bundle (or items within it) represent:

- **New addition** – Net-new objects and work (create goals, requirements, criteria, backlog items with no required link to existing artifacts).
- **Part of an earlier body of work** – New objects that extend or relate to **existing/historical artifacts**. The processor should **allow or create links** to those historical artifacts, because they often provide valuable context (business requirements, constraints that are likely to evolve over time) and can be **formal enough** to support an **abstract semantic description** of how the new things relate to and build upon the legacy.

**Implications:**

- **Legacy refs in the bundle:** Bundle templates can declare refs to existing objects (e.g. `goal_refs: [GOAL-001]`, `related_patterns: [POL-CODE-007]`, or a dedicated `legacy_refs` / `extends_ref` field). The processor, when applying, creates the new objects and sets these refs so the new work is explicitly tied to the prior art.
- **Semantic relationship:** Support an abstract semantic description of the relationship – e.g. “extends”, “implements”, “refines”, “replaces”, “satisfies” – so the system can represent *how* new work builds on legacy (not just “links to” but “refines requirement REQ-X” or “implements aspect of GOAL-Y”). That can live in a relationship type field, a dedicated link object, or in the object’s description with a standard vocabulary.
- **Context preservation:** Linking to historical artifacts preserves business requirements and constraints that evolve over time; the bundle processor should make it easy to “create new + link to legacy” in one pass so scope and lineage stay clear.

**Effect:** Bundles can express both greenfield work and work that builds on legacy; the processor creates the new objects and the links to historical artifacts in a first-class way, with optional semantic relationship types so “new vs legacy-linked” is explicit and machine-usable.

---

## 6. References

- **Semantic types:** `docs/architecture/architecture/semantic-types-ontology-v1.0.md` (reference type)
- **Lifecycles:** `docs/architecture/_internal/lifecycles/` (statuses, transitions, preconditions)
- **Bundles:** `pkg/scenario/bundle.go`, `test-scenarios/persistence-bundle/persistence-bundle.yaml`
- **Compile-time substitution (related):** REQ-990, `docs/architecture/SCHEDULER_MAINTENANCE_SEMANTIC_TYPE_AND_POLICY.md`
- **Bundle apply:** `pkg/scenario/apply.go`, `pkg/scenario/apply_traceability.go` (create order, ref resolution)
