# ZQK Architecture & Documentation Index (K:F-USA-002 / CRIT-CEF-R8K-USA-002)

**Last Verified:** 2026-09-02

## Documentation Framework (Divio Model)
- [`DOCS_INDEX.md`](./DOCS_INDEX.md): Master index of all docs.

## Divio Framework Organization

```
                  ┌───────────────────┬───────────────────┐
                  │    PRACTICAL      │    THEORETICAL    │
┌─────────────────┼───────────────────┼───────────────────┤
│ LEARNING / WORK │    Tutorials      │    Explanation    │
│                 │ (docs/onboarding) │ (docs/architecture│
├─────────────────┼───────────────────┼───────────────────┤
│ DAILY REFERENCE │   How-To Guides   │     Reference     │
│                 │ (docs/process)    │ (docs/specs, API) │
└─────────────────┴───────────────────┴───────────────────┘
```

### 1. Explanation (Concepts & Architecture)
- [`SYSTEM_SPECIFICATION.md`](./SYSTEM_SPECIFICATION.md): Core architectural invariants, graph data model, and memory hierarchy.
- [`BINARIES.md`](./BINARIES.md): Catalog and role matrix for all 9 CLI entry points.
- [`ADR_INDEX.md`](./ADR_INDEX.md): Index of Architectural Decision Records with lifecycle status.
- [`SUBSYSTEM_STORAGE_AND_SCHEDULER.md`](./SUBSYSTEM_STORAGE_AND_SCHEDULER.md): Deep dive into CAS storage, WAL durability, and distributed scheduler pipeline.
- [`JWT_ALGORITHM_SPECIFICATION.md`](./JWT_ALGORITHM_SPECIFICATION.md): Centralized cryptography and JWT algorithm allowlists.
- [`SPECBUILDER_PERFORMANCE.md`](./SPECBUILDER_PERFORMANCE.md): Generated specbuilder compilation and caching characteristics.

### 2. Tutorials (Onboarding & Mental Model)
- `docs/onboarding/AI_AGENT_ONBOARDING.md`: Core operating protocol for autonomous agents in ZQK swarms.
- `docs/onboarding/COMMUNITY_FIRST_RUN.md`: Developer quickstart for local compilation, setup, and first CLI execution.
- `docs/onboarding/SYSTEM_OBJECTS_GUIDE.md`: Introduction to kernel objects, specs, lifecycles, and graph relations.

### 3. How-To Guides (Operational Recipes)
- `docs/process/backlog/README.md`: How to create, claim, update, and promote Backlog Items (BLIs).
- `scripts/maintenance/README.md`: Maintenance script catalog and operational repair procedures.
- `docs/best-practices/coding/ERROR_HANDLING_STANDARDS.md`: Standards for structured logging and actionable error messages.

### 4. Reference (Specifications & Schemas)
- `docs/process/schemas/`: Formal JSON schema specifications for all kernel object kinds.
- `docs/licensing/LICENSE_BOUNDARY.md`: Open-core boundary and dependency segregation rules.
