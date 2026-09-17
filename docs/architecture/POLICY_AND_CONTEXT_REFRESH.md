# Policy vs Context Refresh Schedule

**Last Verified:** 2026-08-31


**Purpose:** Clarify the difference between the **policy** object kind and the **context_refresh_schedule** object kind, when to use each, and how they are used by the system. This avoids conflating "policy" (project standards and enforcement) with operational scheduler configuration.

## Summary

| Aspect | `policy` | `context_refresh_schedule` |
|--------|----------|----------------------------|
| **What it is** | Project standards, guidelines, enforcement index | Scheduler config: when to refresh context (e.g. `.cursor`, AGENT_CONTEXT_REFRESH) |
| **Count** | Many (dozens) | One or few per project/profile |
| **ID prefix** | POL-* (e.g. POL-CODE-007) | CON-* |
| **Storage** | `.zqk/process/policies/` | `.zqk/process/context_refresh_schedules/` |
| **Schema** | Rich: body, category, policy_type, enforcement, applicability | Narrow: target, cadence, last_refresh, next_refresh |
| **Visibility** | Public (docs, agents) | Internal (scheduler only) |
| **Used by** | Enforcement index, agent guidelines, init templates | Scheduler context-refresh handler |

## When to use which

- **Use `policy`** when defining or indexing project standards, expectations, requirements, or guidelines (e.g. POL-CODE-007, POL-AGENT-001). See [PROJECT_POLICY_SYSTEM.md](./PROJECT_POLICY_SYSTEM.md).
- **Use `context_refresh_schedule`** when configuring *when* the system should refresh context artifacts (e.g. one schedule for `project:zqk` with cadence P1D). Do not use it for policy content.

## Naming rationale

The kind was previously named **context_refresh_policy**, which suggested a subtype of "policy" and caused confusion with the main `policy` kind. It has been **renamed to context_refresh_schedule** because:

- It defines a **schedule** (cadence, last_refresh, next_refresh), not standards or expectations.
- "Policy" in this project means content-backed standards and enforcement; this object is operational configuration only.

## Path forward

1. **Policies:** Continue using `policy` for all project standards and guidelines. Many instances per project is expected.
2. **Context refresh:** Use `context_refresh_schedule` for scheduler-driven context refresh. One or a few instances per project/profile; create/update via CLI only (see process-data-cli-only).
3. **Implement refresh logic:** The scheduler handler that lists and updates `context_refresh_schedule` objects has a TODO to implement the actual context refresh behavior (e.g. writing AGENT_CONTEXT_REFRESH). See `pkg/scheduler/handlers_context_refresh.go` and `docs/refactoring/TODO_TRIAGE.md`.

## Migration note (rename from context_refresh_policy)

After the rename to `context_refresh_schedule`, the previous instance (CON-001) may still exist under `.zqk/process/context_refresh_policies/`. That directory is no longer used by the system (the scheduler and CLI use `context_refresh_schedules/`). You can remove the old directory or the single YAML file there as a one-time cleanup if desired. New schedule instances live in `.zqk/process/context_refresh_schedules/`.

## References

- [PROJECT_POLICY_SYSTEM.md](./PROJECT_POLICY_SYSTEM.md) — policy object and categories
- [AGENT_GUIDELINES.md](../enforcement/AGENT_GUIDELINES.md) — agent use of policies
- Object specs: `.zqk/specs/objects/policy.yaml`, `context_refresh_schedule.yaml`
- Handler: `pkg/scheduler/handlers_context_refresh.go`
