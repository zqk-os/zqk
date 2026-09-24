# Wave plan

**cef_version:** 0.1.0  

Waves are sequential. Do not start Wave N+1 until Wave N integrator gate passes (or operator waives in `run_log.md`).

---

## Wave 0 — Mechanical preflight (D-HIGH)

**Agents:** 1 mechanical operator (can be human or script-assisted agent).  
**Lens:** `L-PREFLIGHT`  
**Outputs:** `preflight.json`, draft context/container diagrams, initial `tooling_gaps.md`, `run_scope.yaml` filled.

**Gate:** Languages detected; include/exclude globs frozen; available tools listed.

---

## Wave 1 — Structure & risk skeleton

**Lenses (parallel OK):**
- `L-ARCHITECTURE` (specialist → adversarial)
- `L-SECURITY` (specialist → adversarial)
- `L-SUPPLY-RELEASE` (specialist → adversarial)

**Gate:** C4-L1/L2 anchored; critical/high security & supply findings have E2+ pending adversarial; architecture Top N emitted.

---

## Wave 2 — Production qualities

**Lenses (parallel OK):**
- `L-RELIABILITY`
- `L-OBSERVABILITY`
- `L-CONCURRENCY`
- `L-PERFORMANCE`

**Gate:** Fail-path sequences for top reliability/concurrency hotspots; false-green OBS findings flagged.

---

## Wave 3 — Changeability & stranger experience

**Lenses (parallel OK):**
- `L-CODE-QUALITY` (D-HIGH exhaustive subset first, then D-LOW Top N)
- `L-TESTING`
- `L-USABILITY`
- `L-DOCS-MODEL`

**Gate:** Exhaustive literal/secret/banned scans complete within scope; usability/docs Top N ready.

---

## Wave 4 — Integrator judgment

**Agent:** Integrator only (see `prompts/L-INTEGRATOR/specialist.md`).

**Tasks:**
1. Apply adversarial resolutions to findings.  
2. Dedupe cross-lens duplicates.  
3. Assign diamond axis grades with drivers.  
4. Emit `scorecard.json`, consolidated `findings.jsonl`, `HANDOFF` package.

**Gate:** No E0 in handoff; no unresolved critical without `stand` or explicit waive.

---

## Parallelism & seats

- Max recommended concurrent specialist seats: **4** (token control).  
- Adversarial should be a **different** agent instance than the specialist for the same lens.  
- Integrator must not have authored the findings it judges.

## Pros / cons

| Pros | Cons |
|------|------|
| Front-loads inventory and security | Longer calendar time than one megaprompt |
| Matches density rule | Operators may skip Wave 4 — forbid “specialist-only” as final |

**Why this order:** Structure and abuse surface first (what can hurt you), then production semantics, then craft/usability — matching how international quality models separate characteristics while controlling token burn.
