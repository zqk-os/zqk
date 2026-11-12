# ADR-0051: Autonomous Provider Interceptor & Middleware Pipeline Chain v0.3.6

## Status
Accepted

## Context
As the ZQK Knowledge Kernel v0.3.6 ecosystem expands with autonomous tool providers, dynamic plugins, and external agent bridges, individual provider invocations require consistent cross-cutting concerns. These concerns include request logging, sensitive parameter redacting, authentication token injection, execution latency tracing, and telemetry aggregation. Embedding these concerns directly into each provider implementation results in code duplication, maintenance friction, and potential security oversights.

To establish clean package boundaries and modular extensibility, the ecosystem requires an Autonomous Provider Interceptor & Middleware Pipeline Chain. This architecture enables composing modular pre- and post-invocation interceptors in a predictable onion-style execution chain surrounding provider actions.

## Decision
1. **Interceptor Chain Architecture**:
   - Define `ProviderInterceptor` as `func(ctx context.Context, req any, next ProviderHandler) (any, error)`.
   - Implement `InterceptorPipeline` that chains multiple interceptors together preserving sequential evaluation order (first registered executes outer pre-processing and outer post-processing).
2. **Standard Community Middlewares**:
   - Provide built-in middlewares for:
     - `LoggingInterceptor`: logs invocation timestamps, duration, and completion status.
     - `RedactionInterceptor`: dynamically scrubs sensitive tokens or keys before downstream dispatch.
     - `TimeoutInterceptor`: strictly enforces maximum execution duration per provider invocation.
3. **Managed Concurrency & Zero Naked Goroutines**:
   - All async middleware workers must be spawned through `goroutinelabels` under `POL-CONCURRENCY`.

## Consequences
- **Positive**:
  - Consistent cross-cutting concern enforcement across all provider plugins.
  - Composable, isolated, and unit-testable middleware stages.
- **Negative / Trade-offs**:
  - Slight overhead per invocation for deep middleware chains; mitigated by lean function pointers and zero unnecessary allocations.
