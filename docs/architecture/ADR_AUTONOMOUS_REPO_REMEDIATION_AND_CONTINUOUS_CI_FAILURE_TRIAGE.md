# ADR: Autonomous Repo Remediation & Continuous CI Failure Interceptor

## Context & Motivation
In autonomous agent swarms, continuous pull request hygiene and proactive CI failure remediation are vital to prevent branch stalling and stale PR accumulation. When background test runs or CI workflows encounter failures, flakes, or regression gates, the system must not wait for manual operator intervention.

This architecture introduces continuous PR health observation and automatic remediation enqueueing directly into the ZQK kernel lifecycle:
1. Continuous background triage of active pull requests and integration runs.
2. Structured failure classification into transient environment flakes versus deterministic code defects.
3. Automatic minting of kernel `technical_debt` or `backlog_item` objects linked directly to failing components.
4. Autonomous dispatch of worker subagents to resolve failures prior to human operator escalation.

## Decisions
- **Proactive Interception**: PR failure triage hooks run inside the kernel ambient daemon (`com.zqk.overseer`), observing GitHub PR status check rollups.
- **Three-Fold Failure Classification**:
  - *Static Floor Invariant*: Lint, format, license, or schema violations are dispatched with pre-configured auto-formatters.
  - *Operational Dynamic Proof*: Test flakes or timeouts trigger automatic re-runs with bounded retry budgets (max 2 retries).
  - *Negative Adversarial Boundary*: Deterministic code errors or panics generate structured `technical_debt` objects with attached failure logs.
- **Zero Orphaned PR Policy**: Stale integration branches with merged PRs are cleaned up automatically; branches with failing PRs receive automated fix proposals.

## Consequences & Verification
- Ensures high developer velocity with zero PR drift.
- Verified via unit test suites covering system improvement reports, metric collection, and adaptive remediation dispatch (`cmd/zqk/system/improvement_report_test.go`).
