# Rubric: L-PERFORMANCE

**Lens:** `L-PERFORMANCE` · **Density:** D-MED · **Axes:** REL, ROB

## Criteria

1. Hot paths identified with anchors.  
2. Unbounded allocations / unbounded fan-out.  
3. N+1 / accidental quadratic patterns.  
4. Caching correctness (stale/wrong cache worse than slow).  
5. Backpressure / load shedding if networked.

## Citations

| Work | Point |
|------|-------|
| Kleppmann — *DDIA* | Latency vs correctness; amplification |
| Brendan Gregg — systems performance methodology | Measure before tune |
| ISO/IEC 25010 | Performance efficiency |

## Pros / cons

| Pros | Cons |
|------|------|
| Emphasizes bounds and cache honesty | Microbenchmark theater — require anchors |
| Caching wrongness → ROB/REL | Without profiles, severity must stay moderate unless algorithmic proof |
