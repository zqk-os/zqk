# Rubric: L-OBSERVABILITY

**Lens:** `L-OBSERVABILITY` · **Density:** D-MED · **Axes:** OBS, OPS, RCV

## Criteria

1. Can a stranger answer: is it healthy, what failed, what to do next?  
2. Structured logs vs printf soup; correlation IDs.  
3. Metrics/traces presence and cardinality sanity.  
4. Honesty of status endpoints (no false green).  
5. Runbooks / on-call affordances in-repo.

## Citations

| Work | Point |
|------|-------|
| Google SRE Book — Monitoring Distributed Systems | Four golden signals (adapt to monolith/CLI) |
| OpenTelemetry conceptual docs | Traces/metrics/logs correlation |
| ISO/IEC 25010 | Operability-related qualities |

## Pros / cons

| Pros | Cons |
|------|------|
| Catches false-healthy dashboards | Easy to demand enterprise APM for a library — calibrate |
| Status honesty is high leverage | Log volume ≠ observability |

## False green

If status claims OK while a subsystem timed out or was skipped, severity ≥ high under OBS (and CMP if advertised).
