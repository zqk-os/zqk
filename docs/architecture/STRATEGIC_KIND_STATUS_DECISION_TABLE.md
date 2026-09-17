# Strategic-Kind Status Vocabulary & Promotion Decision Table

**Last Verified:** 2026-08-31


**Backlog Item:** `BLI-1786693876531050000-3bc9507f`  
**Requirement:** `REQ-KERNEL-LIFECYCLE-FITNESS-001`  
**Priority Plan:** `PRI-1786687873940250000-a6c3a985`  
**Criteria:** `CRIT-1786695439226615000-7b191ae0`, `CRIT-1786695439226622000-a7e32b08`, `CRIT-1786695439226627000-794c17c7`, `CRIT-1786695439226634000-43d42912`, `CRIT-1786695439226640000-53bfdb37`

---

## 1. Executive Summary & Semantic Principles

To prevent lifecycle theater, eliminate confusing vocabulary collisions, and ensure process gates have machine-enforceable teeth:
1. **Goals & Requirements (`goal`, `requirement`):** Live program targets are commitments and are represented with `active` (not "planned work"). Preliminary proposals use `proposed`. Terminal completion is `complete` or `archived`.
2. **Risk Blockers (`risk_blocker`):** Open/mitigating hazards use `active` or `mitigating` (not "approved document"). Resolved hazards transition to `resolved` or `accepted`.
3. **Criteria (`criteria`):** Acceptance gates exist in verification states: `awaiting_verification`, `in_progress`, `validated`, or `rejected` (not chore-like `not_started`).
4. **Agent Tasks (`agent_task`):** Execution units follow `proposed` &rarr; `approved` (shovel-ready) &rarr; `in_progress` &rarr; `pending_verification` &rarr; `implemented` (terminal success). Promotion from `proposed` to `approved` strictly requires `title`, `description`, `assignee_persona_ref`, and `pipeline_ref`.
5. **Questions (`question`):** Inquiries follow `open` &rarr; `answered` / `closed`.

---

## 2. Comprehensive Status Decision Table

| Kind | Allowed Statuses | Default / Origin Status | Role | Promotion Preconditions | Terminal States |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`goal`** | `proposed`, `active`, `blocked`, `complete`, `archived`, `error` | `proposed` (preliminary) | `realign` (proposed), `shovel_ready` (active), `halted` (blocked/error), `terminal` (complete/archived) | `proposed` &rarr; `active`: At least one `milestone_ref` or active link | `complete`, `archived` |
| **`requirement`** | `draft`, `active`, `deprecated`, `superseded`, `archived`, `error` | `draft` (preliminary) | `realign` (draft), `shovel_ready` (active), `terminal` (deprecated/superseded/archived) | `draft` &rarr; `active`: Valid goal linkage, non-empty `title` & `description` | `deprecated`, `superseded`, `archived` |
| **`risk_blocker`** | `identified`, `active`, `mitigating`, `accepted`, `resolved`, `archived`, `error` | `identified` (preliminary) | `realign` (identified), `halted` (active), `execution_locked` (mitigating), `terminal` (resolved/accepted/archived) | `identified` &rarr; `active`: Severity and impact defined | `accepted`, `resolved`, `archived` |
| **`criteria`** | `awaiting_verification`, `in_progress`, `validated`, `rejected`, `archived`, `error` | `awaiting_verification` | `shovel_ready` (awaiting_verification), `execution_locked` (in_progress), `terminal` (validated/rejected/archived) | `awaiting_verification` &rarr; `in_progress`: Verification test or method referenced | `validated`, `rejected`, `archived` |
| **`agent_task`** | `proposed`, `approved`, `in_progress`, `pending_verification`, `implemented`, `archived`, `error` | `proposed` (preliminary) | `realign` (proposed), `shovel_ready` (approved), `execution_locked` (in_progress), `halted` (pending_verification/error), `terminal` (implemented/archived) | `proposed` &rarr; `approved`: `title`, `description`, `assignee_persona_ref`, `pipeline_ref` set | `implemented`, `archived` |
| **`question`** | `open`, `in_review`, `answered`, `closed`, `archived` | `open` | `shovel_ready` (open), `execution_locked` (in_review), `terminal` (answered/closed/archived) | `open` &rarr; `answered`: Response resolution documented | `answered`, `closed`, `archived` |

Design exam for kinds not in this table (`priority_plan`, `policy`, `workstream`, …): [LIFECYCLE_STATE_MACHINE_RUBRIC.md](./LIFECYCLE_STATE_MACHINE_RUBRIC.md). `active` is a homonym — shovel-ready on plans, enforced on policies, execution-locked on convergence sessions.

---

## 3. Enforcement & Verification Invariants

1. **Zero Planned Goals/Requirements:** The active CAS membrane contains zero goals or requirements in `status: planned`.
2. **Zero Not-Started Criteria:** All criteria operate in standard verification cycles (`awaiting_verification`, `in_progress`, `validated`).
3. **No Hollow Promotions:** Validators fail-close when required fields are missing during promote operations.
