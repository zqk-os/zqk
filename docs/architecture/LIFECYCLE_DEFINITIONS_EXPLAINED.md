# Lifecycle Definitions: Architecture and Management

**Created:** 2026-01-12  
**Status:** Documentation  
**Purpose:** Clarify lifecycle definitions vs. objects, their structure, and update mechanisms

## Key Distinction: Lifecycle Definitions Are NOT Objects

**Lifecycle definitions are configuration/metadata files, not objects.** They define state machines (statuses, transitions, preconditions) for object kinds, but they themselves are not objects that can be created/updated through the normal object storage system.

## What Are Lifecycle Definitions?

Lifecycle definitions are YAML files stored in `docs/architecture/_internal/lifecycles/` that describe:

1. **Valid statuses** for an object kind (e.g., `exploring`, `validated`, `in_progress`, `complete`)
2. **Valid transitions** between statuses (e.g., `exploring → validated`)
3. **Preconditions** for statuses and transitions (e.g., "priority_plan_ref is set")
4. **Status properties** (initial, terminal, archive, system)
5. **Percent complete configuration** (how to calculate completion percentage)

### Example: `backlog_item_lifecycle.yaml`

```yaml
object_type: backlog_item
extends: base_lifecycle
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: validated
    display: Validated
  - value: in_progress
    display: In Progress
    preconditions:
      - priority_plan_ref is set
      - At least one milestone_ref linked
  - value: complete
    display: Complete
    terminal: true
transitions:
  - from: exploring
    to: validated
    description: Idea reviewed and approved
    manual: true
    auto: false
  - from: validated
    to: in_progress
    description: Active development begins
    manual: true
    auto: false
    preconditions:
      - priority_plan_ref is set
```

## Why Don't Lifecycle Definitions Have Object Specs?

**Lifecycle definitions don't need object specs because:**

1. **They're not objects** - They're metadata/configuration files, not instances of a "lifecycle" object kind
2. **They're validated by structure, not spec** - The Go `Lifecycle` struct defines the schema
3. **They're internal/system files** - They're in `_internal/`, not user-facing objects
4. **They're loaded directly, not through storage** - `LifecycleLoader` reads YAML files directly via `os.ReadFile()`

### What Validates Lifecycle Definitions?

Lifecycle definitions are validated by:

1. **YAML parsing** - Must be valid YAML syntax (handled by `yaml.Unmarshal`)
2. **Go struct validation** - Must match the `Lifecycle` struct in `pkg/objects/lifecycle_loader.go`
3. **Runtime validation** - Circular dependency detection, inheritance validation
4. **No formal spec validation** - There's no `lifecycle.yaml` spec file in `object_specs/`

### Structure Definition (Go Struct)

The structure is defined in code:

```go
// pkg/objects/lifecycle_loader.go

type Lifecycle struct {
    ObjectType      string                `yaml:"object_type"`
    Extends         string                `yaml:"extends,omitempty"`
    StatusMapping   map[string]string     `yaml:"status_mapping,omitempty"`
    Statuses        []Status              `yaml:"statuses"`
    Transitions     []Transition          `yaml:"transitions"`
    PercentComplete PercentCompleteConfig `yaml:"percent_complete"`
}

type Status struct {
    Value         string   `yaml:"value"`
    Display       string   `yaml:"display"`
    Initial       bool     `yaml:"initial,omitempty"`
    Terminal      bool     `yaml:"terminal,omitempty"`
    Archive       bool     `yaml:"archive,omitempty"`
    System        bool     `yaml:"system,omitempty"`
    Preconditions []string `yaml:"preconditions,omitempty"`
    Description   string   `yaml:"description,omitempty"`
}

type Transition struct {
    From          string   `yaml:"from"`
    To            string   `yaml:"to"`
    Description   string   `yaml:"description"`
    Manual        bool     `yaml:"manual"`
    Auto          bool     `yaml:"auto"`
    Preconditions []string `yaml:"preconditions,omitempty"`
}
```

## How Are Lifecycle Definitions Updated?

**Lifecycle definitions are updated by directly editing YAML files** (not through CLI object management).

### Current Update Mechanism

1. **Direct file editing** - Edit the YAML file directly (e.g., `docs/architecture/_internal/lifecycles/backlog_item_lifecycle.yaml`)
2. **No CLI command** - There's no `zqk object create lifecycle` or `zqk object update lifecycle` command
3. **Cache invalidation** - After editing, call `zqk system check --refresh-cache` to reload lifecycles
4. **Manual validation** - Validate syntax/structure by running `zqk system check` and checking for lifecycle-related errors

### Why Not Through Object Storage?

Lifecycle definitions are NOT managed through object storage because:

