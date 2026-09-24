# Rubric: L-TESTING

**Lens:** `L-TESTING` · **Density:** D-MED · **Axes:** TST, REL

## Criteria

1. Critical paths have tests that would fail if broken.  
2. Test pyramid sanity (too many E2E / too few unit).  
3. Flake signals; hidden sleeps; order dependence.  
4. Determinism (time, network, filesystem isolation).  
5. Coverage as **signal**, not a vanity gate — look for untested packages that own risk.

## Citations

| Work | Point |
|------|-------|
| Kent Beck — *TDD by Example* | Characterization of fast feedback (not dogma that all code is TDD) |
| Google Testing Blog / SWE Book testing chapters | Test sizes; hermetic tests |
| ISO/IEC 25010 | Testability under maintainability |

## Pros / cons

| Pros | Cons |
|------|------|
| Focuses on risk coverage not % | Hard without running tests — reading tests still yields E2 |
| Flake detection prevents false REL confidence | Coverage tools may be missing — tooling gap |

## Execution policy

Prefer static reading. Run only narrow tests when claim verification needs it; record command + timeout.
