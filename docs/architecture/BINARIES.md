# ZQK Binary Directory & CLI Entry Points (K:F-USA-003 / CRIT-CEF-R8K-USA-003)

**Last Verified:** 2026-08-31


## Overview
ZQK compiles into dedicated operational binaries tailored for specific runtimes, execution boundaries, and daemon roles.

| Binary | Source Entrypoint | Target Audience | Purpose |
| :--- | :--- | :--- | :--- |
| **`zqk`** | `cmd/zqk/main.go` | Developers, AI Agents, Operators | Main command-line interface for knowledge kernel operations, object management, scheduler control, and workflows. |
| **`zqk-community`** | `cmd/zqk-community/main.go` | Open Source Community | Apache-2.0 open-core distribution stripped of proprietary extensions. |
| **`zqk-mcp`** | `cmd/mcp-simple/main.go` | AI Agent Hosts (Claude, Cursor, Antigravity) | Model Context Protocol (MCP) server exposing kernel tools over JSON-RPC stdio. |
| **`ensure_scheduler_running`**| `cmd/utilities/ensure_scheduler_running/main.go` | Crontab / System Supervisors | Safe, idempotent watchdog ensuring the scheduler daemon stays running without process flapping. |
| **`zqk-neuron`** | `cmd/utilities/av-subkernel/main.go` | Agent Orchestration Cells | High-order reasoning and cognitive reflection worker for autonomous execution swarms. |
| **`zqk-muscle`** | `cmd/utilities/github-steward-subkernel/main.go` | GitHub Automation / Worker Cells | Deterministic execution engine for CI/CD, Git synchronization, and CAS state commit. |
| **`zqk-heart`** | `cmd/utilities/heart/main.go` | System Supervisors | Keepalive heartbeat monitor and ambient healthcheck daemon. |
| **`zqk-lung`** | `cmd/utilities/lung/main.go` | Storage & Memory Subsystems | Log compaction, WAL tail pruning, and snapshot retention manager. |
