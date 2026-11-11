# Object Scaffolding Hygiene and Reference Field Initialization Guide

## Overview

This guide formalizes the architectural invariants governing object scaffolding, draft plane initialization, and reference field validation across the **ZQK Knowledge Kernel**.

In the Knowledge Kernel, references establish typed, directional edges within the persistent graph (linking goals, requirements, criteria, test cases, backlog items, priority plans, milestones, and personas). A reference points to a valid object identifier (e.g., `GOAL-001`, `REQ-100`, `BLI-901`) or is unpopulated (`null` / omitted).

Under no circumstances should optional reference fields be scaffolded or serialized as empty string literals (`""`).

---

## 1. Core Scaffolding Invariants

### Invariant 1: Scalar Reference Omission (POL-DOC-001)
Optional scalar reference fields—attributes where the value represents a single foreign key identifier (such as `team_configuration_ref`, `release_ref`, `milestone_ref`, `owner_ref`, or `previous_plan_id`)—**MUST be omitted entirely** from generated scaffolding templates and draft-plane skeletons when unset.

When a field is optional and has no assigned value:
- **Correct (Omitted)**: Field key is absent from the YAML document.
- **Correct (Explicit Null)**: If a placeholder is required for schema completeness or lifecycle progression, the field is explicitly set to `null` (e.g., `priority_plan_ref: null`).
- **PROHIBITED (Empty String Literal)**: `team_configuration_ref: ""` or `release_ref: ""` is strictly forbidden.

### Invariant 2: Slice and Array Reference Initialization
Optional plural reference fields—attributes where the value is an array or list of object identifiers (such as `criteria_refs`, `requirement_refs`, `goal_refs`, `milestone_refs`, `persona_refs`, `workstream_refs`, or `related_object_refs`)—**MUST be initialized as empty arrays/slices (`[]`)** or omitted entirely.

- **Correct (Empty Array)**: `requirement_refs: []` or `criteria_refs: []`.
- **Correct (Omitted)**: Field key is absent when not required.
- **PROHIBITED (Empty String Literal)**: `requirement_refs: ""` (scalar empty string for slice field).
- **PROHIBITED (Empty Entry in Array)**: `requirement_refs: [""]` or `criteria_refs: ["CRIT-001", ""]`.

---

## 2. Field Classification Taxonomy

A field is classified as a reference field if it satisfies any of the following criteria:

| Category | Criteria | Example Fields | Canonical Scaffolding Representation |
| :--- | :--- | :--- | :--- |
| **Scalar Reference** | `field.Type == "reference"`, `field.SemanticType == "reference"`, suffix `_ref`, or non-base `_id` | `team_configuration_ref`, `release_ref`, `owner_ref`, `milestone_ref`, `previous_plan_id` | Omitted from template or set to `null` |
| **List/Array Reference** | Suffix `_refs` or `_ids`, or `field.Type` in (`list`, `array`, `slice`) with `semantic_type: reference` | `criteria_refs`, `requirement_refs`, `goal_refs`, `persona_refs`, `workstream_refs` | Initialized as `[]` or omitted |

---

## 3. Scaffolding Engine Implementations

Scaffolding hygiene is enforced across three primary generation surfaces:

### 1. Printable CLI Templates (`zqk object template <kind>`)
Located in [`cmd/zqk/object/template.go`](../../cmd/zqk/object/template.go):
- `isReferenceField(field)` detects both scalar (`_ref`, `_id`, `type: reference`, `semantic_type: reference`) and plural (`_refs`, `_ids`) reference fields.
- `isOptionalRefField(field)` omits optional reference attributes from the generated YAML template so users are not presented with empty string defaults.
- Plural reference fields that are generated produce `[]` rather than string placeholders.

### 2. Interactive and Scripted Minting (`zqk new <kind>`)
Located in [`cmd/zqk/new/`](../../cmd/zqk/new):
- Relational flags (`--plan`, `--req`, `--goal`, `--bli`, `--criteria`) only populate reference fields when non-empty arguments are provided.
- If flags are omitted or blank, reference fields are not populated with empty string values.

### 3. Draft Plane Materialization (`WriteObjectToDraftPlane`)
Located in [`pkg/storage/object_draft_plane.go`](../../pkg/storage/object_draft_plane.go):
- `getDraftPlaneTemplateForKind(kind)` merges specialized field placeholders.
- Optional scalar reference fields are excluded from merging via `isDraftPlaneReferenceField`.
- List reference fields are scaffolded as `[]any{}` (empty arrays).

---

## 4. Strict Validation Rejection (Negative Adversarial Boundary)

To prevent downstream CAS state corruption, the validation subsystem in [`pkg/validation/`](../../pkg/validation) strictly rejects empty string literals in reference fields:

### Validation Rules
1. **Scalar Reference Check**: If a scalar reference field (`team_configuration_ref`, `release_ref`, etc.) contains an empty string (`""` or whitespace-only string), validation fails closed and emits a `ValidationError` with rule `reference_not_empty`.
2. **List Reference Entry Check**: If a list reference field contains any empty string entries (e.g. `[""]` or `["VALID-001", ""]`), validation fails closed and emits a `ValidationError` with rule `reference_not_empty`.
3. **Draft and CAS Membrane Protection**: Both draft plane validation and authoritative CAS promotion enforce these constraints. Corrupted drafts containing empty string references cannot be promoted to CAS storage.

---

## 5. Summary Matrix

| Representation | Scalar Reference (`team_configuration_ref`) | Array Reference (`criteria_refs`) |
| :--- | :--- | :--- |
| **Omitted (field absent)** | **VALID** | **VALID** |
| **`null` (`nil`)** | **VALID** (if optional) | **INVALID** (must be array or omitted) |
| **`[]` (empty array)** | **INVALID** (type mismatch) | **VALID** |
| **`""` (empty string)** | **REJECTED** (`ValidationError`) | **REJECTED** (`ValidationError`) |
| **`[""]` (array with empty string)** | N/A | **REJECTED** (`ValidationError`) |
| **`"VALID-ID-001"`** | **VALID** | N/A |
| **`["VALID-ID-001"]`** | N/A | **VALID** |
