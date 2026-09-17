# Local CI Contract & Remote Translation

**Last Verified:** 2026-08-31


## Overview
ZQK implements a Local CI capability (`zqk ci run`, `zqk ci checkout`, `zqk ci status`) designed to execute test suites against committed code without blocking the developer's live studio (working directory). This solves the race condition where mid-flight edits cause `go test` to fail or produce inconsistent results.

## The Local CI Contract
When Local CI runs, it adheres to the following contract:

1. **Isolation**: Tests do not run in the live studio root. Instead, a git worktree is checked out at the committed SHA in `.zqk/local-ci/workdir`.
2. **Honesty**: By default, `zqk ci run` will refuse to execute if the studio has tracked changes (`--allow-dirty` must be passed to bypass, or a pinned `--sha` must be provided).
3. **Traceability**: The checked-out state is recorded in `.zqk/local-ci/SOURCE_SHA` and `.zqk/local-ci/PROMOTED_AT`.
4. **Separation of Concerns (split-root)**: The `scan-tests` pipeline is run with `--source-root=.zqk/local-ci/workdir`. This ensures `go test` targets the isolated code, while health logs, `SCH-run` objects, and command diagnostics remain in the studio project root.

## Remote CI Translation
This contract makes translating to a Remote CI environment (e.g., GitHub Actions, GitLab CI) seamless:

1. **Checkout Bypass**: Remote CI already checks out a specific SHA. Therefore, the Remote CI pipeline does not need to run `zqk ci checkout`.
2. **Root Equivalence**: In Remote CI, the repository root *is* the workdir. Thus, `--source-root` defaults to the current directory, unifying the studio and the test target.
3. **Diagnostics**: Because health logs are generated at the execution root, Remote CI simply archives the `.zqk` folder at the root of the repository as its test artifact.

## Archival Forensics (Optional)
Local CI can optionally generate a `drop-<short>.tar` archive and a lightweight git repository in `.zqk/local-ci/archive-git`. This allows developers to diff historical test drops and trace exact file states at the time of failure, mimicking the forensic capabilities of a Remote CI artifact store without the network latency.
