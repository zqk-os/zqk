# Trait Definitions

**Status**: Active  
**Date**: 2026-01-05  
**Related**: REQ-9005, CRIT-9008, BLI-630

## Overview

This directory contains trait definition files that specify the semantics, dependencies, and usage rules for all traits used in the zqk system. Traits are declarative markers that indicate capabilities and behaviors of objects and fields.

## Trait File Format

Each trait is defined in a YAML file with the following structure:

```yaml
name: trait_name
description: |
    Human-readable description of what the trait means
category: standard | domain-specific | system
object_level: true | false  # Can trait be used at object level?
field_level: true | false   # Can trait be used at field level?
requires:                   # List of required traits (dependencies)
    - readable
conflicts:                  # List of conflicting traits
    - immutable
version: 1.0.0
created_at: "2026-01-05T12:00:00Z"
status: active | draft | deprecated
```

## Standard Traits

Standard traits are commonly used across most object types:

- **listable**: Object can be listed/queried in collections
- **readable**: Object/field can be read/retrieved
- **writable**: Object/field can be written/created (requires `readable`)
- **modifiable**: Object/field can be modified/updated (requires `readable`)
- **removable**: Object can be removed/deleted (requires `readable`)
- **formatable**: Object/field can be formatted for display
- **groupable**: Object/field can be grouped in collections
- **filterable**: Object/field can be filtered in queries
- **sortable**: Object/field can be sorted in collections
- **searchable**: Object/field can be searched

## System Traits

System traits are used for system-level functionality:

- **snapable**: Object/field can participate in snapshot operations (requires `readable`)

## Base Trait Groups

Base trait groups provide a way to define common trait sets that are automatically applied to objects extending `auditable` or `base_object`:

- **base_auditable_traits**: Base trait group for all objects extending `auditable`
  - Includes: `listable`, `readable`, `writable`, `modifiable`, `removable`, `formatable`, `filterable`, `sortable`, `searchable`
  
- **base_object_traits**: Base trait group for all objects extending `base_object`
  - Includes: All `base_auditable_traits` plus `groupable`

## Specialized Trait Groups

Specialized groups extend or compose base groups for specific use cases:

### Object-Level Groups

- **read_only_group**: For read-only objects (excludes `writable`, `modifiable`, `removable`)
  - Includes: `listable`, `readable`, `formatable`, `groupable`, `filterable`, `sortable`, `searchable`
  - Use for: System-generated objects, audit events, metrics
  
- **confidential_group**: For confidential objects requiring `access:confidential`
  - Includes: All `base_object_traits`
  - Use for: Objects with sensitive data, restricted access

### Field-Level Groups

- **field_immutable_group**: For immutable fields (set once, never change)
  - Includes: `readable`, `listable`, `filterable`, `sortable`, `searchable`
  - Use for: System-generated IDs, timestamps, immutable identifiers
  
- **field_read_only_group**: For read-only fields (minimal trait set)
  - Includes: `readable`
  - Use for: Computed fields, audit fields, simple read-only fields
  
- **field_queryable_group**: For fields commonly used in queries
  - Includes: `readable`, `listable`, `filterable`, `sortable`, `searchable`
  - Use for: Identifier fields, timestamps, status fields, commonly queried fields
  
- **field_mutable_group**: For standard editable fields
  - Includes: `readable`, `writable`, `modifiable`
  - Use for: User-editable fields like descriptions, titles, content
  
- **field_reference_group**: For reference fields (foreign keys)
  - Includes: `readable`, `filterable`
  - Use for: Object references, foreign keys, relationship fields
  
- **field_display_group**: For fields commonly displayed in lists/tables
  - Includes: `readable`, `listable`, `formatable`
  - Use for: Title, description, status, display-oriented fields

### Using Base Trait Groups

In object specs, you can reference base trait groups instead of listing all traits individually:

```yaml
# Instead of listing all traits:
traits:
  - listable
  - readable
  - writable
  # ... etc

# You can use:
traits:
  - base_object_traits  # Expands to all base_object traits
  - snapable            # Additional specific trait
```

