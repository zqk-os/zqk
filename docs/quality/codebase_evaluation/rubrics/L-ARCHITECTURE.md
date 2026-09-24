# Rubric: L-ARCHITECTURE

**Lens:** `L-ARCHITECTURE` · **Density:** D-LOW (Top N=10) · **Axes:** MNT, MOD, RDB

## Purpose

Judge whether abstractions and boundaries clarify or obscure the domain; detect conflicting architectural patterns; illuminate major systems.

## Criteria (Top N)

1. **Boundary clarity** — modules map to responsibilities; cyclic deps called out.  
2. **Abstraction honesty** — interfaces exist for reasons; no “interface for every struct.”  
3. **Pattern conflict** — e.g. mixed layered + free-for-all imports; dual sources of truth.  
4. **Complexity hotspots** — algorithms/state machines needing sequence/flow diagrams.  
5. **Extension points** — how new features are meant to land (plugins, registries, codegen).

## Non-criteria (avoid)

- Rewriting to a preferred architecture religion without evidence of pain.  
- Naming bikesheds without maintainability impact.

## Citations

| Work | Point |
|------|-------|
| Simon Brown — C4 Model | Context/container/component views |
| Robert C. Martin — *Clean Architecture* | Dependency rule; boundaries |
| Eric Evans — *Domain-Driven Design* | Bounded contexts (when domain language is rich) |
| ISO/IEC 25010 | Maintainability / modularity characteristics |

## Pros / cons of this rubric

| Pros | Cons |
|------|------|
| Forces conflict detection over purity cosplay | Top-N may miss a silent dependency cycle — pair with preflight graph if tools exist |
| Mandates diagrams for hotspots | DDD citations over-applied to CRUD tools — adversarial should retract |

## Recommendation style

Prefer “two patterns coexist; pick one and migrate” over “adopt Clean Architecture everywhere.”
