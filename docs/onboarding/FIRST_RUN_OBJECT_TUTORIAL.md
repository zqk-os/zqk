# First-run object tutorial (template → create → get → update)

**Audience:** New users after `zqk init` (Journey B in [CLI_ALPHA_LAUNCH_PLAN.md](../architecture/CLI_ALPHA_LAUNCH_PLAN.md)).  
**Backlog:** Tracked as part of alpha CLI launch work (see priority plan *CLI alpha launch readiness*).

This path uses the **`question`** kind as a **small** object: few required fields, suitable for learning `object template` / `object create` without editing large YAML. Adjust the kind if your org standardizes another “low-risk” kind.

## Prerequisites

- Shell at the **project root** (directory containing `go.mod` and `.zqk/` after init).
- `zqk` on `PATH` (or invoke `./bin/zqk` from a built tree).

## 1. Generate a template

```bash
zqk object template question --include-optional=false -o /tmp/zqk-first-question.yaml
```

Open the file. Set at least:

- **`title`** — short summary (e.g. `First-run sanity question`).
- **`question_text`** — one line of text.
- Keep the generated base metadata defaults (`created_at`, `updated_at`, `created_by`, `updated_by`) unless your workflow requires different values.
- Leave **`id`** empty if your workflow uses automatic ID assignment from the file; otherwise set an id that matches the kind’s pattern after you read `zqk object create question --help`.

Use **`#` comments** in the file as reminders; strip or keep them per your style (YAML parsers ignore full-line comments).

## 2. Create the object

```bash
zqk object create question --file /tmp/zqk-first-question.yaml
```

On success, note the **object id** from the command output (or run `zqk object list question --format table`).

## 3. Read it back

```bash
zqk object get <QUESTION_ID> --format yaml
```

Try `--format json` for scripting.

## 4. Update a field

Example (adjust status only if allowed by that object’s lifecycle):

```bash
zqk object update <QUESTION_ID> --field title="Updated title after first get"
```

## 5. Clean up (optional)

When you no longer need the example object, delete it per project policy (`zqk object delete <QUESTION_ID>`), or keep it as a fixture in a dev workspace only.

## Troubleshooting

- **`failed to parse YAML` on create:** Re-open the generated file and check indentation/colons near the line number in the error. Fast check: `zqk object create question --dry-run --file /tmp/zqk-first-question.yaml`.
- **Validation errors:** Read the message; fix the cited field. For kind-specific rules, see `docs/architecture/_internal/object_specs/<kind>.yaml` or `zqk system check <kind> <id>` after create.
- **Status/lifecycle rejection on update:** Show allowed status values with `zqk object <kind> fields` and choose a valid transition from the lifecycle.
- **Scheduler daemon not running:** Start it with `zqk scheduler start`, or for commands that support degraded mode re-run with `--allow-degraded` (guardrails: `docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md`).
- **Long-running tests:** Prefer `zqk scheduler go test` for package gates; see project scheduler docs and `PRE_CHANGE_CHECKLIST.md` section 6 for scope.

## See also

- [AI Agent Onboarding Guide](./AI_AGENT_ONBOARDING.md) — full agent/human norms (CLI-only process data, etc.).
- [CLI_ALPHA_LAUNCH_PLAN.md](../architecture/CLI_ALPHA_LAUNCH_PLAN.md) — alpha journeys and backlog seed.
- [scripts/onboarding_roadmap/README.md](../../scripts/onboarding_roadmap/README.md) — full **onboarding curriculum** as objects (milestone-first order); advanced-tutorial pattern.
- [ONBOARDING_EVALUATION_SCENARIO.md](../process/testing/ONBOARDING_EVALUATION_SCENARIO.md) — isolated scenario with **`zqk-ts`** / **`ZQK_TS_TEST_ROOT`** (optional after first-run comfort).
