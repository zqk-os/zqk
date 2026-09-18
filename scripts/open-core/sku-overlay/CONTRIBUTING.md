# Contributing (ZQK Community)

This checkout is the **community SKU**. Command examples use the default executable token. The live binary is `brand.executable_name`.

## First run

```bash
make
./bin/zqk --version
./bin/zqk system agent-onboard --format json
./bin/zqk workflow whats-next --format json
```

See [docs/onboarding/COMMUNITY_FIRST_RUN.md](docs/onboarding/COMMUNITY_FIRST_RUN.md).

## Rules that matter here

- Process data under `.zqk/process/` goes through `./bin/zqk` (object create/update). Do not edit hash-named YAML by hand.
- Do not `export ZQK_PROJECT_ROOT` in your shell profile.
- There is no `zqk-admin` or public brew/GitHub release on this SKU. Scheduler is `./bin/zqk scheduler start|stop|status`.
- `make` builds `./bin/<brand.executable_name>`. There is no `make promote-stable`.

## Checks

```bash
./bin/zqk system check
./bin/zqk object list
```

License: Apache 2.0 (`LICENSE` + `NOTICE`).
