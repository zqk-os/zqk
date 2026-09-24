# Lens catalog

**cef_version:** 0.1.0  

Each lens has a **density class** controlling thoroughness (see Constitution §6).

| Lens ID | Name | Density | Primary diamond axes | Rubric | Prompts |
|---------|------|---------|----------------------|--------|---------|
| `L-PREFLIGHT` | Mechanical preflight / inventory | D-HIGH | CMP, MOD | [`rubrics/L-PREFLIGHT.md`](./rubrics/L-PREFLIGHT.md) | Wave 0 scripted |
| `L-ARCHITECTURE` | Architecture & abstractions | D-LOW | MNT, MOD, RDB | [`rubrics/L-ARCHITECTURE.md`](./rubrics/L-ARCHITECTURE.md) | [`prompts/L-ARCHITECTURE/`](./prompts/L-ARCHITECTURE/) |
| `L-CODE-QUALITY` | Code quality & consistency | D-MED / D-HIGH* | RDB, MNT | [`rubrics/L-CODE-QUALITY.md`](./rubrics/L-CODE-QUALITY.md) | [`prompts/L-CODE-QUALITY/`](./prompts/L-CODE-QUALITY/) |
| `L-SECURITY` | Security & abuse resistance | D-MED | SEC, ROB | [`rubrics/L-SECURITY.md`](./rubrics/L-SECURITY.md) | [`prompts/L-SECURITY/`](./prompts/L-SECURITY/) |
| `L-RELIABILITY` | Reliability & robustness | D-MED | REL, ROB, RCV | [`rubrics/L-RELIABILITY.md`](./rubrics/L-RELIABILITY.md) | [`prompts/L-RELIABILITY/`](./prompts/L-RELIABILITY/) |
| `L-OBSERVABILITY` | Observability & operability | D-MED | OBS, OPS, RCV | [`rubrics/L-OBSERVABILITY.md`](./rubrics/L-OBSERVABILITY.md) | [`prompts/L-OBSERVABILITY/`](./prompts/L-OBSERVABILITY/) |
| `L-TESTING` | Test strategy & testability | D-MED | TST, REL | [`rubrics/L-TESTING.md`](./rubrics/L-TESTING.md) | [`prompts/L-TESTING/`](./prompts/L-TESTING/) |
| `L-USABILITY` | Usability of product surfaces | D-LOW | OPS, RDB, CMP | [`rubrics/L-USABILITY.md`](./rubrics/L-USABILITY.md) | [`prompts/L-USABILITY/`](./prompts/L-USABILITY/) |
| `L-SUPPLY-RELEASE` | Supply chain & release hygiene | D-HIGH | SEC, OPS, RCV | [`rubrics/L-SUPPLY-RELEASE.md`](./rubrics/L-SUPPLY-RELEASE.md) | [`prompts/L-SUPPLY-RELEASE/`](./prompts/L-SUPPLY-RELEASE/) |
| `L-PERFORMANCE` | Performance & resource bounds | D-MED | REL, ROB | [`rubrics/L-PERFORMANCE.md`](./rubrics/L-PERFORMANCE.md) | [`prompts/L-PERFORMANCE/`](./prompts/L-PERFORMANCE/) |
| `L-CONCURRENCY` | Concurrency & failure semantics | D-MED | ROB, REL, RCV | [`rubrics/L-CONCURRENCY.md`](./rubrics/L-CONCURRENCY.md) | [`prompts/L-CONCURRENCY/`](./prompts/L-CONCURRENCY/) |
| `L-DOCS-MODEL` | Docs & mental model | D-LOW | RDB, OPS, CMP | [`rubrics/L-DOCS-MODEL.md`](./rubrics/L-DOCS-MODEL.md) | [`prompts/L-DOCS-MODEL/`](./prompts/L-DOCS-MODEL/) |

\* `L-CODE-QUALITY` splits: **literal/secret/banned-API scans = D-HIGH exhaustive**; naming/style philosophy = D-LOW top-N.

---

## Pros / cons of this catalog

| Pros | Cons |
|------|------|
| Covers ISO 25010-ish concerns without being a formal audit | Overlap between reliability/concurrency/performance — integrator must dedupe |
| Density class prevents “pedantic architecture essays” | Agents may mis-classify density to avoid hard work — wave plan assigns class |
| Separates usability/docs from code beauty | Easy to skip L-USABILITY on libraries — still required for CLIs/services with UX |

**Why:** Your brief named quality, architecture, security, usability; industry practice (ISO/IEC 25010; OWASP for SEC; Google SRE for REL/OBS) adds observability, recoverability, release hygiene, and concurrency as first-class — otherwise “world-class” silently ignores production reality.
