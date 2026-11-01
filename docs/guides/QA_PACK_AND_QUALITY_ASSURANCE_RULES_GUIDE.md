# QA Pack and Quality Assurance Rules Operational Architecture Guide

## Overview

The `pkg/qapack` package provides composition metadata, object kind bindings, specification directory bindings, and lifecycle definitions for the quality assurance (QA) domain pack within the ZQK Knowledge Kernel. It formalizes code quality metrics, validation rules, process hygiene invariants, test audit aggregation, verification matrices, and scenario specifications as first-class, content-addressed kernel entities.

## Architectural Placement & Pack Model

In the ZQK open-core architecture, domain capabilities are decomposed into modular packs (`packs/<pack-name>`). The Go package `pkg/qapack` acts as the compile-time composition root for the QA pack:

```
┌─────────────────────────────────────────────────────────────┐
│                    ZQK Composition Root                     │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│         pkg/qapack          │     │        packs/qa/...         │
│  - Name: "qa"               │     │  - bldr_v2                  │
│  - SpecDir: specs           │     │  - bldr_lifecycle_v1        │
│  - LifecycleDir: lifecycles │     │  - bldr_instance_v1         │
│  - Kinds: 10 QA entities    │     │                             │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Pack Identification Contract:**
   - `Name`: Declares the canonical pack identifier (`"qa"`).
   - `SpecDir`: Establishes the relative path to versioned object schemas (`"packs/qa/specs"`).
   - `LifecycleDir`: Defines the relative path to state machine definitions (`"packs/qa/lifecycles"`).

2. **Omission Build Tag Support:**
   - `OmitBuildTag`: Exposes `"zqk_omit_qapack"`, enabling builds to selectively drop the QA pack from lightweight or production-stripped kernel binaries without source-level patching.

3. **Governed Kind Bindings:**
   The `Kinds()` function enumerates the canonical set of object kinds governed by this pack:
   - `code_quality_metric`: Measurable source code quality dimensions, complexity metrics, and lint results.
   - `code_reference`: Concrete AST anchor points, file references, line ranges, and symbol pointers.
   - `maturation_report`: Rollup reports tracking object and test maturation progress across cycles.
   - `process_hygiene_rule`: Invariant rules governing development process hygiene, git discipline, and branch isolation.
   - `qa_success`: Verified quality attestations certifying readiness for release or promotion.
   - `scenario`: Multi-step functional verification scenarios and behavior-driven test cases.
   - `test_audit_aggregation_metric`: Aggregated metrics summarizing test execution logs, durations, and pass/fail distributions.
   - `test_command_rule`: Execution rules defining how automated tests are invoked, isolated, and bounded.
   - `validation_rule`: Semantic schema validation predicates ensuring object graph consistency.
   - `verification_matrix`: Full traceability matrix linking requirements, criteria, test cases, and verification proofs.

4. **Zero-Side-Effect Initialization:**
   Blank imports (`_`) wire up code-generated builders, instance schemas, and lifecycle handlers at package initialization time, ensuring that importing `pkg/qapack` fully populates the kernel reflection registry without manual runtime wiring.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at 100.0%.
- **Constant Immutability:** Constant paths and naming conventions must be asserted against the physical repository topology.
- **Kind Exhaustiveness:** All registered kind strings must validate against the global kind taxonomy registry.
