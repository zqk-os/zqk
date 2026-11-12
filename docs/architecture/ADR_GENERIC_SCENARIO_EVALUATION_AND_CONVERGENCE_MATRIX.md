# ADR: Core Generic Scenario Evaluation Framework & Convergence Matrix Engine

## Status
Accepted

## Context
In multi-agent collaborative workflows and automated swarms, verifying system convergence across multiple distinct model providers and autonomous actors requires deterministic, structured evaluation mechanics. Previously, ad-hoc scenario assertions lacked aggregate statistical variance modeling and multi-model convergence reporting.

To guarantee that code transformations and architectural refactorings converge toward defined invariants, the kernel requires a first-class scenario evaluation framework and matrix convergence engine.

## Decision
1. Implement and standardize `MatrixEngine` under `pkg/eval/` to orchestrate multi-scenario, multi-model evaluation matrices.
2. Provide statistical aggregation across evaluation runs:
   - Run counts, passed counts, and pass rates per model.
   - Mean composite quality scores and score variance calculation.
   - Aggregate remediation recommendation synthesis.
3. Integrate declarative CLI surface `zqk eval matrix` supporting structured output formats (`json`, `yaml`, `stream`).
4. Validate compliance using automated Three-Fold Proof verification covering static floor invariants, operational dynamic proof, and negative adversarial error boundaries.

## Consequences
- Multi-agent convergence sessions obtain reliable, quantitative scoring across multiple models.
- Model performance variances and regression drifts are surfaced before code promotion.
