# Welcome to ZQK

Initialization complete. Choose your path:

---

### Option 1: Go Fast
*For developers who want immediate speed and zero learning curve.*

Drop token-budgeted code search directly into Cursor, Claude Code, Cline, or Aider:

```bash
# Search code with hard token bounds (default 500 tokens, sub-15ms)
zgrep "HandleRequest" --max-tokens 500 -f json

# Go AST structural search
zgrep --ast --kind struct MemoryStore
```

Add this to `.cursorrules`, `CLAUDE.md`, or `.clinerules`:
```markdown
- NEVER run raw recursive grep or find.
- ALWAYS use `zgrep <query> --max-tokens 500 -f json`.
- For Go syntax: `zgrep --ast --kind struct|func <name>`.
```

---

### Option 2: Walk Through
*For developers who want to understand core principles and see the Knowledge Kernel in action.*

Execute shovel-ready work or launch the interactive visual studio:

```bash
# Launch Visual Web Studio (roadmap, DAG, Gantt timeline)
./bin/zqk ui -w
# Open http://127.0.0.1:8080

# Execute shovel-ready work
./bin/zqk do

# Interactive onboarding walkthrough
./bin/zqk system start-here
```

Prompt your paired AI assistant:
> *"You are paired with the ZQK Knowledge Kernel. Let's do a paired walkthrough to capture my project intent, objectify it into kernel objects with `zqk new <kind>`, and launch our first autonomous execution."*

---

### Ambient & Agent Integration
- **Agent Seating & Directives**: Run `./bin/zqk system agent-onboard` to detect and prime editor directives.
- **Model Context Protocol (MCP)**: Run `./bin/zqk mcp proxy --tcp 127.0.0.1:7777` to expose MCP tools.
- **Task Discovery**: Run `./bin/zqk workflow whats-next` to inspect shovel-ready items.
- **Full Guide**: [docs/onboarding/COMMUNITY_FIRST_RUN.md](docs/onboarding/COMMUNITY_FIRST_RUN.md).
