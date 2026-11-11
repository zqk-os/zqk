# ZQK Context Paging — Design Note (v0.1 Thin Scope)

**BLI:** `[REDACTED-ID]` | **Plan:** `[REDACTED-ID]`
**Scope:** Documentation of current state + v0.1 design intention. Full MMU implementation is v0.2+.

---

## Problem statement

Agent context windows have hard limits (~128k tokens for frontier models, much less for local inference). A long-running research or planning agent will hit this wall:

```
Session start:   hot context = current task + recent history   (~8k tokens)
After 20 turns:  hot context grows unbounded                   (~40k tokens)
Window blowup:   agent loses early context → drift + errors    (undefined)
```

The naive fix — truncation — loses state. The ZQK fix: treat the kernel as the source of truth, not the prompt.

---

## What exists today

| Component | Location | Status |
|-----------|----------|--------|
| `CacheMode` enum | `pkg/context/cache_mode.go` | ✅ (test sync only — not episodic) |
| Local hot pages (`sessionpaging`) | `pkg/sessionpaging/` | ✅ (bounded disk pages; no Studio deps) |
| Graph store provider | `pkg/graph/provider/` | ✅ (Neo4j + MemGraph) |
| Graph state lock | `pkg/graph/state_lock.go` | ✅ |
| Object persistence (CAS) | `pkg/storage/` | ✅ (all kernel objects persist) |
| `high_volume_event_cache.go` | `pkg/storage/` | ✅ (write-behind cache) |
| Cold episodic store API | — | ❌ Not implemented |
| Token budget enforcement | — | ❌ Not implemented |
| Hot→cold eviction policy | — | ❌ Not implemented (local LRU bound only) |

**Key insight:** ZQK already avoids the worst-case. Since agent state lives in kernel objects (not prompt text), `zqk workflow whats-next` always reconstructs accurate current state in ~50 tokens — the agent queries the kernel rather than reconstructing from conversation history. This is a partial solution to window blowup without a full pager. Stranger sessions also keep hot snippets via `pkg/sessionpaging` under `.zqk/state/session_pages/` without Studio-only paging modules.

---

## v0.1 position (honest)

ZQK prevents drift via kernel-managed state. A ZQK agent that hits context limits can:

1. **Restart cleanly** — `zqk workflow whats-next` gives full current mission with ~50 tokens
2. **Query last checkpoint** — `zqk object list backlog_item --filter status=in_progress` shows live work
3. **Keep local hot pages** — `pkg/sessionpaging` stores bounded pages on disk (fail-closed if a Studio paging module path is requested)
4. **Hand off to peer agent** — `assignee_persona_ref` transfers ownership; new agent starts cold with kernel state

This is not a full virtual memory pager, but it prevents the primary failure mode (state loss on context limit) without Studio paging dependencies. Gate: `scripts/open-core/check-community-session-paging.sh`.

---

## v0.2 design intention

### Cold episodic store

```
Hot context (bounded window):
  Active BLIs, current sprint, recent decisions     ← always in prompt

Cold episodic store (graph-backed):
  Completed task history                            ← Neo4j / MemGraph
  Prior session search results                      ← zqk object create episode
  Long-running research chunks                      ← queryable by agent on demand
```

**Proposed API (not yet implemented):**
```sh
# Snapshot current session to cold store
zqk context snapshot --session-id <id> --note "completed research phase 1"

# Query cold store from new session
zqk context recall --query "go concurrency patterns" --limit 5

# Agent calls via MCP:
# tools/call: zqk-community_context_recall → returns structured episode refs
```

### Token budget enforcement

```go
// Proposed: pkg/context/token_budget.go
type TokenBudget struct {
    HardLimit  int   // e.g., 100_000 — trigger eviction
    SoftLimit  int   // e.g.,  80_000 — start pre-evicting
    CurrentUse int64 // atomic counter
}
```

### Hot→cold eviction trigger

1. Agent estimates current token usage (or host tracks via LLM response metadata)
2. When `CurrentUse > SoftLimit`: kernel snapshots least-recently-used objects to cold store
3. Agent receives compact summary refs instead of full object payloads

---

## Contribution guide

The paging layer is a good first contribution for the OSS community:

| Area | Difficulty | Files |
|------|------------|-------|
| `episode` object kind | Easy | `.zqk/process/kind/` |
| `context snapshot` CLI command | Medium | `cmd/zqk/context/` |
| `context recall` MCP tool | Medium | `pkg/mcp/handler.go` |
| Token budget counter | Medium | `pkg/context/token_budget.go` |
| Hot→cold eviction policy | Hard | `pkg/storage/` + `pkg/graph/` |