1. **They're system metadata** - They define how the system validates objects, not user data
2. **They need to be available before object validation** - LifecycleLoader needs to load lifecycles before validating object instances
3. **They're static configuration** - They change infrequently (unlike object instances)
4. **They're version-controlled with code** - They're part of the codebase, not runtime data
5. **No need for CAS/hash registries** - They're not objects, so they don't need object-level integrity tracking

### File-Based Management

- **Location:** `docs/architecture/_internal/lifecycles/`
- **Naming:** `{kind}_lifecycle.yaml` (e.g., `backlog_item_lifecycle.yaml`)
- **Loading:** `LifecycleLoader.LoadLifecycle(kind)` reads file directly
- **Caching:** LifecycleLoader caches loaded lifecycles (invalidated on file mtime change)
- **Inheritance:** Lifecycles can extend other lifecycles (e.g., `extends: base_lifecycle`)

## Comparison: Object Specs vs. Lifecycle Definitions

| Aspect | Object Specs | Lifecycle Definitions |
|--------|-------------|----------------------|
| **Location** | `docs/architecture/_internal/object_specs/` | `docs/architecture/_internal/lifecycles/` |
| **File Pattern** | `{kind}.yaml` | `{kind}_lifecycle.yaml` |
| **Are They Objects?** | No (they're metadata) | No (they're metadata) |
| **Do They Have Specs?** | No (they define specs) | No (validated by Go struct) |
| **How Updated?** | Direct file edit | Direct file edit |
| **How Loaded?** | `SpecLoader.LoadSpec()` | `LifecycleLoader.LoadLifecycle()` |
| **Used For?** | Validating object instances | Validating object status/transitions |
| **Versioned?** | Yes (`schema_version` field) | No (no version field) |
| **Can Extend?** | Yes (`extends` field) | Yes (`extends` field) |
| **CLI Management?** | No (`zqk internal update spec` might exist but edits file) | No (direct file edit only) |

## Should Lifecycle Definitions Have Specs?

**Current Design Decision: No**

### Arguments Against Specs for Lifecycle Definitions

1. **They're system metadata, not user data** - They define validation rules, not business objects
2. **Structure is simple and well-defined** - The Go struct is sufficient
3. **Changes are infrequent** - Not worth the overhead of spec-based validation
4. **They're already validated** - YAML parsing + struct validation is adequate
5. **No user-facing operations** - Users don't create/update lifecycle definitions frequently

### Arguments For Specs for Lifecycle Definitions (Future Consideration)

1. **Consistency** - All metadata files could have specs
2. **Validation** - More rigorous validation (enum values, pattern matching)
3. **Documentation** - Spec fields provide better documentation (purpose, validation rules)
4. **Tooling** - Could generate forms/editors from specs
5. **Change tracking** - Spec-based changes could be tracked/audited

### Recommendation

**Keep current design (no specs for lifecycle definitions)** unless:

- Lifecycle definitions become user-editable (not just developer-editable)
- Validation requirements become more complex
- Tooling needs (editors, validators) require spec-based generation

## Related Concepts

### `lifecycle_ref` Field in Object Specs

Some object specs have a `lifecycle_ref` field (e.g., in `extensible_object.yaml`):

```yaml
lifecycle_ref:
  type: string
  semantic_type: reference
  validation:
    pattern: ^docs/architecture/_internal/lifecycles/.*\.yaml$
    required: false
  purpose: Explicit reference to lifecycle definition file (optional - defaults to naming convention)
```

This field allows objects to **reference** a lifecycle definition file (override the default naming convention), but it doesn't make lifecycle definitions into objects.

### Graph Backend Lifecycle Objects

In the graph backend, lifecycle definitions **might** be stored as objects (see `pkg/zqkcli/get.go` and `list.go` which check for `kind: lifecycle`). This is an implementation detail for graph storage, but lifecycle definitions are still primarily file-based configuration.

## Design Direction (Preferred Implementation)

**Lifecycle definitions are internal objects** with built-in immutable defaults and overridable system objects (see `LIFECYCLE_AS_OBJECTS_DESIGN.md` for full design):

- **Built-in immutable defaults** (generated from lifecycle builders)
- **Overridable internal objects** (managed via CLI like other internal objects)
- **Spec-based validation** (lifecycle.yaml spec)
- **Backward compatibility** (file-based loading continues to work during migration)

This aligns with the existing built-in object pattern and makes lifecycle definitions first-class system objects while maintaining defaults.

## Summary (Current Implementation)

- **Lifecycle definitions are currently configuration files, not objects**
- **They don't have object specs** - validated by Go struct, not spec files
- **They're updated by direct file editing** - no CLI object management
- **They're system metadata** - define validation rules, not user data
- **Future direction:** Transition to internal objects with built-in defaults (see design doc)