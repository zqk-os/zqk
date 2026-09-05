# ZQK Environment Variables Reference Guide

This document provides a comprehensive quick reference for all environment variables supported by the Zen Quantum Kernel (ZQK) system. All brand-prefixed variables are prepended with `ZQK_` at runtime.

---

## 1. Scheduler Configuration

These variables control the execution behavior, concurrency limits, and timeouts of the ZQK background scheduler daemon.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_SCHEDULER_DEFAULT_PACKAGE_CONCURRENCY`** | Integer | `1` | Fallback max concurrent `run_wrapper` jobs per Go package path. Increase this (e.g., `8` or `16`) to execute test bundles concurrently. |
| **`ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX`** | Duration | `2h` | Maximum wall time a dispatch context may wait on goroutine gates/ceiling gates before abandoning the run. |
| **`ZQK_SCHEDULER_GOROUTINE_CAP`** | Integer | - | Hard limit on the maximum number of concurrent goroutines the scheduler can spawn. |
| **`ZQK_SCHEDULER_MAX_WALL_DURATION`** | Duration | - | Graceful shutdown timer. The daemon will automatically stop after this duration from startup (e.g., `"45m"`, `"2h"`). |
| **`ZQK_SCHEDULER_TRIGGERED_POOL_SIZE`** | Integer | - | Concurrency pool size for triggered/immediate jobs. |
| **`ZQK_SCHEDULER_DAEMON_BIN`** | String | - | Absolute path to the scheduler daemon binary. |
| **`ZQK_SCHEDULER_DAEMON_MODE`** | String/Bool| - | Controls scheduler startup mode (daemon vs foreground). |
| **`ZQK_SCHEDULER_LOGS_CONFIG`** | String | - | Configuration path/JSON for scheduler log output routing. |
| **`ZQK_SCHEDULER_MAINTENANCE_CONFIG`** | String | - | Configuration path/JSON for scheduler maintenance windows. |

---

## 2. LLM & AI Providers

Configuration details for LLM clients used by the ZQK agent and analytical subsystems.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_LLM_PROVIDER`** | String | `"gemini"` | The primary LLM provider to use (e.g., `"gemini"`, `"openai"`, `"qwen"`). |
| **`ZQK_GEMINI_API_KEY`** | String | - | API key specifically for the Google Gemini API. |
| **`ZQK_LLM_API_KEY`** | String | - | Fallback API key for the primary LLM client. |
| **`ZQK_LLM_BASE_URL`** | String | - | Custom base URL for the LLM API client (useful for local proxying). |
| **`ZQK_LLM_CHAT_MODEL`** | String | - | Name of the chat model to use (e.g., `gemini-1.5-pro`). |
| **`ZQK_LLM_EMBED_MODEL`** | String | - | Name of the vector embedding model to use. |
| **`ZQK_QWEN_API_KEY`** | String | - | API key specifically for Qwen model endpoints. |
| **`ZQK_QWEN_BASE_URL`** | String | - | Base URL specifically for Qwen model endpoints. |

---

## 3. Testing, Scenarios & Validation

