# ADR: Public Documentation Quality Gate, Topic Character Budgeting, and Kernel ID Scrutiny

## Context
Public-facing documentation at `docs.zqk.dev` serves external developers, operators, and evaluators adopting the ZQK ecosystem. When documentation is overly verbose, bloated with internal sprint retrospectives, riddled with conflicting acronym expansions (e.g. `ATK` defined inconsistently), or leaks internal Knowledge Kernel entity IDs (`BLI-*`, `REQ-*`, `PRI-*`, `TST-*`, `CRIT-*`, `GOAL-*`, `MIL-*`), adoption friction increases and public trust degrades. 

To maintain pristine editorial standards, public documentation must be succinct, unambiguous, free from internal cycle implementation narratives, and rigorously vetted against canonical acronym definitions.

## Decision
1. **Automated Documentation Scrutiny Suite (`zqk-vet -suite docs`)**:
   - Integrated a dedicated documentation scrutiny suite into `zqk-vet` governed by `pkg/vet/check_docs.go` and `config/gates.yaml`.
   - Enforces character count ceilings per high-level topic (sections delineated by `##`):
     - Concept and design guides: max 4,000 characters per section.
     - Architecture specifications: max 8,000 characters per section.
   - Validates acronym consistency across all markdown files against canonical definitions registered in `pkg/acronyms/acronyms.go` (e.g., ensuring `ATK` denotes "Agent Task", rejecting conflicting expansions).
   - Prohibits internal cycle narrative achievements (e.g. "In Cycle 98 we fixed...") and project-specific raw kernel IDs (`BLI-*`, `REQ-*`, `PRI-*`, `TST-*`, `CRIT-*`, `GOAL-*`, `MIL-*`) in public guides and docs.
2. **Docs Portal Decoupling & Sanitization (`deploy-docs-portal.sh`)**:
   - `scripts/open-core/docs-portal/deploy-docs-portal.sh` executes `./bin/zqk-vet -suite docs` as Step [0/4] before compiling HTML pages.
   - Internal sprint retrospectives (`*_CYCLE*.md`, `*BURNDOWN*.md`, `*DEFECT_REMEDIATION*.md`, `*NORMALIZATION_MAP*.md`) are excluded from public portal compilation in `scripts/open-core/docs-portal/generate_docs_portal.py`.
   - Markdown documents undergo automated sanitization during portal compilation to prevent raw internal kernel IDs from reaching static HTML assets.
3. **Knowledge Kernel Policy & Invariant**:
   - Codified `POL-DOC-001` in the Knowledge Kernel CAS, declaring that all documentation artifacts must adhere to topic brevity limits, canonical terminology, and zero raw kernel ID leakage.
   - Codified Three-Fold Proof verification in `pkg/community/docs_quality_gate_test.go` verifying static floor invariants, operational dynamic proof, and negative adversarial boundary rejection.

## Status
Accepted and codified in Knowledge Kernel for Cycle 101.
