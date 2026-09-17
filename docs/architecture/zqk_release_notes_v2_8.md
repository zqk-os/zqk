# ZQK v2.8 Release Notes: Multi-Agent Pipeline Orchestration

## Overview
ZQK v2.8 introduces the foundational architecture for long-running, multi-stage agent workflows. By migrating from monolithic execution loops to decoupled pipeline stages, the system now provides robust state management, explicit handoffs, and rich context hydration.

## Architectural Additions
- **Pipeline Schemas**: Introduced `pipeline_definition` (templates) and `pipeline_execution` (active states) schemas to formalize orchestration.
- **State Machine Orchestrator**: Developed `pkg/pipeline/orchestrator.go` featuring strict, decoupled transitions: `Pending -> PreProcessing -> AgentDispatch -> AgentExecution -> HandoffVerification -> PostProcessing`.
- **Plugin System**: Delivered the `PipelinePlugin` contract for extensible, modular hooks into the execution lifecycle.
- **Context Hydration**: Rolled out `pkg/pipeline/plugins/plugin_context_hydration.go` as the inaugural plugin, responsible for deep codebase search and kernel context aggregation prior to sub-agent dispatch.

## Technical Milestones & Fixes
- Addressed legacy import cycle regressions between `pkg/pipeline`, `pkg/storage`, and `pkg/validation`.
- CLI spec registry generated and synchronized with `PLX-` and `PLD-` object schemas.
- Backlog items `BLI-REDACTED`, `BLI-REDACTED`, and `BLI-REDACTED` successfully completed and merged into the active feature branch.
