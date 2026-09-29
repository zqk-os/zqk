# Testing standards (ZQK agents)

**Last updated:** 2026-09-24

## Default: kernel test_case objects

Do **not** run long or package-scale suites in the agent foreground.

```bash
./bin/zqk test discover
./bin/zqk test run TST-<id>
# or: ./bin/zqk test run --all
# long go test: ./scripts/test-runner.sh --package ./pkg/foo --log-file .zqk/logs/tests/foo.log
```

Always record **job id** + **log path** when printed, then **continue** other work. Revisit logs / `zqk scheduler history` later.

There is no `zqk scheduler scan-tests` command and no disk test-bundle JSON workflow.
Do not treat `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl` as SSOT.

## Foreground `go test` (narrow probes only)

- `-timeout` ≤ 60s
- Tight `-run` / small package you just edited
- Not `./...`, not full `./cmd/zqk/system`

## Canonical rules

- `.cursor/rules/tests-background-output-to-file.mdc`
- `.cursor/rules/tests-go-test-timeout.mdc`
- `.cursor/rules/zqk-test-bundles-commit.mdc`
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` §6
