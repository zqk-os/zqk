# pkg/systemcheck/congruence

Package `congruence` provides operational congruence validation, object-disk count disparity auditing, and storage volume metrics collection for the ZQK microkernel.

## Architecture

This package decouples operational congruence analysis from the presentation layer (`cmd/zqk/system`):

- **Data Gathering (`gather.go`)**: Collects cache status, process integrity, object counts by kind, and filesystem snapshots.
- **Reporting (`report.go`)**: Synthesizes and writes human-readable and machine-parseable congruence reports and dashboard snapshots.
- **Metrics Recording (`metrics.go`)**: Records object and stream volume telemetry series.
- **Runner (`runner.go`)**: Coordinates execution pipelines and alerts for background operations.
- **Types (`types.go`)**: Defines report schemas, cache statuses, and integrity models.
