# Pipeline Package

This package provides a robust builder and runtime for the standardized data pipeline lifecycle, enabling structured, multi-stage processing of data and tasks within the Knowledge Kernel.

## Overview

- **Pipeline Builder**: Construct data processing pipelines sequentially with standard lifecycle stages (`StageIngest`, `StageNormalize`, `StageDecide`, `StageCommit`, `StageFinalize`).
- **DAG Execution**: Define and execute Directed Acyclic Graphs (`DAG`) of tasks with dependencies, state sharing, and parallel execution (`dag_executor.go`).
- **Envelopes**: Immutable wrappers (`Envelope`) for processing payloads with trace IDs, idempotency keys, and partition keys.
- **Resilience**: Decorators for stages like retry logic (`AddRetryStage`) and token-based rate limiting (`AddTPMStage`) via circuit breakers.
- **Metrics & Observability**: Integrated recording of stage durations, latencies, and completion statuses via pluggable `MetricsSink`.

## Usage

```go
import "github.com/lanceman/zqk/pkg/pipeline"

// Build a sequential processing pipeline
bldr := pipeline.NewBuilder("document_ingestion", logger).
	WithMetrics(metricsSink).
	AddStage(pipeline.StageIngest, myIngestFn).
	AddStage(pipeline.StageNormalize, myNormalizeFn).
	AddRetryStage(pipeline.StageCommit, 3, time.Second, nil, myCommitFn)

pipe := bldr.Build()

// Execute the pipeline
pCtx := &pipeline.Context{
	TraceID: "job-123",
	BaseCtx: context.Background(),
}
result, err := pipe.Run(pCtx, myPayload)
```

## Architecture

- **Data Pipeline Lifecycle**: Adheres strictly to the architectural spec (INGEST → NORMALIZE → DECIDE → COMMIT → FINALIZE), ensuring separation of data mutation and persistence.
- **DAG Executor**: A multi-threaded task runner that dynamically orchestrates parallel jobs, tracking dependencies, output hashes, and task sessions.
- **Context Budgets & Routing**: Advanced traffic shaping, pipeline routing, and resource budgeting to protect underlying systems during hyper-scale workloads.

## Related

- [Project README](../../README.md) - Project overview
- [Architecture Docs](../../docs/architecture/) - System architecture
