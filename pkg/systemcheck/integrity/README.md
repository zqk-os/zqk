# pkg/systemcheck/integrity

Package `integrity` provides core kernel verification, object compliance tracking, and reference integrity capabilities for the ZQK microkernel.

## Architecture

This package decouples internal integrity analysis and compliance rollups from the presentation layer (`cmd/zqk/system`):

- **Object Compliance (`object_compliance.go`)**: Evaluates the instance-validation health slice of kernel integrity from system-check snapshots (`system-check.json`), tracking issue delta and classification trend (`improving`, `worsening`, `flat`) in append-only JSONL history.
- **Reference Integrity (`dangling_refs.go`)**: Scans critical schema kinds for unresolvable outbound references (`*_ref`, `*_refs`, `dependencies`) and generates precise unlink update maps.
- **Membrane & Custom Rules Gates (`gates.go`)**: Validates that all mutation pipelines enforce kernel CAS invariants (`kernelcas.Run*`) and that legacy Go custom validator bodies remain completely removed in favor of compose overlays.
- **Kernel Integrity Diagnostics (`report.go`)**: Builds a unified `KernelIntegrityReportPayload` covering KMP pipeline coverage, composition registry size, dangling reference counts, and dual-layer health signals.
