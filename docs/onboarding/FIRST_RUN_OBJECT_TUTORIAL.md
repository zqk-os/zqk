# First-run object tutorial (template → create → get → update)

**Audience:** After `./bin/zcom system init`.  
**CLI:** default executable token; applybrand rewrites from `brand.executable_name`.

This path uses the **`question`** kind as a small object.

## Prerequisites

- Shell at the project root (directory containing `.zqk/` after init).
- `./bin/zcom` from this tree (or the branded name on `PATH`).

## 0. Orient

```bash
./bin/zcom workflow whats-next --format json
./bin/zcom object list
```

## 1. Create

```bash
./bin/zcom object template question --include-optional=false -o /tmp/zqk-first-question.yaml
# edit required fields, then:
./bin/zcom object create question --file /tmp/zqk-first-question.yaml
```

Or:

```bash
./bin/zcom new object question --title "First-run sanity question"
```

## 2. Get / update

```bash
./bin/zcom object get <QUESTION_ID> --format yaml
./bin/zcom object update <QUESTION_ID> --field title="Updated title after first get"
```

## 3. Promote when ready

```bash
./bin/zcom object promote <QUESTION_ID>
```

When you no longer need the example, `./bin/zcom object delete <QUESTION_ID>` (or keep it as a local fixture).

## Optional: keep the organism running

```bash
./bin/zcom scheduler start
./bin/zcom scheduler status
```

Init already wrote the starter graph and slim maintenance jobs. The daemon ticks them. CRUD works without it.

## If it fails

- YAML parse: check indentation; `--dry-run` on create.
- Lifecycle rejection: `./bin/zcom object question fields` for allowed statuses.
- Unauthorized / no kernel: `./bin/zcom system init --project-name <name>` from the project directory.
- Optional daemon: `./bin/zcom scheduler start` then `./bin/zcom scheduler status`.
