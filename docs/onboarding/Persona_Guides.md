# ZQK Persona Guides

How you leverage ZQK depends heavily on your scale and goals. 

## Hobbyist Product Builders
*   **Focus:** Lightweight usage and rapid prototyping.
*   **Strategy:** Hobbyists should rely on simple tracking objects, primarily `question` and `backlog_item`.
*   **Essential Commands:** `zqk system init` and the "First-Run Object Tutorial".
*   **Value Proposition:** Frictionless context persistence without the overhead of complex architectural setup. It tracks *what* you're doing without forcing you to write extensive specs.

## Advanced Users (Enterprise & Platform)
*   **Focus:** Robust governance, auditability, and swarm orchestration.
*   **Strategy:** Advanced users must leverage `priority_plan`, `workstream`, `convergence_session`, and structured metrics. 
*   **Essential Mechanics:** Scaling via `zqk scheduler`, integrating with the built-in `MCP` server for multi-agent delegation, and strictly utilizing PKI keystores for secure enterprise automation.
*   **Value Proposition:** 100% cryptographic traceability, ensuring that no autonomous agent mutation occurs without policy approval.
