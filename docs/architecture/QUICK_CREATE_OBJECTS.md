# Quick Create: One-Click System Objects from Text or Files

The **`zqk quick`** commands create system objects (backlog items, decisions, questions) from markdown, text files, or inline content so you can capture ideas without writing YAML.

## Commands

| Command | Kind | Title source | Body/context |
|---------|------|--------------|--------------|
| `zqk quick backlog-item` | backlog_item | First `#` line or first line | description |
| `zqk quick decision` | decision | First `#` line or first line | context |
| `zqk quick question` | question | First `#` line or first line | context; optional `--answer` |

## Usage

**From a file (.md or .txt):**

```bash
zqk quick backlog-item --file=notes.md
zqk quick decision --file=adr-001.md
zqk quick question --file=open-questions.txt
```

**From inline content:**

```bash
zqk quick backlog-item --content="Add Redis cache

We need a shared cache for sessions and feature flags."
```

**With explicit title (overrides first line):**

```bash
zqk quick backlog-item --title="Add Redis cache" --content="We need a shared cache..."
```

**Question with answer (Q&A capture):**

```bash
zqk quick question --content="How do we deploy?" --answer="Via CI pipeline and ArgoCD."
```

## Parsing rules

- **Title:** First line that starts with `#` (heading) is used as title (the `#` is stripped). If there is no `#` line, the first non-empty line is the title.
- **Body:** Everything after the title line becomes the object’s description or context (trimmed).

All objects are created through the normal object create flow (IDs generated as usual; validation and lifecycle apply). Use `zqk object update <id> ...` to add more fields later.

## References

- Object create: `zqk object create <kind> --file=...` or `--data=...`
- Specs: `docs/architecture/_internal/object_specs/` (backlog_item, decision, question)
