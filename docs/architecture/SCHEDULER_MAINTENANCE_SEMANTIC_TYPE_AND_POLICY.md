# Scheduler Maintenance: Semantic Type and Policy Alignment

**Last Verified:** 2026-08-31


**Status:** Design  
**Purpose:** Define a semantic type for the scheduler maintenance config shape so check and ensure share one named, machine-readable contract. Evaluate alignment with the bundling and scenario patterns.

---

## 1. Why a semantic type

Option A (policy with a structured field) works only if we **semantically understand** the data shape:

- **Single word:** We need one term that means "this value is a scheduler maintenance config" so both system check and ensure-retention-jobs (and any future consumers) know how to validate and apply it.
- **Machine-readable contract:** Validation (check) and application (ensure) should use the same schema and rules, not ad-hoc logic in each place.
- **Spec alignment:** Object specs already use `semantic_type` on fields (statement, reference, expression, etc.) for validation and UX. Extending that to a **named config shape** gives a single place to define the contract.

So we should define a **semantic type** for this shape and use it wherever the config appears (policy field, config file, or future bundle section).

---

## 2. Proposed semantic type: `scheduler_maintenance_config`

**Name:** `scheduler_maintenance_config` (or a shorter alias such as `maintenance_job_set` if we want a more generic name for "set of required maintenance jobs").

**Definition:** A structured value that defines which scheduler maintenance jobs are required and how they are created (from templates). Both **check** (compliance: "does running state match this config?") and **ensure** ("create or correct jobs from this config") consume this shape.

**Shape (current):**

```yaml
required_jobs:
  - id: SCH-101
    job_type: maintenance
    template_file: scripts/scheduler_jobs/maintenance_wal_trigger.yaml
  # ...
# Optional (future): startup_burst_job_ids, trigger_queue_on_start
```

**Contract:**

- Root: object.
- `required_jobs`: list of objects; each entry has:
  - `id` (string, required): fixed scheduler_job id.
  - `job_type` (string, required): expected job_type.
  - `template_file` (string, required): path relative to project root to a job template YAML.
- Optional top-level fields (reserved for future use): `startup_burst_job_ids`, `trigger_queue_on_start`.

**Where to define it:**

- **Ontology / semantic types:** Add `scheduler_maintenance_config` to the semantic types ontology (`docs/architecture/semantic-types-ontology-v1.0.md`) with the same pattern as `statement`, `reference`, `expression`: definition, validation rules, and optional Schema.org/ISO 11179 mapping.
- **Ontology registry:** In `pkg/validation/ontology_registry.go`, register a new mapping with validation rules that enforce the shape (object with `required_jobs` list; each item has `id`, `job_type`, `template_file`).
- **Policy spec:** Add a field (e.g. `scheduler_maintenance`) on the policy kind with `type: object` and `semantic_type: scheduler_maintenance_config`. Then:
  - **Check** can read policy objects, find a policy that carries `scheduler_maintenance`, validate it via the semantic type, and compare to running scheduler state (required jobs exist, enabled, correct type).
  - **Ensure** can read the same field (or the same config file that is the current source of truth) and apply it. Over time, policy can become the source of truth for "what maintenance we require" and the YAML config file can be generated from or validated against policy.

**Single word:** We use the semantic type name **`scheduler_maintenance_config`** (or `maintenance_job_set`) as the single term. Any field or document that holds this shape should declare that type so tooling knows how to validate and interact with it.

---

## 3. Alignment with bundle and scenario

### 3.1 Existing patterns

- **Bundle (`pkg/scenario/bundle.go`):** Top-level document with `APIVersion`, `Kind`, `Metadata`, `Objects` (e.g. `FixtureGroup.SchedulerJobs`), and `Steps`. `SchedulerJobFixture` is `{ IDHint, ID, Template }` — i.e. **inline** job template (full `map[string]any`). Used for traceability and test scenarios; apply flow creates objects from the bundle.
- **Scenario object spec:** Has structured config fields such as `data_generation_config`, `infrastructure_config`, `scheduler_config` (SCN-008). `scheduler_config` is a generic object ("enabled, project_type, job overrides"); it does not currently define the same shape as our required_jobs + template_file list. See `docs/testing/SCENARIO_SPEC_FIELDS.md` and `.zqk/specs/objects/scenario.yaml`.
- **Scenario spec enhancement:** `docs/testing/SCENARIO_SPEC_ENHANCEMENT_SUMMARY.md` documents `scheduler_config` (SCN-008) and other config blobs; these are purpose-described but not yet a shared semantic type.

### 3.2 Pattern to follow

- **Define the shape once as a semantic type:** Use the same approach as other semantic types: one definition (ontology doc + registry validation) for `scheduler_maintenance_config`. Then:
  - The **config file** (`scheduler_maintenance_config.yaml`) is an instance of that type.
  - A **policy field** `scheduler_maintenance` with `semantic_type: scheduler_maintenance_config` holds the same shape and is validated by the ontology registry.
  - Optionally, a **bundle** kind (e.g. `kind: scheduler_maintenance`) or a **scenario** field could reference or embed this shape: e.g. a bundle section that lists required jobs with `template_file` (ref-based) and/or optional inline `template` (bundle-style) for portability. That keeps one semantic contract while allowing both "ref to template file" (repo as source of truth) and "inline template" (bundle as source of truth) if we want.
- **Bundle vs config file:** The current ensure path is **ref-based** (template_file). The bundle pattern is **inline** (Template map). Both can coexist: the semantic type defines the **ref-based** shape (id, job_type, template_file). Bundles can reuse that shape for a section (e.g. `objects.scheduler_maintenance: { required_jobs: [...] }`) or we can add a separate bundle kind that applies "maintenance job set" from a bundle. The important part is that the **semantic type** names and validates the ref-based shape so check and ensure share one contract; bundling then becomes one way to *carry* that shape.

### 3.3 Recommendation

- Add the **semantic type** `scheduler_maintenance_config` and its validation to the ontology and registry.
- Add a **policy field** (e.g. `scheduler_maintenance`) with `semantic_type: scheduler_maintenance_config` so policy can be the declarative source and both check and ensure use it.
- Keep the **existing config file** as a valid source (and optionally have it validated/loaded as an instance of the same semantic type); over time, policy can drive or mirror the config so there is still a single logical source.
- **Bundling:** Use the same shape (required_jobs with id, job_type, template_file) in any future bundle section or kind that represents "maintenance job set"; avoid duplicating the structure. If we later want inline templates in bundles, we can extend the semantic type to allow optional `template` in each entry and have ensure support both ref and inline.

---

## 4. References

- **Semantic types ontology:** `docs/architecture/semantic-types-ontology-v1.0.md`
- **Ontology registry:** `pkg/validation/ontology_registry.go` (RegisterMapping, ValidateSemanticType)
- **Policy spec:** `.zqk/specs/objects/policy.yaml` (structured fields: applicability, enforcement)
- **Scenario spec / scheduler_config:** `.zqk/specs/objects/scenario.yaml` (SCN-008), `docs/testing/SCENARIO_SPEC_FIELDS.md`
- **Scenario spec enhancement:** `docs/testing/SCENARIO_SPEC_ENHANCEMENT_SUMMARY.md`
- **Bundle types:** `pkg/scenario/bundle.go` (Bundle, FixtureGroup, SchedulerJobFixture)
- **Current maintenance config:** `.zqk/specs/configs/scheduler_maintenance_config.yaml`, `pkg/config/scheduler_maintenance.go`
- **Maintenance WAL and config:** `docs/architecture/MAINTENANCE_WAL_AND_RUNNER.md` (centralized config section)
