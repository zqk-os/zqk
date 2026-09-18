# Onboarding (ZQK Community)

**CLI:** `./bin/zcom` until public launch (then `zqk`). Kernel data stays under `.zqk/`.

## Read this, in order

1. **[Community first-run](./COMMUNITY_FIRST_RUN.md)** — build, init, agent-onboard, MCP, `object list`, `whats-next`.
2. **[Quickstart / MCP](./QUICKSTART.md)** — same text as `./bin/zcom system start-here`.
3. **[First-run object tutorial](./FIRST_RUN_OBJECT_TUTORIAL.md)** — create / get / update a small `question`.
4. **[Edge / headless](./EDGE_HEADLESS_FIRST_RUN.md)** — appliances and `--headless` (optional).

## Not first-run

- **[AI Agent Onboarding](./AI_AGENT_ONBOARDING.md)** is a studio-dense process pack. Do not treat it as the community golden path.
- There is no `make alpha-help`, `zqk-admin`, `zcom-admin`, or public brew/GitHub release on this SKU.
- Scheduler **is** shipped: `./bin/zcom scheduler start|stop|status`. CRUD works without it.

## Session start

```bash
make
./bin/zcom --version
./bin/zcom system agent-onboard --format json
./bin/zcom object list
./bin/zcom workflow whats-next --format json
```
