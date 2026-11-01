# Organization Pack and Team Topology Registry Operational Architecture Guide

## Overview

The `pkg/orgpack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the organization domain pack within the ZQK Knowledge Kernel. It formalizes enterprise structures, team topologies, stakeholder profiles, corporate initiatives, and organizational change events as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/orgpack` acts as the compile-time composition root for the organization pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│        pkg/orgpack          │     │       packs/org/...         │
│  - Name: "org"              │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_lifecycle_v1        │
│  - LifecycleDir: lifecycles │     │  - bldr_instance_v1         │
│  - Kinds: 9 org entities    │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"org"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/org/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/org/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_orgpack"`, enabling builds to selectively drop the organization pack from minimal or single-user kernel distributions without source-level code changes.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `corporate_initiative`: Strategic company-wide transformation initiatives and business targets.
   - `department`: Functional departments within the operating enterprise.
   - `division`: High-level business units grouping related product lines and departments.
   - `organization`: Top-level legal or operating entity governance schemas.
   - `organizational_change`: Structured change management records, restructuring events, and reorg plans.
   - `partnership`: Formal external vendor, consortium, or strategic partner integrations.
   - `stakeholder_profile`: Individual or group stakeholder context, decision authority, and communication preferences.
   - `team`: Persistent engineering, product, or operational delivery units.
   - `team_configuration`: Dynamic team composition profiles, staffing matrices, and seat assignments.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders, instance schemas, and lifecycle handlers at package initialization time, ensuring that importing `pkg/orgpack` fully populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
