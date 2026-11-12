# ADR-0036: Public Release Candidate v0.3.5 Certification, Pre-Flight Gates & Documentation Scrutiny

- **Title: Public Release Candidate v0.3.5 Certification**, Pre-Flight Gates & Documentation Scrutiny
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

Preparing release v0.3.5 of ZQK Open-Core requires formal certification of all pre-flight gates, verification of public documentation conciseness, topic character budgeting, elimination of internal cycle sprint narratives from public portals, and flawless Three-Fold Proof verification across all subsystems.

## Decision

1. **Strict Documentation Quality Gate**:
   - Enforce maximum 12,000 characters per high-level topic topic chapter.
   - Automatically filter and drop internal cycle retrospective narratives from the public documentation portal.
   - Run `zqk-vet` documentation scrutiny gate fail-closed prior to static portal compilation.

2. **Pre-Flight Certification**:
   - Verify 100% of unit test matrix shards pass before cutting the distribution tag.
   - Compile static documentation portal via `generate_docs_portal.py` and deploy to `docs.zqk.dev`.

3. **Ontological Traceability**:
   - Ensure all release deliverables are bound 1:1 to Knowledge Kernel CAS objects (`milestone`, `requirement`, `criteria`, `backlog_item`, `test_case`).

## Consequences

- Public documentation remains pristine, concise, and professional, free of internal cycle terminology or leaked kernel identifiers.
- Production release tags are guaranteed to satisfy all pre-flight gates deterministically.
