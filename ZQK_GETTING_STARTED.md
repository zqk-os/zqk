# Welcome to ZQK

Initialization complete. Up and running in 2 commands:

### 1. Launch Visual Web Studio
```bash
./bin/zqk ui -w
```
Inspect workstreams, milestones, and the real-time Gantt timeline at http://127.0.0.1:8080.

### 2. Execute Shovel-Ready Work
```bash
./bin/zqk do
```
Or prompt your paired AI assistant:
> *"You are paired with the ZQK Knowledge Kernel. Run 'zqk do' to claim and implement work."*

---

### Ambient & Agent Integration (Optional)
- **Agent Seating & Directives**: Run `./bin/zqk system agent-onboard` to detect and prime editor directives.
- **Model Context Protocol (MCP)**: Run `./bin/zqk mcp install` to register local MCP endpoints.
- **Task Discovery**: Run `./bin/zqk workflow whats-next` to inspect shovel-ready items.
- **Knowledge Kernel**: Kernel data stays under `.zqk/`. Process YAML goes through the CLI, not a text editor.
- **Onboarding Guide**: Full documentation at [docs/onboarding/COMMUNITY_FIRST_RUN.md](docs/onboarding/COMMUNITY_FIRST_RUN.md).

---

> For the documentation quick-links index, see [docs/getting-started.md](docs/getting-started.md).
