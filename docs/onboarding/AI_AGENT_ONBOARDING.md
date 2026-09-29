# AI Agent Onboarding & Operating Protocol

Welcome. This document outlines the operational directives, seating configurations, and self-discovery protocols for operating AI agents within a ZQK Knowledge Kernel repository.

---

## 🔌 Agent Seating & Connection Matrix

ZQK is built to interface seamlessly with any AI agent via the Model Context Protocol (MCP) or direct CLI execution. Choose your host below:

### 1. Cursor
- **Directives:** Cursor automatically reads `.agents/AGENTS.md` and `.cursorrules` in your project root.
- **MCP Connection:** Pre-configured in `.cursor/mcp.json`. If configuring manually, add to `.cursor/mcp.json` (or Cursor Settings → Features → MCP):
  ```json
  {
    "mcpServers": {
      "zqk": {
        "command": "zqk",
        "args": ["mcp", "serve"]
      }
    }
  }
  ```
- **Seating Prompt:** In Cursor Composer or Chat, prompt your agent:
  > *"You are paired with the ZQK Knowledge Kernel. Run `zqk workflow whats-next` to discover active goals and tasks, then run `zqk do` to claim and execute work."*

### 2. Claude Desktop & Claude Code
- **Claude Desktop:**
  Auto-install the MCP server configuration:
  ```bash
  zqk mcp install --client claude-desktop
  ```
  Restart Claude Desktop. The ZQK tools icon will appear in the input bar.
- **Claude Code (CLI):**
  Register the ZQK MCP server:
  ```bash
  claude mcp add zqk -- zqk mcp serve
  ```

### 3. Windsurf (Cascade)
- **Directives:** Cascade automatically respects `.windsurfrules` and `.agents/AGENTS.md`.
- **MCP Connection:** Configured in `~/.codeium/windsurf/mcp_config.json`:
  ```json
  {
    "mcpServers": {
      "zqk": {
        "command": "zqk",
        "args": ["mcp", "serve"]
      }
    }
  }
  ```

### 4. Cline / Roo Code (VS Code Extension)
- **Directives:** Cline loads `.clinerules` from the project root.
- **MCP Connection:** Automatically configured into `cline_mcp_settings.json`. Or add via the Cline MCP Settings tab with command `zqk` and args `["mcp", "serve"]`.

### 5. Gemini CLI / Antigravity / Google Agents
- **Directives:** Direct native integration via `.agents/AGENTS.md`.
- **Execution:** Run `zqk` CLI commands directly or connect via MCP standard I/O (`zqk mcp serve`).

### 6. Hermes, OpenClaw & Headless Swarms
- **Direct CLI Execution:** Grant the agent bash permissions. The agent interacts via:
  ```bash
  zqk workflow whats-next --format json
  zqk do <bli-id>
  ```
- **TCP Loopback Proxy:** If your swarm runner connects via TCP instead of stdio:
  ```bash
  zqk mcp proxy --tcp 127.0.0.1:7777
  ```

---

## 🔄 Autonomous Execution Protocol (`zqk do`)

When an agent claims or executes work, use the streamlined single-command loop:

```bash
# 1. Discover active priority plan and shovel-ready backlog items
zqk workflow whats-next

# 2. Claim and begin executing the next prioritized task
zqk do

# 3. Or target a specific Backlog Item
zqk do BLI-001

# 4. Implement code changes, then run verification to latch criteria
zqk do BLI-001 --verify

# 5. Check project health and compliance cake
zqk system check
```

---

## 📜 Core Directives for Autonomous Agents

1. **Kernel Primacy & State Integrity**:
   - Process states, task ownership, roadmaps, and requirements exist exclusively in the Knowledge Kernel.
   - Do not store state, loops, or notes in local vendor-specific scratch directories (such as `.gemini/` or `.cursor/`). If information is not committed to the kernel graph or registered via the CLI, it does not exist.

2. **Session Initialization & Self-Discovery**:
   - On initial contact or when starting a session, run `zqk system agent-onboard` to detect your agent host and register seating.
   - Proactively discover current mission, active priority plans, and backlog items by executing `zqk workflow whats-next`. Do not stop or wait for manual human instructions if active plan work is queued.

3. **Intake & Mutation Boundaries**:
   - Process instances live strictly under `.zqk/process/` and content-addressable storage (CAS).
   - Never write process YAML files directly or bypass the CLI intake layer (`zqk intake` / `zqk object create`). All object creation must satisfy schema validation rules.
   - Project configuration is maintained in `config/zqk.yaml` / `config/zqk-local.yaml`.

4. **Lifecycle & Verification Gates**:
   - Background tasks and test executions must be dispatched through the native scheduler (`zqk scheduler start`).
   - Use `zqk test run` or `zqk do <bli-id> --verify` to verify implementation before completing tasks.
   - Never use direct edits to bypass lifecycle state transitions—always use `zqk object promote` or `zqk do`.
