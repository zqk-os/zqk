---
diagram_id: D-CONTAINER-01
type: c4_container
title: "ZQK Core Subsystems & Containers"
anchors:
  - path: cmd/zqk/main.go
    symbol: main
    note: "primary CLI entrypoint"
  - path: pkg/storage/object_storage_file.go
    symbol: FileObjectStorage
    note: "primary filesystem object storage implementation"
  - path: cmd/zqk-vet/main.go
    symbol: main
    note: "verification engine entrypoint"
claims:
  - "CLI commands interact with storage and verification engines"
evidence_grade: E2
---

# ZQK Core Subsystems & Containers (C4-L2)

```mermaid
graph TD
    CLI["cmd/zqk (CLI Engine)"] -->|Storage API| Storage["pkg/storage (Object Storage / WAL / CAS)"]
    CLI -->|Command Builders| Builders["pkg/cli/bldr_cli_cmd_v1 (Command Specs & Handlers)"]
    CLI -->|Swarm Execution| Swarm["pkg/swarm (Task & Swarm Orchestration)"]
    CLI -->|Workflow Engine| Workflow["pkg/workflow (WhatsNext & Status)"]
    Vet["cmd/zqk-vet (Verification Engine)"] -->|Hygiene / Tree / Payload| Storage
    Storage -->|Atomic Disk Writes| Filesystem[".zqk/ filesystem data store"]
```
