# API Integration Pattern (Spec-Builder)

**Last Verified:** 2026-08-31


## Overview
As ZQK scales, we will integrate with numerous third-party APIs (e.g., Fal.ai, Mubert, OpenAI). The previous ad-hoc implementations lacked observability and structure.

To maintain our rock-solid reliability and traceability, all third-party API wrappers must use the **Spec-Driven Builder Pattern** along with our **Structured Logging & Metrics Framework**.

## The Pattern

1. **Declarative Specification**: 
   Define a structured Request payload (Spec) that can be generated from YAML or code.
   
2. **Type-Safe Builders**:
   Expose a Builder interface that accepts configuration (API keys, base URLs) and returns a runner or client.
   
3. **Execution with Observability**:
   Instead of using standard `http.Client` operations silently, every major step must be instrumented:
   - **Metrics**: Count success vs failure, latency histograms.
   - **Structured Logs**: Use `logging.FluentEvent(logger)` to trace HTTP calls. Ensure all fields (like `prompt`, `url`, `duration`) are stringified and logged.
   - **Resilience**: Implement structured retries with context awareness.

## Example Migration (Media Generators)
- Replace raw `http.NewRequest` sequences in `mubert.go` and `fal.go` with specialized internal builders.
- Apply `logging.NewEventLogger` within the client contexts to ensure every task logs its start, completion, or failure state.
- Expose their operations as formal `pipeline.Pipeline` or `generator.Generator` objects as defined in `pkg/specbuilder/`.