Configuration for test execution, validation rules, and automation bypasses.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_TEST_MODE`** | Boolean | `false` | Enables test-specific behaviors and isolated temporary storage paths. |
| **`ZQK_TEST_VERBOSE`** | Boolean | `false` | Enables verbose test output during Go test execution. |
| **`ZQK_DISABLE_CRITERIA_AUTO_VALIDATE`** | Boolean | `false` | When set to `true`/`1`, the scheduler will NOT automatically mark a requirement/criteria as validated on passing test-bundles. |
| **`ZQK_BYPASS_VERIFICATION_OUTCOME_AUTHORITY`** | Boolean | `false` | Break-glass override. Allows manual (non-system) updates to set criteria verification outcomes. For testing only. |
| **`ZQK_ENABLE_CLI_SCENARIO_TESTS`** | Boolean | `false` | Enables executing CLI command integration scenario tests. |
| **`ZQK_ENABLE_SPEC_CELL_INTEGRATION_TESTS`** | Boolean | `false` | Runs integration tests for greenfield init, bootstrap specs, and update-specs. |
| **`ZQK_ENABLE_SPEC_CELL_REQ019_VALIDATE`** | Boolean | `false` | Runs `update-specs --validate` to enforce schema conformance during spec integration tests. |
| **`ZQK_CVS_LEDGER_MAX_FAILURES`** | Integer | - | Caps the maximum number of failed tests recorded under `convergence_session` ledger. Set `0` for unlimited. |
| **`ZQK_TEST_SKIP_VALIDATION`** | Boolean | `false` | Bypasses core instance validations on test runs. |
| **`ZQK_SKIP_SPEC_SCHEMA_VALIDATION`** | Boolean | `false` | Disables spec schema validation checks during save. |

---

## 4. Model Context Protocol (MCP)

These variables configure the built-in MCP server daemon (`zqk-mcp`) and client logs.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_MCP_TRACE`** | Boolean | `false` | Enables tracing for MCP tool calls. |
| **`ZQK_MCP_TRACE_FILE`** | String | - | Output path for MCP trace logs (now dynamically routed to `.zqk/logs/mcp-trace.log`). |
| **`ZQK_MCP_ROLES`** | String | - | Comma-separated list of RBAC roles granted to the MCP client. |
| **`ZQK_MCP_PERMISSIONS`** | String | - | Custom permissions block for the MCP server. |
| **`ZQK_MCP_ACCOUNT_ID`** | String | - | Associated system account ID for the MCP connection. |

---

## 5. Storage & Retention Configuration

Manage chunk compression, metadata storage engine, and volume retention thresholds.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_STREAM_STORAGE_ENABLED`** | Boolean | `true` | Enables path-based log stream writing and CAS filesystem storage. |
| **`ZQK_METRICS_CHUNK_RETENTION_DAYS`** | Integer | `7` | Number of days to keep metric chunk logs (e.g. `object_volume`, `stream_volume`) before cleanup. |
| **`ZQK_ROLLBACK_RETAIN_COUNT`** | Integer | - | Max number of history snapshots to retain per object. |
| **`ZQK_ROLLBACK_RETAIN_DURATION`** | Duration | - | Duration threshold to keep history snapshots. |
| **`ZQK_ROLLBACK_CAPTURE_DISABLED`** | Boolean | `false` | Set `true` to completely disable capturing history snapshots for undo/rollback. |

---

## 6. Graph Database Connection

Used to hook up Neo4j/MemGraph databases for graph-based semantic indexing.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_GRAPH_ENABLED`** | Boolean | `false` | Enables backend synchronization with the Graph database. |
| **`ZQK_GRAPH_DATABASE`** | String | - | Name of the database to connect to. |
| **`ZQK_GRAPH_HOST`** | String | - | Host address of the Graph database server. |
| **`ZQK_GRAPH_PORT`** | Integer | - | Port of the Graph database server. |
| **`ZQK_GRAPH_USERNAME`** | String | - | Username for Graph database authentication. |
| **`ZQK_GRAPH_PASSWORD`** | String | - | Password for Graph database authentication. |
| **`ZQK_GRAPH_POOL_SIZE`** | Integer | - | Maximum size of the connection pool to the database. |

---

## 7. UI, Prompts & Client Automation

Configuring ambient terminal output, editor sync, and UI preferences.

| Variable Name | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **`ZQK_CURSOR_PASTE_PREFIX_STEPS`** | String | - | Comma-separated keystroke tokens (e.g. `escape`, `cmd_l`) used before `⌘V` in Cursor paste automation. |
| **`ZQK_AGENT_PROMPT_KEYSTROKE_LOG`** | String | - | Path where keystroke logs are written to assist editor context sync. |
| **`ZQK_AGENT_PROMPT_DELIVERY_HTTP_URL`** | String | - | Optional HTTP webhook URL where agent prompt markdown payloads are POSTed. |
| **`ZQK_AGENT_PROMPT_DELIVERY_HTTP_BEARER`**| String | - | Bearer token for webhook request authorization. |
| **`ZQK_DEMO_MODE`** | Boolean | `false` | Strips sensitive logs and formats console outputs for presentations. |
| **`ZQK_ENABLE_AMBIENT_WATCHER`** | Boolean | `false` | Set `true` to enable automatic background file watch/ingest. |
