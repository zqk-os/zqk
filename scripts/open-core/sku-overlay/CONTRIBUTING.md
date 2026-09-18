# Contributing (ZQK Community pressure-test)

This checkout is the **community SKU**. The binary is **`zcom`**. Studio `zqk` is a different product.

## First run

```bash
make
./bin/zcom --version
./bin/zcom system agent-onboard --format json
./bin/zcom workflow whats-next --format json
```

See [docs/onboarding/COMMUNITY_FIRST_RUN.md](docs/onboarding/COMMUNITY_FIRST_RUN.md).

## Rules that matter here

- Process data under `.zqk/process/` goes through `./bin/zcom` (object create/update). Do not edit hash-named YAML by hand.
- Do not `export ZCOM_PROJECT_ROOT` or `ZQK_PROJECT_ROOT` in your shell profile.
- There is no scheduler, `zcom-admin`, or public brew/GitHub release on this SKU.
- `make` only builds `./bin/zcom`. There is no `make zqk` / `promote-stable`.

## Checks

```bash
./bin/zcom system check
./bin/zcom object list
```

License: Apache 2.0 (`LICENSE` + `NOTICE`).
