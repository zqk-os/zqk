# ZQK Semantic Bridge Phase 3: Reconciliation & Inference

## Overview
This document outlines the technical design, complexity breakdown, and implementation plan for Phase 3 of the Semantic Bridge. Our goal is to move from passive ingestion to active reconciliation and intelligent inference.

## 1. Complexity Breakdown

### A. The "One-Way Mirror" (Reconciliation)
*   **Challenge**: Detecting when an internal `object_spec` has drifted from its source (JSON Schema, RDF).
*   **Solution**: Implement `SemanticSensor` in `pkg/semantic`. This sensor will:
    1.  Query `import_tracking` objects.
    2.  Check for updates in the `source_file`.
    3.  Hash the source vs. the translated internal spec.
    4.  Generate a `DriftEvent` if a mismatch is found.
*   **Complexity**: High (IO-intensive, requires efficient hashing and background scheduling).

### B. Next-Step Inference (Strategic Guidance)
*   **Challenge**: Translating "Low Maturity" into "Actionable Backlog."
*   **Solution**: Extend `MaturityAssessor` to produce `backlog_item` specs.
    *   If Level 0 (Naive) -> Propose "Standardize Core Objects" (High Priority).
    *   If Level 1 (Aware) -> Propose "Generate JSON Schemas for Critical Flows."
    *   If Level 2 (Practicing) -> Propose "Link Schemas to Federated Ontology."
*   **Complexity**: Medium (Heuristic-based, requires strict adherence to `backlog_item` spec patterns).

### C. Bi-Directional Sync (The Bridge Lane)
*   **Challenge**: Pushing internal changes back to external ontologies.
*   **Solution**: Implement `OntologyExporter`.
    *   Target format: RDF/Turtle (Level 3 standard).
    *   Logic: Map internal `object_spec` fields to RDF properties.
*   **Complexity**: Medium/High (Requires graph-to-text serialization).

## 2. Architectural Adherence

| Standard | Implementation Detail |
| :--- | :--- |
| **Object-First** | All findings (Drift, Inferences) MUST be persisted as `convergence_session` or `backlog_item` objects. |
| **Traceability** | Every inference must link to the `import_tracking.id` or the `assessment.id`. |
| **Concurrency** | Use `pkg/concurrency` for background scanning to avoid blocking the main CLI thread. |
| **Error Handling** | Wrap all IO errors in `pkg/errfmt`. |

## 3. Phase 3 Road Map (Next 10 Steps)

1.  **Scaffold `pkg/semantic/reconciler.go`**: Define the `SemanticReconciler` struct.
2.  **Implement `DriftDetection` logic**: Compare `import_tracking.source_hash` with current file hash.
3.  **Update `import_tracking` Spec**: Add `source_hash` and `last_checked_at` fields.
4.  **Register `SemanticSensor`**: Plug into the `convergence` engine.
5.  **Implement `InferenceEngine`**: Basic heuristics for Level 0-2 upgrades.
6.  **Create `zqk semantic infer` command**: Expose the engine via the CLI.
7.  **Test Case: JSON Schema Update**: Verify that changing a schema file triggers a `convergence_session`.
8.  **Implement `TurtleExporter`**: First concrete exporter for Level 3 maturity.
9.  **Audit & Lint**: Ensure zero regression in `pkg/objects`.
10. **Release v1.1.0-alpha**: Finalize the bi-directional bridge.
