# Interface Pack and API Command Contract Operational Architecture Guide

## Overview

The `pkg/interfacepack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the interface domain pack within the ZQK Knowledge Kernel. It formalizes programmatic API interfaces, CLI command definitions, RPC boundaries, and endpoint contracts as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/interfacepack` acts as the compile-time composition root for the interface pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│     pkg/interfacepack       │     │    packs/interface/...      │
│  - Name: "interface"        │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_instance_v1         │
│  - LifecycleDir: lifecycles │     │                             │
│  - Kinds: api_spec,         │     │                             │
│           command_spec      │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"interface"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/interface/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/interface/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_interfacepack"`, enabling builds to selectively drop the interface pack from minimal or specialized kernel binaries without source-level modifications.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `api_spec`: REST, gRPC, and GraphQL endpoint definitions, request/response schemas, and protocol bindings.
   - `command_spec`: CLI command signatures, flags, argument grammars, and console interaction specifications.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders and instance schemas at package initialization time, ensuring that importing `pkg/interfacepack` populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
