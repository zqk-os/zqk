# APISpec Builder Pattern Architecture

**Last Verified:** 2026-08-31


**Status**: Active
**Component**: `pkg/specbuilder/`
**Related**: `BLI-1783761336286408000-ca1625db` (PRI-SYM-005), adopters in `pkg/llm` (OpenAI + Gemini) and `pkg/hive/media`

## Overview
The APISpec Builder pattern provides a standardized, thread-safe approach to constructing API clients and configurations within ZQK. It ensures that all outbound API integrations consistently implement telemetry, resiliency (retries, timeouts, circuit breaking), and contextual awareness.

## Problem Statement
Without a unified builder pattern, API clients in different parts of the system may:
- Lack standardized telemetry (metrics, tracing).
- Implement ad-hoc retry logic or lack resiliency completely.
- Be initialized without thread-safety considerations.
- Duplicate context setup and security token management.

## Solution: The APISpec Builder Pattern
The builder pattern centralizes API client construction. Instead of directly instantiating HTTP clients, developers use the APISpec builder which handles injection of middleware, telemetry hooks, and standard configuration.

### Core Components

1. **`APISpec`**: A declarative specification of the API's requirements (e.g., base URL, auth requirements, default timeouts, retry policies).
2. **`SpecBuilder`**: The builder interface that constructs the thread-safe API client from the `APISpec`.
3. **Resiliency Middleware**: Interceptors injected by the builder to handle rate limits, backoffs, and circuit breakers.
4. **Telemetry Middleware**: Interceptors for standard logging, tracing, and metrics (latency, error rates).

### Implementation Details in `pkg/specbuilder/`

```go
package specbuilder

// APISpec defines the configuration and requirements for an API client.
type APISpec struct {
    Name           string
    BaseURL        string
    Timeout        time.Duration
    RetryPolicy    RetryPolicy
    AuthStrategies []AuthStrategy
}

// Builder constructs API clients safely.
type Builder interface {
    WithSpec(spec APISpec) Builder
    WithTelemetry(enabled bool) Builder
    WithResiliency(enabled bool) Builder
    Build() (APIClient, error)
}
```

### Key Capabilities
- **Thread-Safety**: The builder ensures that the constructed `APIClient` instance is safe for concurrent use across multiple goroutines, typically by relying on standard thread-safe underlying HTTP transports and immutable configurations.
- **Telemetry**: Automatically attaches standard metrics (observability) for request durations and error rates, avoiding manual logging calls per the Logging Compliance policy (POL-CODE-007).
- **Resiliency**: Injects configurable retry logic (e.g., exponential backoff) and circuit breakers to prevent cascading failures.

## Alignment with Existing Patterns
- Follows the **Single Context Principle**: The builder requires a unified context that derives downstream contexts for individual requests.
- Adheres to **Context-Driven Bootstrap**: Available telemetry endpoints and resiliency defaults are discovered from the application context.

## Actions Required
1. Implement `APISpec` and `Builder` interfaces in `pkg/specbuilder/`.
2. Implement standard telemetry and resiliency interceptors.
3. Add unit tests for thread-safety and retry behaviors.
4. Refactor existing API integrations to use the `APISpec Builder` instead of direct HTTP client instantiation.
