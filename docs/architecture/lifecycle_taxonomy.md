# ZQK Object Lifecycle Taxonomy & State Transitions

To prevent system sprawl and ambiguity, ZQK strictly categorizes object lifecycles based on their archetype. **Roles** (`status.role`) are the class plane; kind-local status tokens are the object plane. Design exam (N! prune, catalyst / pre / post / shockwave / class-vs-object): **[LIFECYCLE_STATE_MACHINE_RUBRIC.md](../../architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md)**. Do not copy a priority_plan check valve onto kinds where `active` means something else.

Furthermore, state transitions must be **System-Driven** where a qualifying exam or shockwave exists; manual promote is first-class only on dual `manual+auto` edges that share the same preconditions.

## 1. Lifecycle Archetypes


### Execution Objects (`backlog_item`, `technical_debt`)
Tracks the progression of *work*.
*   `exploring`: The item is a raw idea without clear criteria.
*   `validated`: The idea is sound, criteria are defined. Ready for the backlog.
*   `planned`: Attached to a priority plan and milestone.
*   `in_progress`: Active `agent_task`s exist.
*   `complete`: All child tasks are complete and all criteria are verified.

### Structural Objects (`goal`, `requirement`, `rule`)
Tracks the progression of *agreement*.
*   `proposed`: Draft standard lacking consensus.
*   `approved`: Signed off by IA/Leadership. Actively enforced.
*   `deprecated`: Phased out in favor of new ontology.
*   `archived`: Fully retired.

### Ephemeral/Telemetry Objects (`base_metric`, `agent_feed_event`)
Tracks the progression of *data*. Does not undergo human review.
*   `active`: The metric/event is currently relevant.
*   `archived`: Rolled up into an aggregation metric or expired.

---

## 2. System-Driven State Transitions (The Auto-Transition Mandate)

**Agents must never manually toggle the status of higher-order objects like `backlog_item` or `goal`.** 

The ZQK Knowledge Kernel handles state transitions automatically via event listeners and graph dependency checks:

1. **Task Submission:** An agent completes its `agent_task` and submits it for validation (marking the *task* as ready for review).
2. **System Verification:** The ZQK Scheduler/Kernel automatically runs the `criteria` objects linked to the task.
3. **Graph Cascade:** If the criteria pass, the system marks the `agent_task` as `complete`.
4. **Parent Roll-up:** The system checks the parent `backlog_item`. If all child `agent_task`s are `complete` and all `backlog_item_refs` criteria pass, the system automatically transitions the `backlog_item` to `complete`.

This eliminates agent hallucination regarding project state and guarantees deterministic, verifiable progress.
