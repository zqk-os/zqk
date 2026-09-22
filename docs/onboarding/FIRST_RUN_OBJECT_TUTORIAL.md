# First-run object tutorial (template → create → get → update)

**Audience:** New users after `zqk init` (see [community first-run](./COMMUNITY_FIRST_RUN.md) and [architecture (this SKU)](../architecture/README.md)).
**Backlog:** Tracked as part of alpha CLI launch work (see priority plan *CLI alpha launch readiness*).

This path uses the **`question`** kind as a **small** object: few required fields, suitable for learning `object template` / `object create` without editing large YAML. Adjust the kind if your org standardizes another “low-risk” kind.

## Prerequisites

- Shell at the **project root** (directory containing `go.mod` and `.zqk/` after init).
- `zqk` on `PATH` (or invoke `./bin/zqk` from a built tree).

## 0. Orient (recommended)

Before browsing backlog with `object list`, run:

```bash
zqk workflow whats-next --format json
```

That is the community-preferred first command after init.

## 1. Canonical Flow: Mint onto Draft Plane (`new object`)

The fastest, standard way to create any object is via the draft plane:

```bash
zqk new object question --title "First-run sanity question"
```

This immediately persists the object onto `.zqk/object_drafts/` with a stable ID (e.g. `QUE-178...`) without polluting the CAS until it is verified and ready.

## 2. Enrich and Update

Flesh out schema fields on the draft object:

```bash
zqk object update <QUESTION_ID> --field question_text="What is the canonical object creation flow?"
```

## 3. Promote to CAS

Once definition of done and schema requirements are satisfied, advance the lifecycle into CAS:

```bash
zqk object promote <QUESTION_ID>
```

---

## Alternative: YAML Template Scaffold Path

For complex objects requiring offline editing:

```bash
zqk object template question --include-optional=false -o /tmp/zqk-first-question.yaml
```

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
- **Validation errors:** Read the message; fix the cited field. For kind-specific rules, see `.zqk/specs/objects/<kind>.yaml` or `zqk system check <kind> <id>` after create.
- **Status/lifecycle rejection on update:** Show allowed status values with `zqk object <kind> fields` and choose a valid transition from the lifecycle.
- **Scheduler daemon not running:** Start it with `zqk scheduler start`. Do not treat `--allow-degraded` as the default fix — that flag means partial or degraded results are intentionally accepted (see `docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md`).
- **Long-running tests:** Prefer `zqk scheduler scan-tests` for package gates; see project scheduler docs and `PRE_CHANGE_CHECKLIST.md` section 6 for scope.

- [AI Agent Onboarding Guide](./AI_AGENT_ONBOARDING.md) — Agent directives and workflow discipline on ZQK Core.
- [Architecture Overview](../architecture/README.md) — Core system architecture, Knowledge Kernel, and daemon topology.
- [Community First-Run Guide](./COMMUNITY_FIRST_RUN.md) — First-run setup, MCP installation, and kernel verification.
