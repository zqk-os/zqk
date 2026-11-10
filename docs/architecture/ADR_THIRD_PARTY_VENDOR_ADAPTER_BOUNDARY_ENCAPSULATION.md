# ADR: Third-Party Vendor Product Adapter Boundary Encapsulation

## Context & Motivation
Directly referencing 3rd-party vendor filesystem paths, CLI binaries, and configurations (e.g. `~/.gemini/antigravity-cli/bin/agentapi`, `~/.cursor/mcp.json`) in primary business logic, orchestration modules, or core CLI commands violates architectural boundaries and makes the system brittle to external changes.

Following user direction ("if we name a vendor-specific product in our code, it needs to be in an adapter - no hardcoded 3rd-party hacks are allowed"):
1. All vendor-specific path layouts, discovery rules, and binary lookups must reside inside dedicated adapter packages under `pkg/adapters/` (e.g. `pkg/adapters/antigravity`, `pkg/adapters/cursor`).
2. Core orchestrators (`pkg/primaryorch`, `pkg/swarminit`) and MCP sovereignty managers (`pkg/nodesovereignty`) must consume clean, decoupled interfaces provided by these adapters.
3. No raw third-party product names, paths, or directory strings may be hardcoded into core execution engines.

## Decisions
- **Antigravity Adapter (`pkg/adapters/antigravity`)**:
  - Encapsulates `DefaultAgentAPIBinarySubpath` and `DefaultBrainSubpath`.
  - Exposes `LookPathAgentAPI()` to locate vendor binaries through PATH with safe fallback to vendor adapter layout.
  - Decoupled from `pkg/primaryorch` and `pkg/swarminit`.
- **Cursor Adapter (`pkg/adapters/cursor`)**:
  - Implements `IDEPinAdapter` for host-global MCP configuration checks in `pkg/nodesovereignty`.
- **Testing & Verification**:
  - Tested via Static Floor Invariant (clean empty-path handling), Operational Dynamic Proof (binary discovery via canonical vendor subpath), and Negative Adversarial Boundary (rejection of directory matches).

## Consequences
- Eliminates third-party vendor coupling from core kernel logic.
- Enhances portability across multiple agent execution harnesses and IDEs.
