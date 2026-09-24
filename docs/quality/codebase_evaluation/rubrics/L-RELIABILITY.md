# Rubric: L-RELIABILITY

**Lens:** `L-RELIABILITY` · **Density:** D-MED · **Axes:** REL, ROB, RCV

## Criteria

1. Error handling completeness on I/O and RPC boundaries.  
2. Idempotency / retry / timeout behavior.  
3. Data integrity under crash (write ordering, fsync, transactions).  
4. Degradation vs silent corruption.  
5. Fail-closed vs fail-open at safety boundaries.

## Citations

| Work | Point |
|------|-------|
| Google SRE Book | Embracing risk; error budgets (conceptual) |
| Kleppmann — *DDIA* | Reliability; disk/fsync; replication truths |
| ISO/IEC 25010 | Reliability characteristic |

## Pros / cons

| Pros | Cons |
|------|------|
| Distinguishes “works on happy path” from production | Requires domain reading; tests alone insufficient |
| Crash consistency is often missing in reviews | Over-indexing on distributed systems lore for single-process tools |

## Recommendation bias

Prefer evidence of **corruption refusal** and **explicit degradation** over “best effort continue.”
