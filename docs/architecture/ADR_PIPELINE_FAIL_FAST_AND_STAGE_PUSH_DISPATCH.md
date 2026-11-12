# ADR: Pipeline Fail-Fast Early Bail and Stage Push Dispatch

## Status
Accepted

## Context
Continuous integration pipelines and automated convergence loops previously relied on fixed synthetic interval polling or uncoordinated stage runs. When an upstream blocking test suite failed, downstream jobs continued to execute, consuming cloud runner minutes and masking the root failure. Furthermore, agents were prone to idle interval polling rather than subscribing to reactive event-driven push notifications.

## Decision
1. **Pipeline Stage Push Dispatch**: Implement `PipelineNotificationDispatcher` in `pkg/community/pipeline_notify.go` to emit structured HTTP POST notifications at each pipeline stage boundary.
2. **HMAC-SHA256 Authenticated Delivery**: Each event payload is signed with HMAC-SHA256 across payload bytes and includes headers `X-ZQK-Signature-256`, `X-ZQK-Timestamp`, `X-ZQK-Stage`, `X-ZQK-Status`, and `X-ZQK-ExitCode`.
3. **Fail-Fast Early Bail Governor**: Implement `PipelineFailFastGovernor` to evaluate stage completion outcomes. Any blocking stage returning a non-zero exit code immediately trips the governor, aborting downstream jobs fail-closed and halting wasted execution.
4. **Policy Codification**: Codify `POL-CI-FAIL-FAST-001` as a persistent kernel policy governing pipeline execution semantics.

## Consequences
- Immediate reduction in CI compute consumption by terminating failed builds early.
- Elimination of synthetic polling loops through authenticated push notifications.
- Strong cryptographic integrity for pipeline event streams.
