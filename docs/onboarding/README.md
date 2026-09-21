# Onboarding (ZQK Community)

**CLI:** default executable token. Live binary is `brand.executable_name` in `config/zqk-local.yaml` (wins) then `config/zqk.yaml`. Project YAML SSOT is `config/`. Kernel data stays under `.zqk/` (not a second config tree).

## Read this, in order

1. **[Community first-run](./COMMUNITY_FIRST_RUN.md)** — build, init, agent-onboard, MCP, `object list`, `whats-next`.
2. **[Quickstart / MCP](./QUICKSTART.md)** — same text as `./bin/zqk system start-here`.
3. **[First-run object tutorial](./FIRST_RUN_OBJECT_TUTORIAL.md)** — create / get / update a small `question`.
4. **[Edge / headless](./EDGE_HEADLESS_FIRST_RUN.md)** — appliances and `--headless` (optional).

## Not first-run

- **[AI Agent Onboarding](./AI_AGENT_ONBOARDING.md)** — directives and conventions for autonomous AI agents operating on ZQK Core.
- ZQK Core is the canonical upstream kernel; ZQK Studio consumes Core directly as an upstream dependency without bifurcation.
- Scheduler **is** shipped: `./bin/zqk scheduler start|stop|status`. CRUD works without it.

## Session start

```bash
make
./bin/zqk --version
./bin/zqk system agent-onboard --format json
./bin/zqk object list
./bin/zqk workflow whats-next --format json
```
