## Summary

- What changed and why (not a file list).

## Test plan

- [ ] Targeted `go test ./<pkg> -timeout 60s` for the change surface (`go test ./...` only when blast radius is wide)
- [ ] `./bin/zqk system check` clean for the change surface
- [ ] `sh scripts/open-core/test-public-release-gates.sh` (same suite as Community CI)
- [ ] Process data under `.zqk/process/` went through `./bin/zqk` (no hand-edited hash YAML)

## Publication

Open the PR from a `feature/` or `integration/` branch. Do not push directly to `origin/main`. Wait for Community CI to pass.
