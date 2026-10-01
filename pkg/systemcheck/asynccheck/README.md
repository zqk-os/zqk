# pkg/systemcheck/asynccheck

Package `asynccheck` provides asynchronous check execution, progress reporting, and validator event coordination for the ZQK microkernel.

## Architecture

This package decouples asynchronous check pipelines and coordinator telemetry from the presentation layer (`cmd/zqk/system`):

- **Progress Reporting (`progress.go`)**: Manages async operation lifecycle events, progress streaming, throughput reporting, and completion events.
- **Router Coordination (`router_coordination.go`)**: Provides unified event routing for async validator errors, warnings, semaphore exhaustion, and performance metrics across logging and audit streams.
- **Types (`types.go`)**: Declares severity constants, event type identifiers, and operation types.
