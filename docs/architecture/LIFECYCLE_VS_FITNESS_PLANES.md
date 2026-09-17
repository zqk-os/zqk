# Lifecycle vs fitness planes

**Last Verified:** 2026-08-31


**Requirement:** `REQ-KERNEL-LIFECYCLE-FITNESS-001`  
**Glossary:** `GLS-LIFECYCLE-FITNESS-PLANES-001` (`lifecycle_vs_fitness_planes`)  
**Concern layers:** `GLS-1785919198508273000-81429ca0` (`validation_concern_layers`)  
**Code:** `pkg/fitness`  
**Plan:** `PRI-KERNEL-LF-DIAL-001` (mid-flight under kernel health)

## Split

| Plane | Answers | Durable knob |
|-------|---------|--------------|
| **Lifecycle** | Where is this object in *its* process? | `status` (one machine per kind) |
| **Fitness** | Relative to *this use*, is it safe / complete / employable? | Typed findings (`issue_class` + concern layer + surface) |

Do **not** invent parallel lifecycle enums per context. Multiply **surfaces**, not status machines.

## Issue classes

| Class | Meaning | May demote `status→error`? |
|-------|---------|------------------------------|
| `process_failure` | Illegal lifecycle token or status/transition precondition | **Yes** (if kind family allows + lifecycle has `error`) |
| `data_completeness` | Required/missing/empty fields | No |
| `employment_fitness` | RBAC / persona / credential / agent-usable | No |
| `referential_integrity` | Orphan / dangling / broken refs | No (integrity path) |
| `policy_gate` | Policy forbid / not allowed | No |

Classifier: `fitness.ClassifyIssue`. Autofix gate: `fitness.ShouldDemoteToError`.

## Kind families (autofix demote policy)

| Family | Example kinds | Autofix demote→error |
|--------|---------------|----------------------|
| `work_attempt` | `agent_task`, `scheduler_job`, `convergence_session` | Allowed for `process_failure` |
| `execution_work` | `backlog_item`, `technical_debt`, `question` | Allowed for `process_failure` |
| `identity_governance` | `account`, `priority_plan`, `roadmap`, `organization` | **Never** (use suspended/blocked/grooming) |
| `strategic_content` | `mission`, `goal`, `requirement`, `criteria`, `persona` | Allowed only for `process_failure` if lifecycle has `error` |
| `telemetry` | `*_metric`, `audit_event` | Allowed for `process_failure` |
| `other` | remainder | Allowed for `process_failure` if lifecycle has `error` |

Presence of an `error` status in lifecycle YAML is **not** permission to encode completeness as error.

## Surfaces (audience filter)

| Surface | Shows (default) | Suppresses |
|---------|-----------------|------------|
| `auth` | employment, process_failure, policy | completeness noise |
| `agent_dispatch` | employment, process_failure, policy, referential | admin completeness |
| `system_check_l0` | process_failure, data_completeness | employment |
| `system_check_l1` | process + completeness + referential + policy | — |
| `admin_form` | data_completeness, referential | employment |
| `whats_next` | process_failure, employment, policy | field-level completeness |
| `list_default` | process_failure only | everything else |

API: `fitness.VisibleOnSurface` / `FilterClasses`.

## Example

Account `status=active` and not RBAC-ready: **valid lifecycle**, failing **employment** finding. Auth surface shouts; default list does not pretend `status=error`.

## Related

- `docs/architecture/LIFECYCLE_STATUS_ROLES.md` — `halted` vs terminals (role plane; complementary)
- `docs/architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md` — N! prune, class vs object, policy as membrane start
- Autofix: `cmd/zqk/system/check_impl_autofix.go` → `pkg/fitness.ShouldDemoteToError`
