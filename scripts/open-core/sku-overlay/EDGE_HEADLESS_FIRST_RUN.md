# Edge / headless first-run

**Audience:** SSH, appliances, or any host with no IDE agent.  
**IDE path:** [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md).

```bash
./bin/zcom system init --project-name my-project
./bin/zcom system agent-onboard --headless --format json
./bin/zcom object list
./bin/zcom workflow whats-next --format json
```

`--headless` skips IDE rule forests. Do not `export ZCOM_PROJECT_ROOT`. There is no `zqk-neuron` / organ binary on this SKU.
