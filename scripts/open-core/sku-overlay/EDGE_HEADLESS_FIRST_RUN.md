# Edge / headless first-run

**Audience:** SSH, appliances, or any host with no IDE agent.  
**IDE path:** [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md).

```bash
./bin/zqk system init --project-name my-project
./bin/zqk system agent-onboard --headless --format json
./bin/zqk object list
./bin/zqk workflow whats-next --format json
```

`--headless` skips IDE rule forests. Do not `export ZQK_PROJECT_ROOT`. There is no organ binary on this SKU.
