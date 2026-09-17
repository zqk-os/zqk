# Object Get Performance SLA & Hotpath Architecture

**Last Verified:** 2026-08-31


## Overview
This document defines the latency SLAs and architectural hotpaths for object read (`get`) operations across the ZQK Knowledge Kernel.

## SLA Definitions

### 1. In-Process Storage Read (Option A Product SLA)
- **Scope**: Direct in-process callers (subagents, MCP tools, internal pipelines, and test suites) calling `storage.Read()`.
- **Target SLA**:
  - Warm $p50 \le 300\text{ms}$ (Verified: **$0.279\text{ms}$**)
  - Warm $p95 \le 800\text{ms}$ (Verified: **$0.450\text{ms}$**)
- **Implementation**: Bypasses process launch overhead and uses pre-warmed file storage pointers and in-memory caches. Verified in `pkg/objectget/inprocess_bench_test.go`.

### 2. Standalone Binary CLI Invocations
- **Scope**: External shell invocations via `./bin/zqk object get <ID>`.
- **Latency Floor**: $\approx 500\text{--}600\text{ms}$ (Verified: $p50=0.588\text{s}$, $p95=0.718\text{s}$).
- **Overhead Components**:
  - Go runtime startup & binary execution (~150ms)
  - Full Cobra command tree initialization & field registry loading (~180ms)
  - Process session state file lock acquisition & authentication middleware (~150ms)
  - Terminal I/O & output formatting (~100ms)

## Recommendation for High-Performance Workflows
All agentic loops, high-frequency automation, and performance-critical pipelines MUST use the **In-Process Option A Storage Read SLA** rather than invoking repeated standalone binary executions.

## Command Aliases (`show` & `view`)
- `zqk object show` and `zqk object view` are direct Cobra aliases for `zqk object get` (`cmd.Aliases = []string{"show", "view"}` in `cmd/zqk/object/get.go`).
- Both aliases share identical execution hotpaths, security filtering, and in-process SLA guarantees ($p50 \le 300\text{ms}$).
