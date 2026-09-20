---
name: community-arch-design
description: Package boundaries and layering for community kernel work.
---

# System Architecture and Package Boundary Design

## Objective
Govern architectural evolution, maintain clean package boundaries, and prevent cyclic dependencies.

## Architectural Guidelines
1. **3-Tier Layering Model**:
   - Layer 1 (Microkernel): Pure graph, plane routing, IPC, scheduler (`pkg/kernel`).
   - Layer 2 (Standard Library): Core DNA governance schema (`pkg/dna`).
   - Layer 3 (Profiles/Apps): Domain specializations (`pkg/profiles`).
2. **Quarantine Enforcement**:
   - AST compilers, macros, and spec-driven generators must reside exclusively in `internal/codegen`.
   - Runtime packages must never import quarantined internal packages.
3. **Architectural Decision Records (ADRs)**:
   - Document significant architectural changes with permanent epistemic lineage.

