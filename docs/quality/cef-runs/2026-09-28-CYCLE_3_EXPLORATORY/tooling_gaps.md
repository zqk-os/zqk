# Tooling Gaps & Opportunities — Cycle 3

1. **Automated TUI Script Testing**:
   - The interactive REPL in `cmd/zqk/query/repl.go` was verified with unit tests and simulated pipe stdin. A pseudo-TTY driver for complex curses/ansi terminal testing could further enhance coverage.
2. **Dynamic Query Plan Profiler**:
   - Future cycles may add an `EXPLAIN` or `ANALYZE` meta-command in the ZPARQL REPL to output query graph traversal cost models.
