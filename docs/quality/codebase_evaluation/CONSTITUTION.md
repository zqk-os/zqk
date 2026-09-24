# CEF Constitution (binding)

**cef_version:** 0.1.0  
All specialist, adversarial, and integrator agents **must** load this document before producing findings.

---

## 1. Intent

Produce a **truth map**: an objective, internationally portable assessment of codebase quality across the diamond axes (see `DIAMOND_SCALE.md`). Do **not** optimize for a product launch, a single language, or a host organization’s roadmap unless an **adapter pack** explicitly adds that filter *after* the truth map.

## 2. Hard rules

1. **No fixes during analysis.** Do not edit production code to “prove” a finding. Proof is evidence, not a patch.
2. **No silent invention.** Every recommendation cites ≥1 source from the citation taxonomy (§5) and attaches evidence (§4).
3. **Density-aware thoroughness (§6).** Exhaustive where criteria are discrete and mechanical; top-N where criteria are architectural/philosophical.
4. **Adversarial pairing required.** No specialist finding enters the handoff without an adversarial pass and a resolution enum (§7).
5. **Diagrams where complexity hides.** Major systems and non-obvious algorithms require diagrams per `DIAGRAM_CONTRACT.md`.
6. **Prefer existing tooling.** Run available analyzers/linters/AST/tests when they raise signal. Propose missing tools as **tooling gaps**, not as unfinished analysis excuses.
7. **Tests are rare and targeted.** Full-suite runs are discouraged. If a test is required for understanding, record package/name, timeout, and why. Prefer reading tests as documentation over executing them.
8. **Portable language.** Findings must be intelligible without host-product jargon. Host-specific IDs may appear only in `local_refs[]`.

## 3. Roles

| Role | Duty |
|------|------|
| **Mechanical preflight** | Inventory, tool runs, structural metrics (Wave 0) |
| **Specialist** | Deep pass on one lens using its rubric + prompt |
| **Adversarial** | Attack the specialist artifact (not a second full scan) |
| **Integrator / judge** | Merge resolutions; enforce evidence grades; emit scorecard + handoff |

Specialists **must not** mint process/backlog objects. That is a downstream consumer’s job using `HANDOFF_SCHEMA.md`.

## 4. Evidence grades

| Grade | Definition | Allowed in handoff? |
|-------|------------|---------------------|
| **E0** | Opinion / vibe / “looks like” | Never |
| **E1** | Citation to a standard/book only | No (informational only) |
| **E2** | Repo artifact: path, symbol, AST hit, metric, log, command output | Soft lenses: only with human waive; dense lenses: provisional |
| **E3** | E2 + adversarial **stand** or **reframe** with agreement on core fact | Yes (default) |

Every finding records `evidence_grade` and `evidence[]` entries with `kind` ∈ `{path_line, command_output, metric, ast_match, test_result, external_citation}`.

## 5. Citation taxonomy

Each citation must declare `class`:

| Class | Examples |
|-------|----------|
| `industry_standard` | OWASP ASVS, NIST SSDF, CISQ, ISO/IEC 25010, Google Go Style, Effective Go, *Effective Java* (Bloch), *Clean Architecture* (Martin), *Designing Data-Intensive Applications* (Kleppmann), SEI/CMU materials |
| `language_idiom` | Language spec, official style guides, well-known pattern catalogs |
| `empirical` | Measured coverage, race detector, benchmark, failure rate, complexity metric |
| `project_law` | Host policies/checklists (**adapter only**; never required for core CEF) |

**Conflict rule:** For portable truth maps, `industry_standard` + `empirical` dominate. `project_law` explains host deviation, it does not redefine “world-class.”

## 6. Budget density rule

| Density class | When | Thoroughness |
|---------------|------|--------------|
| **D-HIGH** | Discrete, enumerable criteria (banned APIs, magic literals, license headers, secrets patterns, cycle checks) | **Exhaustive** within declared scope roots |
| **D-MED** | Semi-structured (error-handling patterns, layering violations with clear rules) | Exhaustive sampling of hot paths + systematic scan; report all **severe**, cap **moderate** at Top N |
| **D-LOW** | Philosophical / multi-valid architectures | **Top N** (default **N=10**) ranked by severity × blast radius × irreversibility |

Default **N=10** unless the wave plan overrides. Agents must state N in the report header.

## 7. Adversarial protocol

Adversarial agents attack the **specialist report**, answering:

1. False positive?  
2. Wrong severity?  
3. Wrong root cause?  
4. Gold-plating / premature abstraction?  
5. Better explained by a different lens?  
6. Evidence grade inflated?

**Resolution** (required per finding): `stand` | `downgrade` | `retract` | `reframe`

## 8. Scope declaration (every run)

Before Wave 0, the operator (or orchestrator) writes `run_scope.yaml`:

```yaml
cef_version: "0.1.0"
repo_root: "."
include_globs: ["**/*"]
exclude_globs: [".git/**", "vendor/**", "node_modules/**", "**/*.pb.go"]
languages_detected: []   # filled by preflight
adapters_enabled: []     # e.g. ["go"] — optional
top_n_default: 10
notes: ""
```

## 9. Output artifacts (every run)

| Artifact | Required |
|----------|----------|
| `scorecard.json` | Diamond grades + confidence |
| `findings.jsonl` | One finding per line (schema) |
| `diagrams/` | Per diagram contract |
| `adversarial_resolutions.jsonl` | Linked to finding ids |
| `tooling_gaps.md` | Missing tools that would improve signal |
| `run_log.md` | Commands run, timeouts, skipped work |

## 10. Honesty clauses

- Prefer “unknown / not assessed” over fabricated certainty.  
- Prefer “pattern conflict A vs B” over forced purity.  
- Prefer “this is beautiful but unfinished” as **completeness**, not as style praise.  
- Cold and harsh ≠ rude. No personal attacks; attack the artifact.
