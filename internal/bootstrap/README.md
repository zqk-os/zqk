# Bootstrap package

Embedded bootstrap archive for `zqk system init`.

- **Build:** The build process creates `archive/bootstrap.tar.gz` and `archive/manifest.txt` via `make bootstrap-archive` (or as a dependency of `make build-zqk-staging`). Script: `scripts/build-bootstrap-archive.sh`.
- **Source:** Archive contents are built from:
  - `.zqk/specs` → extracted to project `.zqk/specs`
  - `.zqk/cli/specs` → extracted to project `.zqk/cli/specs`
  - `scripts/scheduler_jobs/` (retention_tolerance_catchall.yaml, audit_event_aggregation_default.yaml) → extracted to project `scripts/scheduler_jobs/` so `init --with-maintenance-jobs` works in greenfield projects without the main repo.
- **Init:** When running init, the binary extracts the embedded archive via `ExtractTo`. If the archive is missing (e.g. dev build), `ExtractFiles` falls back to copying from the repo source (`extract_from_source.go`). CLI entry is a thin wrapper in `cmd/zqk/system`.
- **Traceability:** `ManifestPaths()` returns the list of bundled paths.

After changing `.zqk/specs`, `.zqk/cli/specs`, or the bundled scheduler job templates, run `make bootstrap-archive` (or any build target that depends on it) so the embedded archive is updated.
