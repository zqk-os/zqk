# Diagram contract

**cef_version:** 0.1.0  

Diagrams are **evidence**, not decoration. Every diagram must make a non-obvious structure *legible* and must be **referentially anchored** to the codebase.

---

## 1. When diagrams are mandatory

| Situation | Required diagrams |
|-----------|-------------------|
| Repo-wide truth map (Wave 1) | Context (C4-L1) + Container (C4-L2) |
| Each major subsystem (Wave 1+) | Component (C4-L3) **or** module dependency graph |
| Cross-service/protocol boundaries | Sequence (happy path) + Sequence (fail/closed path) |
| Non-obvious algorithms / state machines | Flowchart **or** state diagram + short rationale |
| Concurrency / locking designs | Sequence or collaboration diagram naming locks/queues |

**Major subsystem** = a directory or module owning a distinct responsibility (storage, API, scheduler, auth, UI, etc.) with >1k LOC or clear domain language.

## 2. Anchoring rules (anti-hallucination)

Each diagram file includes a metadata header:

```yaml
diagram_id: D-STORAGE-01
type: c4_container | c4_context | c4_component | sequence | flowchart | state | dependency_graph
title: "…"
anchors:
  - path: pkg/storage/file_object_storage.go
    symbol: WriteObject
    note: "primary write path"
claims:
  - "Writes are content-addressed before index update"
evidence_grade: E2
```

- **anchors** must exist (path/symbol). If unsure, mark `evidence_grade: E1` and do not elevate to handoff.  
- **claims** are falsifiable statements the diagram asserts. Adversarial agents attack claims.

## 3. Notation

Preferred (in order):

1. **Mermaid** or **D2** in `.md` (portable, reviewable in PRs)  
2. ASCII boxes if tooling unavailable  
3. External tools (PlantUML, Structurizr) only if operator authorizes — record as tooling gap otherwise

C4 model vocabulary: Simon Brown, *The C4 Model for Visualising Software Architecture* (c4model.com).

## 4. Sequence diagram minimum content

- Actors/systems with real names from the code  
- Request/response or message names matching APIs/events where possible  
- Explicit **error/cancel** path (ROB/RCV lenses care)  
- Timeouts/retries if present in code  

## 5. Pros / cons

| Pros | Cons |
|------|------|
| Forces agents to name real modules | Agents may invent boxes without anchors — hence mandatory anchors |
| C4 scales from context to component | Over-diagramming wastes budget — only major/complex areas |
| Fail-path sequences expose robustness gaps | Sequence diagrams go stale; require git SHA in run_log |

**Why:** Casual scanners miss control flow; picturesque structure is part of a world-class truth map (understandability ∈ ISO 25010 maintainability / usability characteristics).
