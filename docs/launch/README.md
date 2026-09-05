# ZQK Launch Docs

**Open-Core v0.1 Day -3/-0 artifacts** | Plan: `[REDACTED-ID]`

---

| Doc | Purpose | Status |
|-----|---------|--------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Host FSM · capability flow · paging · MCP diagram | ✅ |
| [EXAMPLES.md](./EXAMPLES.md) | 3 production examples: jailed worker, MCP DB agent, long-running research | ✅ |
| [BENCHMARKS.md](./BENCHMARKS.md) | Init time, memory/agent, context efficiency vs Python frameworks | ✅ |
| [SHOW_HN.md](./SHOW_HN.md) | Show HN post + README: diagram, quickstart, comparison table | ✅ |

## 60-second demo script

```sh
# Forbidden action blocked by capability engine:
mkdir demo && cd demo
zqk system init --project-name demo

# Create a read-only agent persona (no write capability)
zqk object create backlog_item \
  --field 'title=Attempt privileged write' \
  --field 'context={"role": "read-only"}'

# Via MCP — agent tries to delete; kernel blocks before execution:
# tools/call: zqk-community_object_backlog_item_delete → DENIED (role: read-only)
# Graceful recovery: agent receives structured error, logs event, continues
```
