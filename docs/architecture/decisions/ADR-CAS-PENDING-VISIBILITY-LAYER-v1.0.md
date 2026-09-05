# ADR: CAS pending visibility layer for cross-process read-your-writes

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-07-31  
**Status:** Accepted  
**Decision Date:** 2026-07-31  

**Related:** `[REDACTED-ID]` · `REQ-CAS-PENDING-001` · `[REDACTED-ID]` · fail-closed get TRACK `[REDACTED-ID]` · CLI durability flush TRACK `[REDACTED-ID]` · create-repair anti-ghost TRACK `[REDACTED-ID]` · [CAS_LIST_GET_CONSISTENCY.md](../CAS_LIST_GET_CONSISTENCY.md)

## Context

CAS mutations publish two durable artifacts: a **content file** (`docs/process/<kind>/<hash>.yaml`) and a **locator index** (`id → hash`). Writers update an **in-memory** mapping immediately and enqueue **async** index persistence. Cross-process `object get` trusts the on-disk index and **fail-closes on miss** (no O(n) scan) for hot-path performance.

That combination creates **ghost / forked-id** failures when:

1. Process A reports create/update **success** (id handed out; same-process reads work).
2. Process B (new CLI, agent, MCP peer) runs `get` before the durable index catches up → **not found**.
3. Agents recreate under a **new id**, forking history.

Operational amplifiers (SIGKILL of short-lived `zqk`, incomplete flush, kill mid-shutdown) make the lag visible, but the root issue is not “writes fail.” It is that **GET behaves as if locator publication were instantaneous** while publication to the shared disk index is not.

Full two-phase “index fsync before every success” is correct but heavy and easy to get wrong across update/orphan paths. Ad-hoc `rebuild-cas-index` / get-scan recovery are **symptoms**, not the product contract.

## Decision

### 1. Layered GET (normative)

```text
GET(id):
  1. pending / creation visibility cache   // tiny, hot, published at success
  2. in-memory CAS index                   // same process
  3. on-disk CAS index
  4. not found                             // fail-closed; no full-dir scan on hot path
```

### 2. Success ⇒ pending entry visible before return

When create/update is allowed to report success (including write-behind paths once the WAL intent is durable and the id/hash are known), the writer **must** populate a **cross-process-visible pending map** (`id → hash` [+ kind/bucket]) **before** returning success.

Same-process `setIndexMappingInMemory` remains necessary but **insufficient** for multi-CLI / agent workflows.

### 3. Pending layer shape

- Small, bounded (recent mutations only); not a second full index.
- Durable enough for other processes to read (e.g. file/mmap under `.zqk/`, or a long-lived steward all reads use).
- Background promotion into the durable CAS index (existing write queue); evict pending once disk index confirms the same mapping (or TTL + confirmed flush).

### 4. Shutdown contract

On ordered shutdown (scheduler stop / SIGTERM drain path):

1. **Reject new writes.**
2. **Dump pending → durable index** (+ fsync as required).
3. Drain write-behind / WAL apply.
4. Exit.

Polite stop must not leave success-published ids invisible to the next process.

### 5. Force kill / crash

SIGKILL and equivalent remain an accepted residual risk (possible loss or incomplete publication). On restart, reconcile **WAL** (and any pending file that reached disk). Do not treat force-kill as a design that must be zero-loss without WAL.

### 6. Non-goals

- Restoring O(n) get-on-miss directory scans as the primary correctness path.
- Making ad-hoc `recover-cas` / blanket `fix-hash-mismatches` the daily consistency mechanism.
- Agent policy of recreate-on-first-not-found.

## Consequences

**Positive**

- Read-your-writes after success without waiting for full index flush semantics on every GET.
- Keeps fail-closed hot path (no scan).
- Separates **visibility** (pending) from **durable index merge** (async).
- Shutdown drain has a clear job: promote pending, then exit.
- Crash story stays WAL-centric and well understood.

**Negative / cost**

- New shared pending artifact and GET check order (must stay small and correct under concurrent writers).
- Shutdown must reject writes and dump pending (discipline in scheduler/CLI stop paths).
- Force kill can still drop unpublished pending; restart must detect incomplete transactions.

**Alternatives considered**

| Alternative | Why not primary |
|-------------|-----------------|
| Index fsync before every success | Correct but heavier; still needs crash story; does not simplify update/orphan ordering |
| Get scan on index miss | Perf regression; already rejected for hot path (`[REDACTED-ID]`) |
| Rebuild scripts as ops SOP | Brittle; treats ghosts as normal |
| In-process pending only | Does not fix cross-process agent/CLI get |

## Implementation guidance

See [CAS_LIST_GET_CONSISTENCY.md](../CAS_LIST_GET_CONSISTENCY.md) § Pending visibility layer. Track delivery on the dedicated priority plan and `REQ-CAS-PENDING-001`.

**Locator merge rules (normative for index load):** pending may **fill gaps** only (id missing from durable index). A pending hash must **never replace** a different durable id→hash — that caused cross-process `get`→not-found while the durable blob still existed (anti-ghost TRACK `[REDACTED-ID]`). After durable `SetMapping`/`SetMappings` persists id→hash, **evict pending when it still advertises that same hash** (`EvictPendingIfHash`); do not evict a newer concurrent pending hash. Failed Update paths that published then rolled back must **restore** pending to the prior hash (or evict).
