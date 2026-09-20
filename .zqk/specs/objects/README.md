# Object Specs Directory

**Status**: Active  
**Date**: 2025-12-21  
**Last Updated**: 2026-01-25

This directory contains object specifications in YAML format. All v1 specs have been archived to `object_specs_archived/` and the system now uses specs from this directory.

## Current State

✅ All v2 specs moved from `object_specs_v2/` to this directory  
✅ File naming: `{object_type}.yaml` (no `_v2` suffix)  
✅ System uses specs from `object_specs/` directory  
✅ Display length constraints added for listable fields

## Spec Files

All object spec files are located here:
- `base_object.yaml` - Core object fields
- `backlog_item.yaml` - Backlog item fields
- `goal.yaml` - Goal fields
- `milestone.yaml` - Milestone fields
- `workstream.yaml` - Workstream fields
- `requirement.yaml` - Requirement fields
- And many more...

See the directory listing for the complete set of spec files.

## High-volume kinds

Kinds designated as **high-volume** (see `../configs/high_volume_kinds.yaml`) retain full specs here but **require efficient, compressed storage** (stream storage or timeseries). The legacy one-file-per-object (CAS) format is deprecated for them. See `docs/architecture/HIGH_VOLUME_STORAGE_DEPRECATION.md`.

Each high-volume kind spec includes **`storage_profile: stream`** at the top level (after `visibility`) so the resolved spec does not inherit **`cas_entity`** from `auditable` via `base_object`; the canonical list and code that enforces stream storage live in `../configs/high_volume_kinds.yaml` and `pkg/storage/stream_config.go`. Examples include metric kinds, `mcp_session`, `zqk_session`, and **`verification_matrix`** (matrix registry metadata and frequent updates use stream segments + `stream_current`).

## Display Length Constraints

All fields with the `listable` trait should include a `display_length` constraint in their `validation` section. This ensures consistent table formatting across CLI commands.

**Example**:
```yaml
job_type:
    traits:
        - listable
    validation:
        required: true
        display_length: 28  # Maximum characters for table display
```

See `../documentation/display-length-constraints-v1.0.md` for complete guidelines.
