# Data stream summary pilot (test-bundle health)

**Status:** Pilot implementation for REQ-DATASTREAM-001 (queryable summary per logical stream).

## Summary

- **Path alias:** `scheduler_test_bundles_health_jsonl` (constant `StreamSummaryTestBundleHealthAlias` in `pkg/scheduler/stream_summary_health.go`).
- **Logical file:** `.zqk/logs/scheduler/test-bundles/health.jsonl` (see `TestBundlesHealthFilePath`).
- **Written artifact:** `<projectRoot>/.zqk/stream_summary/test_bundle_health.json` — JSON row with `path_alias`, `kind`, `logical_path`, `byte_size`, `line_count`, `updated_at_rfc3339`.

## API

Call `scheduler.WriteTestBundleHealthStreamSummary(projectRoot)` from maintenance tooling, a future `zqk system` refresh command, or tests. Line count is bounded (scanner cap) so very large files do not block unbounded work.

## CLI (repo root)

**Refresh the summary file** (after test bundles append to `health.jsonl`):

```bash
go run ./scripts/write_test_bundle_stream_summary
go run ./scripts/write_test_bundle_stream_summary /path/to/project
```

**Dashboard / metrics payload:** `zqk system health-data --format json` (PRUNED) includes `test_bundle_health` when `.zqk/stream_summary/test_bundle_health.json` exists (same row as above). Default table output prints one line for the stream when present.

## Data cell

Full coordinator-backed membrane (REQ-DATACELL-001) is out of scope for this pilot; this row is the **summary** sidecar for one stream. Extend with additional files under `.zqk/stream_summary/` as new aliases are added (keep top-level entry count bounded per `FILESYSTEM_DATA_LAYOUT.md`).

**Spec plane boundary:** Kind definitions, field behavior, and validation remain on the **spec origin plane** ([SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md), including **Compatibility with data cells and streams**). Stream summaries and future cells operate on **instance and operational telemetry**, not on a second copy of object spec YAML.

## Report-driven feedback (planning and quality)

Use this path when closing the loop between **automated runs** and **process planning**:

1. **Raw stream:** `.zqk/logs/scheduler/test-bundles/health.jsonl` — one JSON line per test-bundle outcome (pass / fail / error), with fingerprint and optional suggested rerun hints (see `pkg/scheduler/job_log_writer.go`, `AppendTestBundleHealthEvent`).
2. **Queryable summary:** `.zqk/stream_summary/test_bundle_health.json` — produced by `scheduler.WriteTestBundleHealthStreamSummary` (or `go run ./scripts/write_test_bundle_stream_summary`). Agents and humans can read **line_count**, **byte_size**, and **updated_at** without scanning the full JSONL.
3. **Planning:** Combine with **`zqk scheduler test-failures`** / convergence tooling and **priority plan / backlog** work (e.g. milestone **MIL-007**, plan **PRI-EXAMPLE**): failing bundles feed backlog and verification; improving bundles reduces noise in `health.jsonl` and summary metrics.

**Audit / stream stewardship:** High-volume audit and aggregation paths remain documented in **`docs/architecture/README.md`** and **`docs/architecture/README.md`**. This pilot does not change **`audit_event` list hot-path** code; it adds a measurable, summarized view for **test-bundle health** only.

## Glossary

- Data stream summary: `GLS-EXAMPLE`
- Data cell: `GLS-EXAMPLE`
