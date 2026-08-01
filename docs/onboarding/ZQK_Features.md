# ZQK Semantic Features Inventory

ZQK provides a robust, self-updating semantic organization of features designed to scale from individual hobbyists to enterprise organizations. 

## Core Capabilities

*   **Graph-based Backend:** Pluggable graph database support (MemGraph, Neo4j, RDF stores) that models system lifecycles.
*   **Distributed Knowledge Kernel:** Git-based distribution with PKI authority verification and secure keystore entries.
*   **CLI Ontology & Tooling:** Spec-driven CLI interactions (`zqk object create`, `zqk object update`) and alias registry (e.g., `get`, `task`, `plan`) to enable AI and human collaboration.
*   **MCP Server Integration:** Built-in Model Context Protocol server (`bin/zqk-mcp`) allowing agents to interface directly with the kernel.
*   **Metrics & Telemetry:** An integrated scheduler (`zqk scheduler start`) for executing background tasks, metrics collection, and continuous testing without blocking foreground agents.
*   **Spec-driven Object System:** More than 50 defined system objects (`goal`, `backlog_item`, `priority_plan`, `workstream`, `question`, etc.) providing goal traceability across every system artifact.
