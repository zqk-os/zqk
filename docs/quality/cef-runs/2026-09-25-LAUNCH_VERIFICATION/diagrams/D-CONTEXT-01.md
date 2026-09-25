---
diagram_id: D-CONTEXT-01
type: c4_context
title: "ZQK System Context Overview"
anchors:
  - path: cmd/zqk/main.go
    symbol: main
    note: "primary CLI entrypoint"
claims:
  - "Agents and operators interact with the ZQK Knowledge Kernel via CLI and filesystem objects"
evidence_grade: E2
---

# ZQK System Context (C4-L1)

```mermaid
graph TD
    Operator["Human Operator"] -->|CLI commands| CLI["ZQK CLI / Daemon"]
    Agent["Autonomous Agent"] -->|CLI & Task Protocols| CLI
    CLI -->|Read/Write Objects| Kernel["ZQK Kernel Graph & DataCell"]
    CLI -->|Verification| Vet["zqk-vet verification engine"]
    CLI -->|Git Worktree| Worktree["Local Repository Filesystem"]
```
