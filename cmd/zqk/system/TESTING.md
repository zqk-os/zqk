# System package tests

## Timeout

The system package has many integration-style tests (temp dirs, storage, async check). The default `go test` timeout (10m) is often too low for a full run.

**To run the full suite without hitting the timeout:**

```bash
go test ./cmd/zqk/system/... -count=1 -timeout 15m
```

For a quicker run, use `-short` (some tests skip slow paths); you may still need a longer timeout:

```bash
go test ./cmd/zqk/system/... -short -count=1 -timeout 5m
```

## Check command tests

Tests that run the real check command (e.g. `TestCheckCommand_JSONLOutput`) use a short context timeout (~20s) so they don’t block when the async check doesn’t complete in a minimal env. Those tests may skip format checks if no output file is produced.
