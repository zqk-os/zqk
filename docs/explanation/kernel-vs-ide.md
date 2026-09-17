# Explanation: Kernel Primacy vs IDE Projections

Understanding the boundary between the ZQK Knowledge Kernel and vendor IDE environments (Cursor, VS Code, Windsurf).

---

## 1. The Kernel is the Single Source of Truth
In standard AI-assisted workflows, state is often scattered across editor caches, local markdown scratchpads, chat histories, and ephemeral vendor memory files (`.cursorrules`, `.gemini/`, prompt buffers). When multiple developers or agents work together, context quickly fractures.

In ZQK:
- **The Knowledge Kernel owns all process state:** Goals, requirements, criteria, backlog items, and architectural decisions exist as typed, validated objects stored in Git Content-Addressable Storage (CAS).
- **IDEs are read/write projections:** Editor rules (such as `.cursorrules` or `.agents/AGENTS.md`) are projections generated or primed from the kernel.

---

## 2. Headless-Safe Operations
ZQK is designed to run headlessly in CI/CD, local terminal swarms, or background daemons without requiring a graphical IDE. The IDE connects to the kernel via the Model Context Protocol (MCP), ensuring that whether an agent runs inside an IDE window or as a daemon seat-worker, both share identical capabilities and constraints.
