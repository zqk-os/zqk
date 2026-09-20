# First-run quality gate

Run the independently verifiable launch checks from this checkout:

```bash
./bin/zqk workflow vds evaluate --format json --persist
```

The chunks in [`vds_chunks.yaml`](./vds_chunks.yaml) verify the launch criteria
for `BLI-1789630408733990000-0a2023fb`: kernel isolation, shipped documentation
registration, and exclusion of archived documentation.

They also provide the executable first-run acceptance surface for umbrella
`BLI-1789681478369698000-263b989c`: the commands advertised to a new user must
exist on the built SKU, and local pressure-test branding must not leak into the
committed documentation.

For executable test-to-criteria lineage, run:

```bash
./bin/zqk test dashboard
```

Before preparing any public artifact, run the dest-owned payload gate:

```bash
sh scripts/open-core/check-public-release-payload.sh
```

The broader local/CI gate also proves the community build and help surfaces:

```bash
sh scripts/open-core/test-public-release-gates.sh
```

These release checks implement
`BLI-1789717939876745000-1a629968`. A passing payload gate is not permission
to push; publication still requires explicit human acknowledgment.
