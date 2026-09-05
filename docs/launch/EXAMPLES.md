# ZQK — Three Production Examples

> Working patterns for autonomous agent deployments with ZQK.

---

## Example 1: Jailed File/Terminal Worker

**Use case:** An agent that can read/write files and run terminal commands — but only within a declared capability surface. Forbidden operations are blocked at the kernel level before execution.

```sh
# 1. Initialize project
mkdir file-worker && cd file-worker
zqk system init --project-name file-worker

# 2. Create a backlog item scoped to a specific file path
zqk object create backlog_item \
  --field 'title=Summarize all .go files under pkg/' \
  --field 'category=Engineering' \
  --field 'context={"allowed_paths": ["pkg/"], "deny_write": true}'

# 3. Assign to AI agent with restricted role
zqk object update <BLI_ID> \
  --field 'assignee_persona_ref=PER-FILE-READER-ONLY'

# 4. Expose via MCP — agent can only call tools the kernel permits
zqk-community mcp serve --stdio
```

**What ZQK enforces:**
- Role enforcement gate rejects writes outside declared paths
- Agent cannot escalate to admin operations (no `ZQK_ADMIN_API_KEY`)
- LLM call timeout: 300s max — hung file reads are killed

**Forbidden action demo:**
```
Agent attempts: write to /etc/hosts
Kernel response: DENIED — role PER-FILE-READER-ONLY has no write_fs capability
No side effect. Agent receives structured error. Host logs event.
```

---

## Example 2: MCP DB / GitHub Agent

**Use case:** An agent that manages GitHub Issues and a local database via MCP tools — capability-gated so it cannot delete repositories or access unrelated repos.

```sh
# 1. Set up project with GitHub + DB context
mkdir gh-agent && cd gh-agent
zqk system init --project-name gh-agent

# 2. Create mission + goals
zqk object create mission \
  --field 'title=Triage GitHub issues and update DB status'

# 3. Connect AI tool via MCP
# In Cursor mcp.json:
{
  "mcpServers": {
    "zqk": {
      "command": "zqk-community",
      "args": ["mcp", "serve", "--stdio"]
    }
  }
}

# 4. Agent workflow: discover → claim → execute → verify
# Agent calls: tools/call zqk-community_object_backlog_item_list
# Agent calls: tools/call zqk-community_object_update (status=in_progress)
# Agent calls: tools/call zqk-community_report_blockers
```

**ZQK provides:**
- Structured workflow state (goal → plan → BLI → task)
- Agent cannot bypass kernel for direct DB/API calls — all ops go through MCP tools
- Audit trail: every tool call logged with agent identity + timestamp
- Multi-agent safe: TPM + AGY can share the same kernel without collision

---

## Example 3: Long-Running Research with Context Paging

**Use case:** A research agent running overnight across hundreds of documents. Context window fills up — ZQK pages hot context to cold episodic store and resumes without loss.

```sh
# 1. Create a long-horizon research plan
zqk object create priority_plan \
  --field 'title=Research: Go concurrency patterns in open-source repos' \
  --field 'horizon=long'

# 2. Seed with backlog items (one per research chunk)
zqk object create backlog_item --field 'title=Analyze top-50 Go repos' \
  --field 'priority_plan_ref=<PRI_ID>'

# 3. Start scheduler — processes items in background
zqk scheduler start

# 4. Monitor progress (survives agent restarts)
zqk workflow whats-next     # always shows live kernel state
zqk object list backlog_item --filter status=in_progress

# 5. When context fills: kernel snapshots hot state → graph store
# Agent restarts → queries kernel → resumes from last checkpoint
# No context reconstructed from prompt text — kernel is the source of truth
```

**Why this works without drift:**
- Agent state lives in the kernel (Neo4j / MemGraph), not in the LLM's context
- `zqk workflow whats-next` reconstructs current mission from live objects
- Long-running work survives: agent crashes, IDE restarts, context window limits
- Multiple agents can hand off the same research task via `assignee_persona_ref`
