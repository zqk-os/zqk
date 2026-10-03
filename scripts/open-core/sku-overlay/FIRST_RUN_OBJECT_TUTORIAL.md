# First-run object tutorial (template → create → get → update)

**Audience:** After `./bin/zqk system init`.  
**CLI:** default executable token; applybrand rewrites from `brand.executable_name`.

This path uses the **`question`** kind as a small object.

## Prerequisites

- Shell at the project root (directory containing `.zqk/` after init).
- `./bin/zqk` from this tree (or the branded name on `PATH`).

## 0. Orient

```bash
./bin/zqk workflow whats-next --format json
./bin/zqk object list
```

## 1. Create

```bash
./bin/zqk new question --title "First-run sanity question"
```

Or draft from a YAML template:

```bash
./bin/zqk object template question --include-optional=false -o /tmp/zqk-first-question.yaml
# edit required fields, then:
./bin/zqk object create question --file /tmp/zqk-first-question.yaml
```

## 2. Get / update

```bash
./bin/zqk object get <QUESTION_ID> --format yaml
./bin/zqk object update <QUESTION_ID> --field title="Updated title after first get"
```

## 3. Promote when ready

```bash
./bin/zqk object promote <QUESTION_ID>
```

When you no longer need the example, `./bin/zqk object delete <QUESTION_ID>` (or keep it as a local fixture).

## Optional: keep the organism running

```bash
./bin/zqk scheduler start
./bin/zqk scheduler status
```

Init already wrote the starter graph and slim maintenance jobs. The daemon ticks them. CRUD works without it.

## If it fails

- YAML parse: check indentation; `--dry-run` on create.
- Lifecycle rejection: `./bin/zqk object question fields` for allowed statuses.
- Unauthorized / no kernel: `./bin/zqk system init --project-name <name>` from the project directory.
- Optional daemon: `./bin/zqk scheduler start` then `./bin/zqk scheduler status`.