The trait registry automatically expands trait groups into their constituent traits during spec loading and validation.

## Domain-Specific Traits

Domain-specific traits are used for specialized functionality:

- **constrainable**: Object can have layout/constraint rules applied (domain-specific, object-level only)

## Work-envelope traits (opt-in)

These are **not** universal. Do not add them to `base_object` or `auditable`. TRACK: `BLI-KERNEL-WORK-ENVELOPE-001`.

- **completable**: Work interval (`started_at`, `completed_at`). Gantt/strategy/verification bodies that **finish** (`roadmap`, `workstream`, `priority_plan`, `goal`, `strategic_plan`, `requirement`, `test_case`, `convergence_session`) plus any kind that lists `effort_aware`.
- **effort_aware**: Planned vs realized cost (`estimated_effort`, `actual_effort`). `includes: [completable]` — listing `effort_aware` is enough. Timesheet kinds only: `backlog_item`, `milestone`, `technical_debt`, `agent_task`. Do **not** put this on Gantt containers (PRI/roadmap/workstream/goal/strategic_plan) — their cost is child rollup.
- **occupiable**: Occupancy slot (`claimed_by`, `claimed_at`). The slot exists; empty `claimed_by` means unoccupied (parked), not “someone is working.” Exclusive **claim** fills the slot; **assignment** (`persona_refs`) is routing and does not occupy. Fields live on `occupancy`, which extends `work_interval` (not `work_unit`). Timesheets **compose** occupancy; Gantt columns inherit it. TRACK: `POL-KERNEL-GANTT-OCCUPANCY-001`.
- **open_countable**: Remaining-open cardinality. The container's behavior changes when `remaining_open_count` hits zero (complete). Fields live on `remaining_open` (compose; do not redeclare). Parallel to occupiable/`occupancy`. Interpreter CAS-decrements the field on a member terminal hop; unset fail-closes. Opt-in: `priority_plan`. TRACK: `POL-ARCH-20260901` / `BLI-CEF-CONTAINER-REMAINING-OPEN-001`.
- **status_reactive**: Admission for status-event listeners. A catalyst publishes one event to each outbound ref; kinds with this trait interpret YAML `on_dependent_status`. Kinds without the trait no-op. Not occupiable, not open_countable, and not `--auto-status`. Opt-in: `priority_plan`, `milestone`, `goal`, `criteria`. TRACK: `POL-ARCH-20260901`.
- **satisfiable**: Predicate whose done-state is “the proposition holds” (evidence, not duration). Does **not** compose `completable`. Opt-in: `criteria`. Category (`acceptance`, `test`, `compliance`, …) stays a field. Do not stamp `started_at`/`completed_at` on the satisfaction hop.

Neither (work or satisfaction): `mission`, `vision` (no work-done hop), `policy` / `glossary_term`, telemetry kinds. Full table: `docs/architecture/WORK_ENVELOPE_AND_EFFORT_FACETS.md`.

## Trait Loading

The `TraitRegistry` automatically loads all trait definitions from this directory when initialized. The system:

1. Scans the `.zqk/specs/traits/` directory
2. Loads all `.yaml` files
3. Validates trait definitions
4. Registers traits for validation and lookup

If trait files cannot be loaded (e.g., in test environments), the system falls back to hardcoded trait definitions for backward compatibility.

## Adding New Traits

To add a new trait:

1. Create a new YAML file in this directory: `{trait_name}.yaml`
2. Follow the trait file format above
3. Set `status: active` when ready
4. The trait will be automatically loaded on next registry initialization

## Trait Validation

The `TraitRegistry` validates:

- Trait names are unique
- Required dependencies are present
- Conflicting traits are not used together
- Traits are used at appropriate levels (object vs field)
- Field-level traits are subsets of object-level traits

## Related Documentation

- Trait System Architecture: `docs/architecture/spec-loading-order-v1.0.md`
- Trait Validation: `pkg/objects/trait_registry.go`
- Snapshot System: `docs/testing/SNAPSHOT_SNAPABLE_TRAIT.md`

