# Library Pack Composition, Object Kind Bindings & Spec Mapping Architecture Guide

## Overview

The `pkg/librarypack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the library domain pack within the ZQK Knowledge Kernel. It formalizes documentation entries, library definitions, and technical specifications as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/librarypack` acts as the compile-time composition root for the library pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│      pkg/librarypack        │     │     packs/library/...       │
│  - Name: "library"          │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_lifecycle_v1        │
│  - LifecycleDir: lifecycles │     │  - bldr_instance_v1         │
│  - Kinds: doc_entry,        │     │                             │
│           library,          │     │                             │
│           technical_spec    │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"library"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/library/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/library/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_librarypack"`, enabling builds to selectively drop the library pack from minimal or specialized kernel binaries without source-level patching.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `doc_entry`: Standard documentation articles, guides, and architectural notes.
   - `library`: Reusable component libraries, interfaces, and function catalogs.
   - `technical_spec`: Formal behavioral specifications, EBNF grammars, and protocol definitions.

4. **Zero-Side-Effect Imports:**
   Blank imports (`_`) wire up code-generated builders and lifecycle handlers at package initialization time, ensuring that registering `pkg/librarypack` fully populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
