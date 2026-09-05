# Specbuilder Generation & Build Performance (L:F-PERF-02 / CRIT-CEF-R8L-PERF-02)

**Last Verified:** 2026-08-31


## Performance Metrics & Analysis
The specbuilder generator produces type-safe builders (`pkg/specbuilder/bldr_*`) from spec definitions.

- **Generated Output**: ~122k LOC across 72 kernel object types.
- **Build Impact**:
  - Incremental Go builds cache AST objects in `~/.cache/go-build/`.
  - Typical `go build ./...` compilation latency: ~1.8s on modern hardware.
  - Spec generation runs only on schema mutation (`make build-all`), decoupling runtime compilation from code generation.
