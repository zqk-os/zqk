# Error Formatting & Message Quality Standards (L:F-USA-02 / CRIT-CEF-R8L-USA-02)

## Standard Template
All error messages across CLI entrypoints and kernel services MUST follow the standard error template:

```
<operation> on <subject> failed: <cause>
```

### Examples
- `failed to resolve path alias for '.zqk/process/backlog': alias not found in cache`
- `unable to acquire file lock on '.zqk/wal/journal.lock': lock held by pid 4122`
- `cannot promote criterion 'CRIT-123': linked parent backlog item is not active`

### Invariants
1. Lowercase start without trailing punctuation (standard Go convention).
2. Always include subject identifier (path, ID, alias, key).
3. Always wrap underlying cause using `%w` or `errfmt.Wrap`.
