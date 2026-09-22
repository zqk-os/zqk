# First-Run Quality & Verification Gates

Run the independently verifiable launch checks from this checkout:

```bash
./bin/zqk workflow vds evaluate --format json --persist
```

The chunks in [`vds_chunks.yaml`](./vds_chunks.yaml) verify critical launch criteria: kernel isolation, documentation completeness, and exclusion of archived drafts.

They also provide the executable first-run acceptance surface: verifying that all advertised CLI commands exist and function, and ensuring clean, uncorrupted configuration.

For executable test-to-criteria lineage, run:

```bash
./bin/zqk test dashboard
```

To run release gates and payload verification before preparing distribution artifacts:

```bash
sh scripts/open-core/check-public-release-payload.sh
sh scripts/open-core/test-public-release-gates.sh
```
