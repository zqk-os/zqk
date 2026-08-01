# Multi-Agent Orchestration Architecture

## Overview
This document defines the architecture and schemas for multi-agent pipeline routing and task delegation.

## Core Schemas

### Agent Pipeline
Defines a sequence or graph of agent tasks.
- **ID**: API prefix (e.g., `APL-`)
- **Fields**:
  - `name`: Pipeline name.
  - `agents`: List of participating agents.
  - `routing_strategy`: Strategy for moving between agents.

### Agent Task
Defines a specific unit of work for an agent.
- **ID**: API prefix (e.g., `ATK-`)
- **Fields**:
  - `instruction`: Prompt or directive for the agent.
  - `status`: Lifecycle status (e.g., pending, in_progress, complete).
  - `assigned_agent`: The agent responsible for this task.

