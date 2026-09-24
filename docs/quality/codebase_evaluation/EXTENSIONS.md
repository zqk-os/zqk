# Extensions

**cef_version:** 0.1.0  

CEF core is language- and product-agnostic. Extensions add precision without capturing the core.

---

## 1. Language adapter pack

Create `adapters/<lang>.md` + optional `adapters/<lang>/banned_apis.txt`, `secrets_patterns.txt`.

Must include:
- Idiom citation list (official style guide, Effective *$LANG*)  
- Recommended mechanical tools  
- Concurrency model notes  
- How D-HIGH scans map to this language  

See [`adapters/go.md`](./adapters/go.md) as an **example**, not a dependency.

## 2. Project / product pack (optional, later)

Examples: open-core launch filter; compliance regime; monorepo policies.

**Rule:** Project packs may **filter/rank** findings for a decision, but must not rewrite diamond grades silently. Publish a separate `launch_triage.md` derived from the truth map.

## 3. New lens

1. Add rubric under `rubrics/L-….md` with citations + density class.  
2. Add row to `LENSES.md`.  
3. Add `prompts/L-…/specialist.md` + `adversarial.md`.  
4. Bump `cef_version` minor.  
5. Assign wave placement in `WAVE_PLAN.md`.

## 4. Schema evolution

- Additive finding fields: minor bump.  
- Removing/renaming diamond axes or severity enums: major bump.
