# Quick Create: Rapid System Objects from Text or Files

**Last Verified:** 2026-09-09

The **`zqk new object`** command allows you to rapidly create system objects (backlog items, decisions, questions, etc.) from markdown, text files, or inline content so you can capture ideas without writing YAML.

## Commands

Use `zqk new object <kind>` and specify either `--file` or `--content`.

| Example | Kind | Title source | Body/context |
|---------|------|--------------|--------------|
| `zqk new object backlog_item` | backlog_item | First `#` line or first line | description |
| `zqk new object decision` | decision | First `#` line or first line | description |
| `zqk new object question` | question | First `#` line or first line | description |

## Usage

**From a file (.md or .txt):**

```bash
zqk new object backlog_item --file=notes.md
zqk new object decision --file=adr-001.md
zqk new object question --file=open-questions.txt
```

**From inline content:**

```bash
zqk new object backlog_item --content="Add Redis cache

We need a shared cache for sessions and feature flags."
```

**With explicit title (overrides first line):**

```bash
zqk new object backlog_item --title="Add Redis cache" --content="We need a shared cache..."
```

## Parsing rules

- **Title:** First line that starts with `#` (heading) is used as title (the `#` is stripped). If there is no `#` line, the first non-empty line is the title.
- **Body:** Everything after the title line becomes the object's description (trimmed).

All objects are created on the draft plane. Use `zqk object update <id> ...` to add more fields later or run `zqk object promote <id>` to persist the object.

## References

- Object creation: `zqk object create <kind> --file=...` or `--data=...`
- Draft plane: `zqk new object <kind> --title=...`
