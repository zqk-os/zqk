# First-run quality gate

Run the independently verifiable launch checks from this checkout:

```bash
./bin/zqk workflow vds evaluate --format json --persist
```

The chunks in [`vds_chunks.yaml`](./vds_chunks.yaml) verify the launch criteria
for `BLI-1789630408733990000-0a2023fb`: kernel isolation, shipped documentation
registration, and exclusion of archived documentation.

For executable test-to-criteria lineage, run:

```bash
./bin/zqk test dashboard
```
