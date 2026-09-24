# Rubric: L-PREFLIGHT (Mechanical inventory)

**Lens:** `L-PREFLIGHT` · **Density:** D-HIGH · **Axes:** CMP, MOD

## Purpose

Establish what exists before opinions form: languages, module graph sketch, build/test entrypoints, generated vs hand-written code, obvious binary/secrets risk paths.

## Evaluation criteria (exhaustive within scope)

1. Detect languages/build systems (go.mod, package.json, Cargo.toml, pom.xml, etc.).
2. Enumerate top-level modules and approximate LOC by area.
3. Identify test runners and CI config presence/absence.
4. Flag generated trees (`*.pb.go`, `node_modules`, `dist/`, `vendor/`).
5. List available analyzers (linters, formatters, security scanners, AST tools).
6. Produce initial C4-L1/L2 diagram drafts with anchors (may be refined in L-ARCHITECTURE).

## Evidence expectations

Command outputs, directory listings, tool version strings. Prefer E2.

## Citations

| Work | Point |
|------|-------|
| ISO/IEC 25010 | Product quality — need inventory before characteristic scoring |
| Software engineering practice | Baseline metrics before qualitative review (common SEI/measurement guidance) |

## Pros / cons

| Pros | Cons |
|------|------|
| Cuts hallucination by grounding later lenses | Inventory alone is not quality |
| Surfaces tooling gaps early | LOC is a weak proxy — label as approximate |

## Deliverables

`preflight.json`, draft `diagrams/D-CONTEXT-01.md`, `diagrams/D-CONTAINER-01.md`, `tooling_gaps.md` seed.
