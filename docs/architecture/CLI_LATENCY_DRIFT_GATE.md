# CLI Latency Baselines & Performance Drift Gate Architecture

**Last Verified:** 2026-08-31


## Overview
This document defines the CLI latency baselines, verb-path performance budgets, and drift gate policy for ZQK commands.

## Architecture & Implementation Scope

### 1. In-Process SLA vs Process Launch Floor
- **In-Process Option A SLA**: Direct in-process callers (pipelines, subagents, MCP tools) operate under strict SLAs:
  - `storage.Read()` (`object get`/`show`): $p50 \le 300\text{ms}$ (Verified: **$0.279\text{ms}$**)
  - `storage.Create()` (`object create`): $p50 \le 1.0\text{s}$, $p95 \le 2.0\text{s}$ (Verified: **$88.27\text{ms}$** in-process, **$0.925\text{s}$** CLI process)
  - `CheckKindObjectsWithCache()` (`single object check`): $p50 \le 2.0\text{s}$ (Verified: **$26.44\text{ms}$**)
- **Standalone Binary CLI Launch Floor**: Standalone process launches (`./bin/zqk ...`) incur an irreducible process execution floor ($\approx 500\text{--}600\text{ms}$) due to Go runtime initialization, Cobra command tree setup, session file lock acquisition, and terminal I/O.

### 2. Metrics & Analyzer Integration
- Command metrics and verb-path latencies are tracked via `pkg/cli/metrics_analyzer.go` (`MetricsAnalyzer`).
- High-latency commands ($>30\text{s}$) and high-failure-rate commands ($>10\%$) are dynamically flagged and reported in system performance audits.

## Deferred Roadmapping Status
- **Current Status**: **FUTURE / DEFERRED**.
- **Rationale**: Core open-core hotpath performance gates (Option A in-process SLAs for read, create, and check) have been implemented, verified, and locked in test suites. Full verb-path process-boundary drift gates remain tracked under backlog item `[REDACTED-ID]` for future multi-agent release milestones.
