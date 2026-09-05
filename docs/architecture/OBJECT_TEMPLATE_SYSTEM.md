# Object Template System

**Last Verified:** 2026-08-31


## Overview

The object template system supports creation of objects from spec-driven YAML templates. It provides a two-step workflow: generate a template for a kind, then create an object from the filled-in file.

## Current Implementation

### 1. Template Generation

**Command**: `zqk object template <kind>`

- Generates a YAML template for the given object kind using the field registry and lifecycle (for status values).
- Includes required fields with placeholders and optional fields (commented) with descriptions.
- Output can be written to stdout or to a file via `--output <path>`.
- Supports `--include-optional` and `--include-comments` (see command spec).

**Location**: `cmd/zqk/object/template.go` (`runTemplate`, `generateTemplate`).

### 2. Create from File

**Command**: `zqk object create <kind> --file <path>`

- Loads object data from the specified YAML (or JSON) file.
- Validates and creates the object; on success, temp files (e.g. with `tmp-` prefix) are removed unless `--keep-file` is set.

**Location**: `cmd/zqk/object/create.go` (`loadObjectData`).

### 3. End-to-End Workflow

1. Generate template: `zqk object template backlog_item --output item.yaml`
2. Edit the file to fill required fields (e.g. `title`, `status`).
3. Create object: `zqk object create backlog_item --file item.yaml`

This workflow is covered by tests in `cmd/zqk/object/template_cleanup_test.go` (e.g. template generation, create from template, cleanup behavior). The **required-field prefix** (through the line before `# Optional fields …`) must be **valid YAML** with at least `kind` and `schema_version`. Lines after that use commented placeholders (`field: # …`) so the **entire** file is not always a single valid YAML document until optional lines are edited.

**`zqk object template <kind>`** uses `pkg/cliexamples` (`GenerateYAMLExample`); see `pkg/cliexamples/generator_test.go` for the same parse contract. Instance mint is separate: **`zqk new object <kind> --title "…"`** (draft plane).

**Spec validation (single path):** field-vetting for object_specs goes through `objects.ValidateLoadedSpec` / `SpecValidator.ValidateSpec` — used when `zqk system update-specs … --validate` runs after bulk edits. New specs and pre-existing specs use the **same** rules (no separate “snowflake” validator). **`objects.LoadSpecAndValidate`** performs one load plus that validation (batch helpers). **`zqk system check --validate-specs`** uses the same validation path. For **`zqk system update-specs <ontology> --field … --operation …`**, add **`--validate`** to run **`LoadSpecAndValidate`** on the written spec (fails if checklist debt remains on that kind).

## Related

- **CLI-first workflow**: See `docs/process/architecture/CLI_VS_DIRECT_YAML_ELICITATION.md` (Template-Based Creation).
- **Gap analysis**: `docs/process/architecture/object-management-api-gap-analysis.md` (section 19) describes possible future extensions: stored `template` objects, `CreateFromTemplate` API, template variables and substitution. The current system satisfies the basic “template for object creation” flow; those items are optional enhancements.
