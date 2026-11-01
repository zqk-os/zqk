# Pipeline Pack and Stage Execution Engine Operational Architecture Guide

## Overview

The `pkg/pipelinepack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the pipeline domain pack within the ZQK Knowledge Kernel. It formalizes multi-stage workflow pipelines, DAG execution stages, automated delivery sequences, and execution runs as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/pipelinepack` acts as the compile-time composition root for the pipeline pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│     pkg/pipelinepack        │     │     packs/pipeline/...      │
│  - Name: "pipeline"         │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_instance_v1         │
│  - LifecycleDir: lifecycles │     │                             │
│  - Kinds: 4 pipeline kinds  │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"pipeline"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/pipeline/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/pipeline/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_pipelinepack"`, enabling builds to selectively drop the pipeline pack from minimal or lightweight runtime distributions without code patching.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `pipeline`: Root workflow pipeline instance connecting triggers, stages, and outputs.
   - `pipeline_definition`: Declarative pipeline blueprint defining step topology, execution parameters, and stage dependencies.
   - `pipeline_execution`: State-tracked execution record capturing telemetry, logs, artifacts, and exit statuses for a specific run.
   - `pipeline_stage`: Discrete stage or step in a pipeline execution graph with input/output contracts.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders and instance schemas at package initialization time, ensuring that importing `pkg/pipelinepack` fully populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
