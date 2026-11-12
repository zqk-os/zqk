# ADR-0058: Autonomous Provider Distributed Tracing & OpenTelemetry Bridge v0.3.6

## Context
As the autonomous provider ecosystem expands across concurrent swarm workers, agents, and external micro-services, understanding request flows and execution cascades requires end-to-end distributed tracing. Without consistent span propagation, diagnosing cross-provider latency bottlenecks, transient errors, and event mesh dispatch cascades is error-prone. Standardizing on W3C Trace Context (`traceparent`) format enables seamless interoperability with OpenTelemetry collectors and downstream APM platforms.

## Decision
1. **Trace & Span Data Model**:
   - `TraceID`: 16-byte cryptographically secure random identifier representing the entire transaction chain.
   - `SpanID`: 8-byte cryptographically secure random identifier representing an individual execution segment.
   - `TraceContext`: Wraps TraceID, SpanID, and trace flags with standard W3C `traceparent` formatting (`00-{trace_id}-{span_id}-{flags}`).
2. **Span Lifecycle & Provider Invariants**:
   - Each provider invocation creates a span via `StartSpan(name, parentContext)`.
   - Spans record start/finish timestamps, tags/attributes, and structured event logs.
   - Calling `End()` finalizes the span duration, marks completion, and publishes to the telemetry buffer.
3. **Adversarial Error Boundaries**:
   - Malformed `traceparent` headers return `ErrInvalidTraceContext` without panics.
   - Ending an unstarted span returns `ErrSpanNotStarted`.
   - All concurrency uses thread-safe mutex guards to prevent race conditions.

## Consequences
- Full distributed visibility across asynchronous provider dispatch pipelines.
- Standardized integration with OpenTelemetry exporters.
- Robust boundary verification and AST compliance under `zqk-vet`.
