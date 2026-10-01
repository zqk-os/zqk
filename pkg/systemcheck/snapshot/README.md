# pkg/systemcheck/snapshot

Package `snapshot` provides snapshot persistence, dynamic file expansion, and snapshot verification utilities decoupled from presentation commands.

## Components

- **Diff (`diff.go`)**: Compares previous and current snapshot state maps, producing categorized delta summaries.
- **Expand (`expand.go`)**: Expands raw snapshot summaries into comprehensive diagnostics, including disk usage and subsystem reports.
- **Save (`save.go`)**: Atomically writes updated system-check snapshots to CAS and filesystem targets.
- **Types (`types.go`)**: Type definitions for snapshot objects, summaries, and options.
- **Verify (`verify.go`)**: Validates snapshot structure and file integrity against schema definitions.
