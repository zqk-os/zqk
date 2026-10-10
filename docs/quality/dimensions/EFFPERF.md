# Continuous Verification Matrix: Dimension EFFPERF

## Overview
- **Code**: `EFFPERF`
- **Name**: Efficiency, Complexity, and Resource Hygiene
- **Applicable File Classes**: `go_prod`, `go_test`, `script`

## Evaluation Standards & Check Rules

| Rule ID | Category | Check Description | Severity | Allowed Scope |
| :--- | :--- | :--- | :--- | :--- |
| `EFFPERF-001` | Time Complexity | Nested loops over unbounded slices or maps creating $O(N^2)$ bottlenecks. | High | Prohibited (use index lookups or maps) |
| `EFFPERF-002` | Context Deadlines | Blocking I/O, IPC sockets, or HTTP requests without context timeout/cancellation. | Critical | Mandatory `context.WithTimeout` |
| `EFFPERF-003` | Memory Buffers | Reading unbounded streams via `io.ReadAll` without size limits. | High | Must use `io.LimitReader` |
| `EFFPERF-004` | Channel Leaks | Unbuffered channels with missing receivers or unbounded buffer growth. | High | Bounded worker pools only |
