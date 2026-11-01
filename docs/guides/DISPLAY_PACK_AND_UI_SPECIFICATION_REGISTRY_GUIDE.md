# Display Pack and UI Specification Registry Operational Architecture Guide

## Overview

The `pkg/displaypack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the display domain pack within the ZQK Knowledge Kernel. It formalizes user interface components, views, dashboards, and display representations as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/displaypack` acts as the compile-time composition root for the display pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│      pkg/displaypack        │     │     packs/display/...       │
│  - Name: "display"          │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_lifecycle_v1        │
│  - LifecycleDir: lifecycles │     │  - bldr_instance_v1         │
│  - Kinds: component,        │     │                             │
│           display           │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"display"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/display/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/display/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_displaypack"`, enabling builds to selectively drop the display pack from minimal, headless, or server-only kernel binaries without source-level modifications.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `component`: Reusable UI components, widgets, cards, and interactive elements.
   - `display`: Full view layouts, screens, dashboards, and presentation compositions.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders, instance schemas, and lifecycle handlers at package initialization time, ensuring that importing `pkg/displaypack` populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
