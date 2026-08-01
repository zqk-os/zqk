# Agent Feed Package

This package provides feed delivery mechanisms for ZQK's agent worker nodes. It enables nested swarm orchestration and dynamic feed delivery modes.

## Overview

- **Nested Swarm Orchestrator**: Orchestrates payload delivery across multiple `AgentFeed` instances based on configured delivery modes (`ModeLog`, `ModeClipboard`, `ModePaste`, `ModeNotify`, etc.).
- **Delivery Service**: Extensible interface (`DeliveryService`) for injecting custom logic when delivering payloads to agents.

## Architecture

- **Agent Feeds**: Feeds act as configurations that dictate how and if an agent receives payloads. Supports toggling functionality and configurable overrides for paths and contracts.

## Related

- [Project README](../../README.md) - Project overview
