# MCP Exposure and Trimming

**Status**: Planning  
**Purpose**: Audit what the full MCP server exposes, why Cursor complains, and how to trim tools/prompts/resources so we can switch back to the full server.

---

## 1. Why This Document

- **mcp-simple (zqk-mcp)** was created to get interactive command flows working first; that work was never completed. The plan was to move interactive logic to the full MCP server once it worked.
- **Full MCP server** (`zqk mcp serve`) exposes **too many tools** (and prompts/resources), and **Cursor enforces limits**, so we run into warnings or hard caps.
- The **goal** is to eventually switch back to the full MCP server after **organizing and trimming** tools, prompts, and resources so we stay within client limits and keep the UX manageable.

This doc audits current exposure, summarizes where limits come from, and sketches a curation/trimming approach.

---

## 2. Audit: What Is Exposed Today

### 2.1 Tools

| Source | When | Approx. count | Notes |
|--------|------|----------------|-------|
| **Built-in (synchronous)** | Every init | ~20+ | `RegisterGraphTools` (3), `RegisterEchoTool` (1), `RegisterCommonTools` (5: object_list, object_get, object_count, system_status, system_check), `RegisterInteractiveTools`, `RegisterWorkflowTools` (4), `RegisterMetricsTools` (2), etc. |
| **CLI discovery** | When `rootCommand != nil` and `register_cli_tools` is true (default) | **All leaf cobra commands** | `BootstrapCLITools()` → `DiscoverCLICommands(rootCmd)` → every executable (RunE/Run) command becomes a tool. Can be **dozens** (object list/get/create/update/delete, system status/check, reports/*, domain commands, etc.). |
| **Total (full server)** | Typical | **Well above 40** | Built-in + CLI often exceeds Cursor’s 40-tool warning and can approach or exceed 80. |
| **mcp-simple (zqk-mcp)** | No root command | **Built-in only** (~20+) | No CLI discovery; only tools registered in `registerToolsAndResources` (graph, echo, common, interactive, workflow, metrics). |

**Where it’s defined**

- Built-in: `server_init_helpers.go` (`RegisterGraphTools`, `RegisterEchoTool`, `RegisterCommonTools`, `RegisterInteractiveTools`, `RegisterWorkflowTools`, `RegisterMetricsTools`), plus `RegisterOnboardingPrompts`.
- CLI tools: `cli_bridge_registration.go` (`RegisterCLIToolsWithRootCommandAndConfig`), gated by `server_init_helpers.go` (`shouldRegisterCLITools` from config).
- Config: `server_config.go` — `register_cli_tools` (nil/true = register CLI; false = built-in only).

**Other limit**

- **Tool name length**: Cursor expects `server:tool_name` to fit in **60 characters**. We cap tool names at **53 chars** in `cli_bridge.go` (`sanitizeToolName`).

### 2.2 Prompts

| Source | When | Approx. count | Notes |
|--------|------|----------------|-------|
| **Onboarding prompts** | Every init | **11–12** | `RegisterOnboardingPrompts` → storage/spec file/programmatic fallback. Names: welcome, getting_started, query_help, create_object_guide, common_tasks, object_lifecycle, filter_syntax, role_based_access, execution_context, big_picture, current_role, help. |
| **Spec/storage** | If present | Variable | Optional prompts from `mcp_spec` objects or YAML specs. |

**Where it’s defined**

- `prompts.go` (`RegisterOnboardingPrompts`), `mcp_spec_applier.go` (`ConvertPromptsToSpec`), `server_init_helpers.go` (calls `RegisterOnboardingPrompts`).

### 2.3 Resources

| Source | When | Approx. count | Notes |
|--------|------|----------------|-------|
| **Critical resources** | Every init | Spec-driven | `RegisterCriticalResources` → from `mcp_spec` / `critical_resources.yaml`. Lifecycles, workflows, system health, etc. |
| **Additional discovery** | Every init | **Variable (can be large)** | `DiscoverAdditionalResources` walks **`docs/architecture/architecture`** and registers discovered files (e.g. markdown). Comment in code references **240+ markdown files** in slow discovery. |

**Where it’s defined**

- `resources.go` (`RegisterCriticalResources`, `DiscoverAdditionalResources`), `server_init_helpers.go` (both run in parallel during init).

---

## 3. Where Limits Come From

### 3.1 Cursor (and similar clients)

- **Tools**: **40-tool warning** (performance), **80-tool hard maximum**. Above 40, Cursor may warn; above 80, tools can be rejected. [Community reports: 40-tool limit, 80 max.]
- **Tool name**: **60 characters** total for `server:tool_name` (we already enforce 53 for the name part in `sanitizeToolName`).
- **Prompts / resources**: No specific number documented here; large lists can still affect UX and payload size.

### 3.2 Our code

- **Tools**: No internal cap; we return whatever is registered (`handleToolsList` returns all `s.tools`).
- **Config**: `register_cli_tools: false` already supports “built-in only” (no CLI discovery), which reduces tools but is all-or-nothing.

---

## 4. Sketch: Curated Set and Trimming Approach

### 4.1 Goals

- Support **full MCP server** as the primary way to run MCP.
- Stay **under Cursor’s tool limit** (e.g. ≤40 to avoid warning, or ≤80 hard).
- **Organize** tools (and optionally prompts/resources) by **role**, **workflow**, or **explicit allowlist** so we expose a focused set by default.

### 4.2 Options for tools

1. **Allowlist (recommended baseline)**  
   - Config: e.g. `mcp.tools.allowlist: ["object_list", "object_get", "object_create", "system_status", "get_current_priority_plan", ...]`.  
   - At registration time (after built-in + optional CLI discovery), **filter** to only register tools whose name is in the allowlist.  
   - Enables a small, stable set (e.g. ~15–25 tools) for “default” or “cursor” profile.

2. **Blocklist**  
   - Config: `mcp.tools.blocklist: ["reports_*", "internal_*", ...]`.  
   - Drop tools matching patterns. Useful to hide noisy or internal commands.

3. **By role / workflow**  
   - Config: `mcp.tools.profiles: { "minimal": [...], "developer": [...], "admin": [...] }` and `mcp.tools.default_profile: "minimal"`.  
   - Register only tools for the chosen profile. Can be combined with allowlist (profile = allowlist name).

4. **Role- and account-driven tool selection (recommended direction)**  
   - **Idea**: Use the **client’s role and account** (already established at `initialize`) to decide **which tools appear** in `tools/list`, rather than one static allowlist for everyone.  
   - **Why**: We already have account_id, roles, and permissions per connection (from init, agent registry, and account files). Execution already enforces permissions. The gap is that **tools/list** returns the same set for every client, so Cursor sees 40+ tools for everyone.  
   - **Approach**: After init we have the current client’s roles and permissions. In `handleToolsList`, **filter** the registered tools to only those the client is allowed to use (e.g. same logic as `FilterCommandsByPermissions` but for the current connection’s secCtx derived from clientInfo). Each client then sees a list of ≤40 tools tailored to their role/account.  
   - **Config**: Optionally keep an allowlist as a **cap** (e.g. “only ever expose these names”) and apply **role/account filter on top** so the final list is: (allowlist ∩ registered) filtered by role/permissions for this client.  
   - **Implementation notes**: Store resolved roles/permissions per connection (e.g. on the client/session object set during init). In `handleToolsList`, build a temporary security context for the current client and filter `s.ListTools()` with the same permission checks used for CLI discovery.

5. **Keep existing switch**  
   - `register_cli_tools: false` → built-in only (~20+ tools).  
   - No allowlist needed; good for “mcp-simple-like” behavior in the full binary.

Implementation could live in `cli_bridge_registration.go` and/or `server_init_helpers.go`: after building the list of tools to register (from built-in + CLI), apply allowlist/blocklist/profile so that `RegisterTool` is only called for the curated set. Role-based filtering would live in `handleToolsList` (filter the returned list by current client's roles/permissions).

**Recommended combination:** Use **role/account** to select which tools each client sees (filter in `tools/list`), and use **aliases** (see below) to keep the registered set small and functional. Allowlist can remain as an optional cap.

### 4.2a Aliases: fewer tools, same functionality

- **Problem**: CLI discovery registers **one tool per leaf command** (e.g. `object list`, `object list backlog_item`, `object get`, `object create`, …), which quickly exceeds 40 tools. Built-in tools already provide **one tool, many uses** (e.g. `object_list` with a `kind` parameter covers all "object list [kind]" variants).
- **Idea – aliases**: Prefer **a small set of generic/alias tools** that accept parameters and dispatch to the right CLI command, instead of exposing every CLI leaf as a separate tool. That **minimizes the number of tools** exposed while **preserving full functionality**.
- **Today**: We already have built-in "alias-style" tools: `object_list`, `object_get`, `object_count`, `system_status`, `system_check`, workflow tools like `get_current_priority_plan`, etc. They take `kind`, `id`, `filter`, etc., and map to one or more CLI invocations. CLI discovery adds many more tools that duplicate or specialize the same operations.
- **Options**  
  1. **Built-in-only / minimal CLI (alias-first)**  
     - Set `register_cli_tools: false` and rely on built-in tools only. No per-command CLI tools; one `object_list` (with `kind`), one `object_get` (with `id`), one `object_create` (with kind + args), etc. Stays under 40 tools by design.  
  2. **Alias mode config**  
     - New config: e.g. `mcp_server.tools.alias_mode: true`. When true, **do not** register every CLI leaf as a tool. Instead, register only a fixed set of "alias" tools (same as built-in list above, or a curated list) that accept `_command_path` or `kind` and forward to the CLI. So we still use the CLI for execution, but we expose ~15–25 alias tools instead of 50+ CLI tools.  
  3. **Explicit alias map**  
     - Config lists alias tool names and the CLI commands they cover (e.g. `object_list` → `object list`, `object list <kind>`). Registration only creates the alias tools; execution resolves the alias to the concrete command from args.  
- **Benefit**: Fewer tools in `tools/list` (good for Cursor), same or better functionality (one `object_list` with `kind` is more flexible than 20 separate "object list X" tools). Role/account can then further filter this smaller set so each client sees only the alias tools they're allowed to use.

### 4.3 Options for prompts

- **Allowlist**: Only register prompts whose name is in a config list (e.g. welcome, getting_started, query_help, current_role).  
- **Cap**: Register at most the first N prompts (e.g. 10).  
- **Profile**: Same idea as tool profiles (e.g. “minimal” = 5 prompts, “full” = all).

### 4.4 Options for resources

- **Critical only**: Disable or limit `DiscoverAdditionalResources` so only critical resources (from spec) are exposed; avoids 240+ discovered resources.  
- **Cap**: Register at most N resources from discovery.  
- **Allowlist/patterns**: Only register resources whose URI or path matches allowed patterns (e.g. `lifecycles_guide`, `health_monitoring`).

### 4.5 Config shape

**Implemented:** Tools allowlist and alias mode in `.zqk/mcp/config.yaml`:

```yaml
mcp_server:
  register_cli_tools: true
  tools:
    allowlist:
      - zqk_object_list
      - zqk_object_get
      - zqk_object_count
      - zqk_system_status
      - zqk_get_current_priority_plan
      - zqk_get_current_backlog_item
      # Add other tool names to stay under Cursor's 40/80 limit
    # When true, only built-in/alias tools are registered (no per-command CLI tools). Reduces count while keeping functionality.
    alias_mode: false
```

- When `tools.allowlist` is non-empty, only these tool names are exposed after registration (built-in + CLI); when empty, all registered tools are exposed (backward compatible).
- When `tools.alias_mode` is true, CLI leaf commands are not registered as tools; only built-in tools (e.g. `zqk_object_list`, `zqk_system_status`) are exposed. Use with or without allowlist.
- **Role/account:** The client's security context (from initialize) is set on the server before tool registration, so `BootstrapCLITools` filters CLI commands by that client's roles/permissions. Each client therefore sees a tool set filtered by their role/account.

**Not yet implemented (sketch):**

```yaml
# prompts allowlist, resource discovery cap, etc.
  prompts:
    allowlist: ["welcome", "getting_started", "query_help", "current_role"]
  resources:
    discovery_enabled: false
    max_discovered: 50
```

---

## 5. Tool name discovery

Tool names do not always match CLI commands intuitively. Use one of these to get the **exact names** to put in `mcp_server.tools.allowlist`:

1. **CLI (recommended)**  
   From the project root, run:
   ```bash
   zqk mcp list-tools
   ```
   This prints one tool name per line (the same set the server would expose after applying the allowlist). Use `--with-command` to show the corresponding CLI path for each tool when available (e.g. `object list`).

2. **MCP `tools/list`**  
   After the server has been initialized (client sent `initialize`), call the `tools/list` method. The returned tool names are the authoritative set. Useful when debugging from an MCP client.

**Naming rules (for reference):**

- **Built-in tools**: Name is `GetToolName(suffix)` → brand prefix + `_` + suffix (e.g. `zqk_object_list`, `zqk_get_current_priority_plan`). Brand comes from the executable (e.g. `zqk` or `zqk-mcp`).
- **CLI-discovered tools**: Name is `sanitizeToolName(command_path)` → command path with spaces replaced by underscores, no brand prefix (e.g. `object list` → `object_list`). Same capability can appear as both a built-in name (`zqk_object_list`) and a CLI name (`object_list`) depending on registration order; the **definitive list** is what `list-tools` or `tools/list` returns.

Copy the exact names from `zqk mcp list-tools` into `tools.allowlist` in `.zqk/mcp/config.yaml`.

---

## 6. Summary

| Item | Current (full server) | Limit / problem | Trimming direction |
|------|------------------------|-----------------|---------------------|
| **Tools** | Built-in + all CLI leaves (often 40+) | Cursor 40 warn / 80 max | Allowlist or profile; keep `register_cli_tools: false` as option |
| **Prompts** | 11–12 + optional spec | UX/payload | Allowlist or cap |
| **Resources** | Critical + full architecture discovery (100s) | Startup time, payload size | Critical-only or cap/allowlist |

Implementing **tool allowlist** (and optionally prompts/resources) in config, plus **documenting** “why we trim” and “how to switch back to full server,” gives a clear path to the full MCP server while staying within Cursor’s limits and keeping the surface manageable.

---

## 7. References

- **MCP package**: `pkg/mcp/` (e.g. `server_handlers_list.go`, `server_init_helpers.go`, `cli_bridge_registration.go`, `server_config.go`).
- **Cursor limits**: Community reports (e.g. 40-tool warning, 80-tool max); tool name 60 chars.
- **This doc**: `pkg/mcp/docs/MCP_EXPOSURE_AND_TRIMMING.md`.
