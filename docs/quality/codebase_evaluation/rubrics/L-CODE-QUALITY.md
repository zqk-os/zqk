# Rubric: L-CODE-QUALITY

**Lens:** `L-CODE-QUALITY` · **Density:** split D-HIGH / D-LOW · **Axes:** RDB, MNT

## D-HIGH (exhaustive)

Scan for discrete defects within scope:

- Magic numbers / unexplained literals (where constants or enums should exist)  
- Hardcoded secrets/credentials patterns  
- Banned/dangerous APIs (language-specific lists in adapters)  
- Inconsistent error-return conventions **when a dominant convention is detectable**  
- Dead/commented-out code blocks of material size  
- Copy-paste clones of non-trivial logic (rule of three)

## D-LOW (Top N)

- Naming clarity and ubiquitous language drift  
- File/module size outliers  
- Style inconsistency that impedes review (not formatter noise)

## Citations

| Work | Point |
|------|-------|
| Robert C. Martin — *Clean Code* | Meaningful names; small functions (use carefully; not dogma) |
| Josh Bloch — *Effective Java* | Prefer clear APIs; avoid public fields; builder where needed (if Java) |
| Google style guides / Effective Go | Language idioms |
| CISQ / ISO 5055 automated source measures | Reliability/security/maintainability weakness categories (conceptual) |

## Pros / cons

| Pros | Cons |
|------|------|
| Exhaustive literal/secret scans are high-signal | Clean Code absolutism causes false severity — adversarial must downgrade |
| Separates mechanical from philosophical | Clone detection without tooling is approximate |

## Pros of recommending constants for magic values

Reduces drift and review load (**MNT**). **Con:** over-constanting one-use literals adds indirection — require second occurrence or domain meaning before finding severity ≥ moderate.
