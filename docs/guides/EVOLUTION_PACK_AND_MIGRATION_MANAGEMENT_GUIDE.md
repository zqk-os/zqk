# Evolution Pack and Migration Management Operational Architecture Guide

## Overview

The `pkg/evolutionpack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the evolution domain pack within the ZQK Knowledge Kernel. It formalizes schema migrations, breaking change coordination, version deprecation cycles, and data plane evolution as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/evolutionpack` acts as the compile-time composition root for the evolution pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│     pkg/evolutionpack       │     │    packs/evolution/...      │
│  - Name: "evolution"        │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_lifecycle_v1        │
│  - LifecycleDir: lifecycles │     │  - bldr_instance_v1         │
│  - Kinds:                   │     │                             │
│      evolution_management   │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"evolution"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/evolution/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/evolution/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_evolutionpack"`, enabling builds to selectively drop the evolution pack from minimal or static kernel binaries without source-level patching.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `evolution_management`: Governance records tracking schema evolutions, phased migrations, deprecation windows, and backwards-compatibility locks.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders, instance schemas, and lifecycle handlers at package initialization time, ensuring that importing `pkg/evolutionpack` fully populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
