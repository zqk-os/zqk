# Diamond Scale — multi-axis quality grading

**cef_version:** 0.1.0  

“World-class” is **not** a single slogan. Like diamond grading, quality is a **vector** of axes. A codebase may be secure yet unreadable, or elegant yet unrecoverable. CEF forbids collapsing the vector into one marketing number without showing the axes.

Inspiration: gemological practice separates **independent** attributes (e.g. GIA’s cut / color / clarity / carat weight) rather than a single adjective ([GIA Diamond Grading](https://www.gia.edu/diamond-grading)). Software quality similarly separates concerns in ISO/IEC 25010 product quality model (functional suitability, performance efficiency, compatibility, usability, reliability, security, maintainability, portability).

---

## 1. Axes (the “Cs” for code)

| Axis ID | Name | Plain meaning |
|---------|------|---------------|
| `RDB` | Readability | Can a skilled stranger understand intent without tribal knowledge? |
| `MNT` | Maintainability | Can change land safely with local reasoning? |
| `TST` | Testability | Can behavior be locked with fast, deterministic checks? |
| `REL` | Reliability | Does it behave correctly under expected load and failure? |
| `OBS` | Observability | Can operators see health, causality, and failure modes? |
| `RCV` | Recoverability | Can the system and humans recover from faults/data loss? |
| `SEC` | Security | Are confidentiality, integrity, and abuse resistance engineered? |
| `ROB` | Robustness | Does it fail closed, bound resources, and reject garbage inputs? |

Optional axes (report separately; do not bury inside the eight):

| Axis ID | Name | When to use |
|---------|------|-------------|
| `CMP` | Completeness | Advertised surface vs implemented/tested reality (truthfulness) |
| `OPS` | Operability | Install, configure, upgrade, debug by a non-author |
| `MOD` | Modularity | Boundary clarity; cost to extract/replace a subsystem |

---

## 2. Grade ladder (per axis)

Use **integer 1–5** plus a **confidence** 0–1.

| Grade | Label | Definition |
|------:|-------|------------|
| **5** | Flawless (exhibition) | Exemplary under the axis rubric; few/no material findings; evidence-rich |
| **4** | Fine | Solid professional quality; minor issues; no systemic failure mode |
| **3** | Commercial | Shipable with known debts; mixed patterns; material but contained findings |
| **2** | Rough / industrial | Significant structural problems; high change risk; weak evidence of control |
| **1** | Cull | Dangerous or opaque under this axis; do not trust without remediation |

**Mapping note:** ISO/IEC 25010 provides *which* qualities matter; the 1–5 ladder is CEF’s operationalization for agent scoring (not an ISO conformance claim).

---

## 3. Scorecard rules

1. **Grade each axis independently.** Do not average away a SEC=1 with RDB=5.  
2. **Record confidence.** Low evidence ⇒ low confidence, not a fake mid-grade.  
3. **List top drivers.** Each axis grade cites ≤5 finding_ids that drove it.  
4. **Conflict callouts.** If two axes trade off (e.g. ROB fail-closed vs usability), state the tradeoff explicitly.  
5. **No single “GPA” in v0.** Optional composite may be added later as an extension; v0 publishes the radar/vector only.

### Scorecard shape

See `schemas/scorecard.schema.json`.

```json
{
  "cef_version": "0.1.0",
  "axes": {
    "RDB": {"grade": 3, "confidence": 0.7, "drivers": ["F-…"], "notes": ""},
    "MNT": {"grade": 2, "confidence": 0.8, "drivers": [], "notes": ""}
  },
  "optional_axes": {},
  "integrator": "role:integrator",
  "assessed_at": "ISO-8601"
}
```

---

## 4. Pros / cons of this scale

| Pros | Cons |
|------|------|
| Separates “pretty code” from “safe in production” | Agents may still halo-bias across axes without adversarial review |
| Portable across languages | 1–5 is coarse; two “3”s can differ widely |
| Aligns with ISO 25010 vocabulary for international readers | Not a certification; must not be marketed as GIA/ISO audit |
| Forces explicit completeness/operability as optional axes | Teams may ignore optional axes that matter for strangers |

**Why this recommendation:** A single “world-class” bit recreates the ambiguity you called out. Multi-axis grading matches both gemology practice (independent attributes) and ISO/IEC 25010 (product quality characteristics), giving a cold comparative language without pretending one number captures truth.
