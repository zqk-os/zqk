# Guided Spec Management v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-25  
**Status**: Design  
**Related**: Spec Persistence Gap Analysis, Graph Backend Architecture

## Overview

Spec management should be **guided and interactive**, leveraging semantic linking to provide discoverability, validation, and context-aware assistance. Users should be able to discover valid options, understand inheritance, and be guided through spec creation/updates without needing to know the schema upfront.

## Core Principles

1. **Discoverability First**: Use semantic linking to discover available kinds, fields, and valid values
2. **Guided Workflows**: Interactive prompts guide users through spec operations
3. **Context-Aware**: Show relevant options based on role, current state, and relationships
4. **Validation During Input**: Validate as user types, not just after-the-fact
5. **Inheritance Visualization**: Show what gets inherited and from where

## User Workflows

### 1. Spec Update (All Users)

```bash
$ zqk spec update
Available specs (based on your role):
  [1] criteria
  [2] requirement
  [3] backlog_item
  [4] milestone
  ...
Select spec to update: [1]

Updating spec: criteria
Available fields:
  [1] category (enum: functional, non-functional, ...)
  [2] validation_method (enum: manual_check, automated_test, ...)
  [3] status (enum: not_started, in_progress, ...)
  ...
Select field to update: [1]

Field: category
Current value: required=false
Available operations:
  [1] Set required=true
  [2] Change enum values
  [3] Update description
  [4] View inheritance chain
Select operation: [1]

✅ Updated: category.validation.required = true

Affected instances: 190 criteria objects
  - 0 currently missing category (will now fail validation)
  - 190 have valid category values

Continue? [y/n]
```

### 2. Spec Creation (Admin Only)

```bash
$ zqk spec create
Creating new object specification...

Step 1: Basic Information
  Ontology name: [new_object_type]
  Description: [Brief description of this object type]
  
Step 2: Inheritance
  Extends (parent spec):
    Available parents:
      [1] base_object (most common)
      [2] auditable (adds created_at, updated_at)
      [3] extensible_object (adds extension points)
      [4] custom: [enter ontology name]
    Select: [2]
  
  Inheritance preview:
    ✓ created_at (from auditable)
    ✓ updated_at (from auditable)
    ✓ created_by (from auditable)
    ✓ updated_by (from auditable)
    ✓ id (from base_object)
    ✓ kind (from base_object)
    ...
  
Step 3: Define Fields
  Add field? [y/n]: y
  
  Field name: [priority]
  Field type:
    [1] string
    [2] enum
    [3] list
    [4] object
    [5] number
    Select: [2]
  
  Enum values (comma-separated): [critical, high, medium, low]
  
  Validation:
    Required? [y/n]: n
    Pattern (optional): []
    Min length (optional): []
    Max length (optional): []
  
  Checklist (field vetting):
    Purpose: [Relative urgency for prioritization]
    System usage: [planning, filtering]
    Criticality: [composition, association, identifier]
    Storage role: [structural, runtime_delta]  # structural = part of canonical definition; runtime_delta = high-churn/ephemeral (status, timestamps, counters) stored via deltas instead of full CAS rewrites
    ...
  
  Add another field? [y/n]: n
  
Step 4: Traits
  Available traits:
    [✓] listable
    [✓] readable
    [✓] writable
    [ ] searchable
    ...
  Select traits: [space to toggle, enter to confirm]
  
Step 5: Review
  Specification preview:
    ontology: new_object_type
    extends: auditable
    fields:
      priority:
        type: enum
        validation:
          enum: [critical, high, medium, low]
        ...
  
  Create spec? [y/n]: y
  
✅ Created spec: new_object_type
📝 Location: .zqk/specs/objects/new_object_type.yaml
🔗 Graph node: ObjectSpec:new_object_type (if using graph backend)
```

### 3. Field Discovery (Interactive)

```bash
$ zqk spec update criteria --field category --help
Field: category

Current definition:
  type: enum
  validation:
    required: true
    enum: [functional, non-functional, acceptance, test, performance, security, compliance]
  
Inheritance:
  ✓ Defined in: criteria.yaml (not inherited)
  
Valid values:
  - functional: Criteria that validate functional requirements
  - non-functional: Criteria that validate non-functional requirements
  - acceptance: Criteria that define acceptance conditions
  - test: Criteria that validate test-related conditions
  - performance: Criteria that validate performance characteristics
  - security: Criteria that validate security properties
  - compliance: Criteria that validate regulatory compliance
  
Related fields:
  - validation_method (often used together)
  - status (lifecycle state)
  
Usage examples:
  - 171 criteria use: functional
  - 12 criteria use: non-functional
  - 5 criteria use: test
  
Semantic links:
  - Referenced by: 190 criteria objects
  - Used in queries: "show items where non-functional criteria not met"
  - Linked to: criteria-categories-v1.0.md (definitions)
```

## Semantic Linking Benefits

### 1. Discover Available Kinds

```go
// Query graph to discover available specs based on role
func DiscoverAvailableSpecs(role string) []string {
    // Query: What specs can this role access?
    query := `
        MATCH (role:Role {id: $role})-[:CAN_ACCESS]->(spec:ObjectSpec)
        RETURN spec.ontology
    `
    // Or from file system if graph unavailable
    // Scan .zqk/specs/objects/*.yaml
}
```

### 2. Discover Valid Values

```go
// Query graph to discover valid enum values
func DiscoverValidValues(spec, field string) []string {
    // Load spec (from graph or file)
    spec := loader.LoadSpec(spec)
    fieldDef := spec.ResolvedFields[field]
    
    // Extract enum values
    if enum, ok := fieldDef["validation"]["enum"]; ok {
        return enum
    }
    
    // Or query existing instances for examples
    query := `
        MATCH (obj {kind: $spec})
        WHERE obj[$field] IS NOT NULL
        RETURN DISTINCT obj[$field] as value
        ORDER BY value
        LIMIT 20
    `
}
```

### 3. Discover Inheritance Chain

```go
// Visualize inheritance chain
func ShowInheritanceChain(ontology string) {
    spec := loader.LoadSpecWithInheritance(ontology)
    
    // Show chain
    chain := []string{}
    current := spec
    for current.Extends != "" {
        chain = append(chain, current.Ontology)
        current = loader.LoadSpec(current.Extends)
    }
    
    // Display: new_object_type → auditable → base_object
}
```

### 4. Discover Related Fields

```go
// Find fields that are often used together
func DiscoverRelatedFields(spec, field string) []string {
    query := `
        MATCH (obj {kind: $spec})
        WHERE obj[$field] IS NOT NULL
        WITH obj, keys(obj) as fields
        UNWIND fields as field
        WHERE field <> $field
        RETURN field, count(*) as frequency
        ORDER BY frequency DESC
        LIMIT 5
    `
}
```

## Implementation Architecture

### 1. SpecStorageProvider Interface

```go
type SpecStorageProvider interface {
    // CRUD operations
    LoadSpec(ontology string) (*Spec, error)
    SaveSpec(spec *Spec) error
    UpdateSpec(ontology string, updates SpecUpdates) error
    DeleteSpec(ontology string, options DeleteOptions) (*DeleteImpact, error)
    
    // Discovery operations
    ListSpecs(role string) ([]string, error)
    GetSpecMetadata(ontology string) (*SpecMetadata, error)
    
    // Semantic queries
    FindSpecsByTrait(trait string) ([]string, error)
    FindSpecsExtending(parent string) ([]string, error)
    GetInheritanceChain(ontology string) ([]string, error)
}

type DeleteOptions struct {
    Cascade         bool   // Delete all instances
    UpdateReferences bool   // Update objects that reference instances
    DryRun          bool   // Show what would be deleted without deleting
    Force           bool   // Skip confirmation prompts
}

type DeleteImpact struct {
    InstancesToDelete     []string            // IDs of instances that will be deleted
    ReferencesToUpdate    []ReferenceUpdate   // Objects that reference instances
    BreakingChanges       []string            // Warnings about breaking changes
    CanRollback          bool                 // Whether deletion can be rolled back
}

type UpdateOptions struct {
    FieldAdditionPolicy FieldAdditionPolicy // How to handle new required fields
    DefaultValue        any                 // Default value for new required fields
    MigrationStrategy   MigrationStrategy   // How to migrate existing instances
    DryRun              bool                // Show impact without applying
    Force               bool                // Skip confirmation prompts
}

type FieldAdditionPolicy string

const (
    // OnlyNewInstances - New required fields only apply to instances created after update
    // Existing instances are grandfathered (no validation failure)
    FieldPolicyOnlyNewInstances FieldAdditionPolicy = "only_new_instances"
    
    // ProvideDefaults - Add default values to all existing instances automatically
    FieldPolicyProvideDefaults FieldAdditionPolicy = "provide_defaults"
    
    // ForceFailure - All existing instances must be updated manually (strict validation)
    FieldPolicyForceFailure FieldAdditionPolicy = "force_failure"
    
    // MigrationWindow - Allow grace period where instances can be updated
    // After window expires, validation fails
    FieldPolicyMigrationWindow FieldAdditionPolicy = "migration_window"
)

type MigrationStrategy string

const (
    // AutoFix - Automatically add default values to existing instances
    MigrationAutoFix MigrationStrategy = "auto_fix"
    
    // Manual - Require manual update of each instance
    MigrationManual MigrationStrategy = "manual"
    
    // Batch - Provide script/batch operation to update instances
    MigrationBatch MigrationStrategy = "batch"
    
    // Gradual - Update instances as they are accessed/modified
    MigrationGradual MigrationStrategy = "gradual"
)
```

### 2. Guided Spec Manager

```go
type GuidedSpecManager struct {
    storage SpecStorageProvider
    loader  *SpecLoader
    validator *SpecValidator
    role    string
}

func (gsm *GuidedSpecManager) InteractiveUpdate(ontology string, options UpdateOptions) error {
    // 1. Load spec
    spec, err := gsm.storage.LoadSpec(ontology)
    
    // 2. Show available fields with context
    fields := gsm.discoverFields(spec)
    
    // 3. Interactive field selection
    field := gsm.promptFieldSelection(fields)
    
    // 4. Show field definition and valid values
    gsm.showFieldContext(field)
    
    // 5. Prompt for update
    update := gsm.promptUpdate(field)
    
    // 6. Check if adding new required field
    isNewRequiredField := gsm.isNewRequiredField(spec, field, update)
    if isNewRequiredField {
        // Prompt for field addition policy
        policy := gsm.promptFieldAdditionPolicy()
        options.FieldAdditionPolicy = policy
        
        if policy == FieldPolicyProvideDefaults {
            // Prompt for default value
            options.DefaultValue = gsm.promptDefaultValue(field)
        }
        
        if policy == FieldPolicyMigrationWindow {
            // Prompt for migration window duration
            options.MigrationWindow = gsm.promptMigrationWindow()
        }
    }
    
    // 7. Validate update
    if err := gsm.validateUpdate(spec, field, update); err != nil {
        return err
    }
    
    // 8. Show impact analysis
    impact := gsm.analyzeImpact(ontology, field, update, options)
    gsm.showImpact(impact)
    
    // 9. Confirm and apply
    if gsm.confirmUpdate() {
        return gsm.storage.UpdateSpec(ontology, update, options)
    }
}

func (gsm *GuidedSpecManager) InteractiveCreate() error {
    // Guided multi-step creation process
    // Uses semantic linking to suggest valid options
}

func (gsm *GuidedSpecManager) InteractiveDelete(ontology string, options DeleteOptions) error {
    // 1. Load spec to verify it exists
    spec, err := gsm.storage.LoadSpec(ontology)
    if err != nil {
        return err
    }
    
    // 2. Safety checks
    if err := gsm.validateDeletionAllowed(ontology, spec); err != nil {
        return err
    }
    
    // 3. Find all instances
    instances, err := gsm.findAllInstances(ontology)
    if err != nil {
        return err
    }
    
    // 4. Find all references to instances
    references, err := gsm.findReferencesToInstances(instances)
    if err != nil {
        return err
    }
    
    // 5. Check for child specs
    childSpecs, err := gsm.findChildSpecs(ontology)
    if err != nil {
        return err
    }
    
    // 6. Build impact analysis
    impact := &DeleteImpact{
        InstancesToDelete:  instances,
        ReferencesToUpdate: references,
        BreakingChanges:    gsm.analyzeBreakingChanges(instances, references, childSpecs),
        CanRollback:        true, // If using transactions
    }
    
    // 7. Show impact (unless --force)
    if !options.Force {
        gsm.showDeleteImpact(impact, childSpecs)
        if !gsm.confirmDeletion() {
            return ErrDeletionCancelled
        }
    }
    
    // 8. Execute in transaction (if supported)
    if options.DryRun {
        return nil // Just show impact
    }
    
    return gsm.storage.DeleteSpec(ontology, options, impact)
}

func (gsm *GuidedSpecManager) validateDeletionAllowed(ontology string, spec *Spec) error {
    // Prevent deletion of base specs
    protectedSpecs := []string{"base_object", "auditable", "extensible_object"}
    for _, protected := range protectedSpecs {
        if ontology == protected {
            return fmt.Errorf("cannot delete protected spec: %s", ontology)
        }
    }
    
    // Check if other specs extend this one
    childSpecs, err := gsm.findChildSpecs(ontology)
    if err != nil {
        return err
    }
    if len(childSpecs) > 0 {
        return fmt.Errorf("cannot delete spec %s: %d child specs extend it: %v", 
            ontology, len(childSpecs), childSpecs)
    }
    
    return nil
}

func (gsm *GuidedSpecManager) findAllInstances(ontology string) ([]string, error) {
    // Query graph or scan files
    if gsm.usingGraph() {
        query := `
            MATCH (obj {kind: $ontology})
            RETURN obj.id as id
            ORDER BY id
        `
        return gsm.graph.Query(query, map[string]any{"ontology": ontology})
    }
    
    // File-based: scan directory
    kindDir := gsm.getKindDirectory(ontology)
    return gsm.scanInstances(kindDir)
}

func (gsm *GuidedSpecManager) findReferencesToInstances(instanceIDs []string) ([]ReferenceUpdate, error) {
    var updates []ReferenceUpdate
    
    // Find all objects that reference these instances
    // Query for fields ending in _refs that contain any of the instance IDs
    for _, id := range instanceIDs {
        refs := gsm.findObjectsReferencing(id)
        for _, ref := range refs {
            updates = append(updates, ReferenceUpdate{
                ObjectID:    ref.ObjectID,
                ObjectKind:  ref.ObjectKind,
                FieldName:   ref.FieldName,
                RemoveValue: id,
            })
        }
    }
    
    return updates, nil
}

func (gsm *GuidedSpecManager) findObjectsReferencing(instanceID string) []ObjectReference {
    var refs []ObjectReference
    
    // Graph query
    if gsm.usingGraph() {
        query := `
            MATCH (obj)-[r]->(target {id: $instanceID})
            WHERE type(r) ENDS WITH '_ref' OR type(r) = 'REFERENCES'
            RETURN obj.id as object_id,
                   obj.kind as object_kind,
                   type(r) as field_name
        `
        // Execute query and collect results
        return gsm.graph.QueryReferences(query, instanceID)
    }
    
    // File-based: scan all object directories
    return gsm.scanForReferences(instanceID)
}

func (gsm *GuidedSpecManager) findChildSpecs(ontology string) ([]string, error) {
    // Find specs that extend this ontology
    if gsm.usingGraph() {
        query := `
            MATCH (spec:ObjectSpec {extends: $ontology})
            RETURN spec.ontology as ontology
        `
        return gsm.graph.Query(query, map[string]any{"ontology": ontology})
    }
    
    // File-based: scan spec directory
    return gsm.scanChildSpecs(ontology)
}

func (gsm *GuidedSpecManager) updateReferences(updates []ReferenceUpdate) error {
    for _, update := range updates {
        // Load object
        obj, err := gsm.loadObject(update.ObjectKind, update.ObjectID)
        if err != nil {
            return err
        }
        
        // Remove reference from field
        refs := obj[update.FieldName].([]string)
        newRefs := []string{}
        for _, ref := range refs {
            if ref != update.RemoveValue {
                newRefs = append(newRefs, ref)
            }
        }
        obj[update.FieldName] = newRefs
        
        // Save updated object
        if err := gsm.saveObject(obj); err != nil {
            return err
        }
    }
    
    return nil
}
```

### 3. Impact Analysis

```go
type ImpactAnalysis struct {
    AffectedInstances int
    ValidationErrors  []ValidationError
    BreakingChanges   []string
    Warnings          []string
    MigrationRequired bool
    InstancesNeedingUpdate []string
}

func (gsm *GuidedSpecManager) AnalyzeImpact(
    ontology, field string, 
    update SpecUpdate,
    options UpdateOptions,
) *ImpactAnalysis {
    // Query existing instances
    instances := gsm.findInstances(ontology)
    
    // Simulate validation with new spec
    analysis := &ImpactAnalysis{
        AffectedInstances: len(instances),
    }
    
    // Check if this is adding a new required field
    isNewRequired := gsm.isNewRequiredField(spec, field, update)
    
    if isNewRequired {
        // Analyze impact based on policy
        switch options.FieldAdditionPolicy {
        case FieldPolicyOnlyNewInstances:
            // No impact on existing instances
            analysis.Warnings = append(analysis.Warnings, 
                "New required field will only apply to instances created after this update")
            
        case FieldPolicyProvideDefaults:
            // All instances will get default value
            analysis.MigrationRequired = true
            analysis.Warnings = append(analysis.Warnings,
                fmt.Sprintf("%d instances will receive default value: %v", 
                    len(instances), options.DefaultValue))
            
        case FieldPolicyForceFailure:
            // All existing instances will fail validation
            for _, instance := range instances {
                if !gsm.instanceHasField(instance, field) {
                    analysis.ValidationErrors = append(analysis.ValidationErrors, ValidationError{
                        Field:   field,
                        Message: fmt.Sprintf("Instance %s missing required field", instance.ID),
                    })
                    analysis.InstancesNeedingUpdate = append(analysis.InstancesNeedingUpdate, instance.ID)
                }
            }
            analysis.BreakingChanges = append(analysis.BreakingChanges,
                fmt.Sprintf("%d instances will fail validation until updated", len(analysis.InstancesNeedingUpdate)))
            
        case FieldPolicyMigrationWindow:
            // Grace period - instances can be updated gradually
            analysis.MigrationRequired = true
            analysis.Warnings = append(analysis.Warnings,
                fmt.Sprintf("Migration window: %v. After this, validation will fail.", 
                    options.MigrationWindow))
        }
    } else {
        // Existing field update - check validation
        for _, instance := range instances {
            if err := gsm.validateWithUpdatedSpec(instance, update); err != nil {
                analysis.ValidationErrors = append(analysis.ValidationErrors, err)
                analysis.InstancesNeedingUpdate = append(analysis.InstancesNeedingUpdate, instance.ID)
            }
        }
    }
    
    return analysis
}
```

## Role-Based Access

### Role Definitions

```yaml
# role.yaml spec
fields:
  permissions:
    type: list
    semantic_type: reference
    # References to permission objects
```

### Permission Model

- **Admin**: Can create, update, delete any spec
- **Developer**: Can update existing specs (with approval workflow)
- **Viewer**: Can view specs but not modify

### Discovery Based on Role

```go
func (gsm *GuidedSpecManager) ListAvailableSpecs() ([]string, error) {
    if gsm.role == "admin" {
        // Admins see all specs
        return gsm.storage.ListAllSpecs()
    }
    
    // Others see specs they have permission for
    return gsm.storage.ListSpecs(gsm.role)
}
```

## CLI Commands

### Spec Update (Interactive)

```bash
zqk spec update [ontology]
  --field <field>          # Specific field to update
  --set <path>=<value>     # Direct update (non-interactive)
  --interactive            # Guided mode (default)
  --dry-run                # Show what would change
  --analyze-impact         # Show impact on existing instances
  --field-policy <policy>  # Field addition policy (only_new, defaults, force_failure, migration_window)
  --default-value <value>  # Default value for new required fields
  --migration-strategy <strategy>  # Migration strategy (auto_fix, batch, gradual, manual)
  --migration-window <duration>    # Grace period for migration_window policy
```

### Spec Create (Guided)

```bash
zqk spec create
  --from <template>        # Start from template
  --extends <parent>       # Specify parent spec
  --interactive            # Guided mode (default)
  --non-interactive        # Use flags only
```

### Spec Delete (Cascade with Safety)

```bash
zqk spec delete <ontology>
  --cascade                # Delete all instances (required)
  --update-references      # Update objects referencing instances
  --dry-run                # Show impact without deleting
  --force                  # Skip confirmation (dangerous)
  --interactive            # Guided mode with impact analysis
```

### Spec Discover

```bash
zqk spec discover
  --field <field>         # Discover valid values for field
  --trait <trait>         # Find specs with trait
  --extends <parent>      # Find specs extending parent
  --related <ontology>    # Find related specs
```

## Benefits

1. **No Schema Knowledge Required**: Users don't need to know YAML structure
2. **Validation During Input**: Catch errors before saving
3. **Impact Awareness**: See what breaks before making changes
4. **Inheritance Understanding**: Visualize what gets inherited
5. **Semantic Discovery**: Find related fields, valid values, usage examples
6. **Role-Based**: Only see/modify what you have permission for
7. **Consistent Interface**: Same commands work for file and graph backends

## Example: Making Category Required

### Current Way (Manual)
```bash
# Edit file, hope you get syntax right
vim .zqk/specs/objects/criteria.yaml
# Change: required: false → required: true
# Save, then run validation to see if it breaks anything
```

### Guided Way (Interactive)
```bash
$ zqk spec update criteria
Select field: category
Current: required=false
Set to required? [y/n]: y

Impact Analysis:
  - 190 criteria instances found
  - 0 missing category field (all have valid values)
  - Validation will now enforce category requirement
  
Apply change? [y/n]: y
✅ Updated: category.validation.required = true
📝 Updated: .zqk/specs/objects/criteria.yaml
🔗 Updated: ObjectSpec:criteria (graph node)
```

## Example: Deleting a Spec (Cascade Delete)

### Guided Delete Workflow

```bash
$ zqk spec delete old_object_type
⚠️  WARNING: Deleting a spec will cascade delete all instances!

Impact Analysis:
  Instances to delete:
    - OLD-001 (old_object_type)
    - OLD-002 (old_object_type)
    - OLD-003 (old_object_type)
    Total: 3 instances
  
  References to update:
    - REQ-123: old_object_refs contains OLD-001 (will be removed)
    - BLI-456: old_object_refs contains OLD-002 (will be removed)
    - MIL-789: old_object_refs contains OLD-003 (will be removed)
    Total: 3 objects need reference cleanup
  
  Breaking changes:
    - 1 requirement will lose old_object reference
    - 1 backlog item will lose old_object reference
    - 1 milestone will lose old_object reference
  
  Rollback available: Yes (spec and instances can be restored)

Options:
  [1] Delete spec + all instances + update references
  [2] Delete spec + all instances (leave broken references)
  [3] Cancel

Select option: [1]

Confirm deletion? This cannot be easily undone. [yes/no]: yes

✅ Deleted spec: old_object_type
✅ Deleted 3 instances: OLD-001, OLD-002, OLD-003
✅ Updated 3 references in: REQ-123, BLI-456, MIL-789
📝 Transaction ID: tx-abc123 (for rollback)
```

### Cascade Delete Implementation

```go
func (gsm *GuidedSpecManager) DeleteSpec(ontology string, options DeleteOptions) error {
    // 1. Load spec to verify it exists
    spec, err := gsm.storage.LoadSpec(ontology)
    if err != nil {
        return err
    }
    
    // 2. Find all instances
    instances, err := gsm.findAllInstances(ontology)
    if err != nil {
        return err
    }
    
    // 3. Find all references to instances
    references, err := gsm.findReferencesToInstances(instances)
    if err != nil {
        return err
    }
    
    // 4. Build impact analysis
    impact := &DeleteImpact{
        InstancesToDelete:  instances,
        ReferencesToUpdate: references,
        BreakingChanges:    gsm.analyzeBreakingChanges(instances, references),
        CanRollback:        true, // If using transactions
    }
    
    // 5. Show impact (unless --force)
    if !options.Force {
        gsm.showDeleteImpact(impact)
        if !gsm.confirmDeletion() {
            return ErrDeletionCancelled
        }
    }
    
    // 6. Execute in transaction (if supported)
    if options.DryRun {
        return nil // Just show impact
    }
    
    return gsm.storage.DeleteSpec(ontology, options, impact)
}

func (gsm *GuidedSpecManager) findAllInstances(ontology string) ([]string, error) {
    // Query graph or scan files
    if gsm.usingGraph() {
        query := `
            MATCH (obj {kind: $ontology})
            RETURN obj.id as id
        `
        return gsm.graph.Query(query, map[string]any{"ontology": ontology})
    }
    
    // File-based: scan directory
    kindDir := gsm.getKindDirectory(ontology)
    return gsm.scanInstances(kindDir)
}

func (gsm *GuidedSpecManager) findReferencesToInstances(instanceIDs []string) ([]ReferenceUpdate, error) {
    var updates []ReferenceUpdate
    
    // Find all objects that reference these instances
    // Query for fields ending in _refs that contain any of the instance IDs
    for _, id := range instanceIDs {
        refs := gsm.findObjectsReferencing(id)
        for _, ref := range refs {
            updates = append(updates, ReferenceUpdate{
                ObjectID:    ref.ObjectID,
                ObjectKind:  ref.ObjectKind,
                FieldName:   ref.FieldName,
                RemoveValue: id,
            })
        }
    }
    
    return updates, nil
}
```

### Reference Update Strategy

```go
type ReferenceUpdate struct {
    ObjectID    string // Object that has the reference
    ObjectKind  string // Kind of object
    FieldName   string // Field containing the reference (e.g., "criteria_refs")
    RemoveValue string // Value to remove from the field
}

func (gsm *GuidedSpecManager) updateReferences(updates []ReferenceUpdate) error {
    for _, update := range updates {
        // Load object
        obj, err := gsm.loadObject(update.ObjectKind, update.ObjectID)
        if err != nil {
            return err
        }
        
        // Remove reference from field
        refs := obj[update.FieldName].([]string)
        newRefs := []string{}
        for _, ref := range refs {
            if ref != update.RemoveValue {
                newRefs = append(newRefs, ref)
            }
        }
        obj[update.FieldName] = newRefs
        
        // Save updated object
        if err := gsm.saveObject(obj); err != nil {
            return err
        }
    }
    
    return nil
}
```

### Safety Checks

1. **Prevent Deleting Base Specs**: Don't allow deletion of `base_object`, `auditable`, etc.
2. **Check for Child Specs**: Warn if other specs extend this one
3. **Transaction Support**: Wrap deletion in transaction for rollback
4. **Backup Before Delete**: Create backup/archive before deletion
5. **Reference Validation**: Ensure all references are updated before completing deletion

## Field Addition Policies

### Policy Selection

When adding a new required field to an existing spec, the system must handle existing instances. Four policies are available:

#### 1. Only New Instances (Grandfathering)

**Policy**: `only_new_instances`

**Behavior**:
- Existing instances: Exempt from new field requirement (grandfathered)
- New instances: Must provide the field (strict validation)
- Validation: Only new instances are validated for the field

**Implementation**:
```go
// Track spec version when field was added
specVersion := spec.SchemaVersion
instanceCreatedAt := instance["created_at"]

// Only validate if instance created after spec update
if instanceCreatedAt > specVersion {
    // Validate new required field
} else {
    // Skip validation (grandfathered)
}
```

**Use Cases**:
- Adding optional-but-recommended fields
- Gradual adoption of new requirements
- Legacy data compatibility

#### 2. Provide Defaults (Auto-Fix)

**Policy**: `provide_defaults`

**Behavior**:
- All existing instances: Automatically receive default value
- New instances: Must provide value (or use default)
- Migration: Automatic via `MigrationAutoFix` strategy

**Implementation**:
```go
// Update all instances with default value
for _, instance := range instances {
    if _, exists := instance[fieldName]; !exists {
        instance[fieldName] = defaultValue
        saveInstance(instance)
    }
}
```

**Use Cases**:
- Safe default exists (e.g., `priority: "medium"`)
- Want immediate consistency
- Can automatically migrate without data loss

#### 3. Force Failure (Strict)

**Policy**: `force_failure`

**Behavior**:
- All existing instances: Fail validation until manually updated
- New instances: Must provide value
- Migration: Manual update required

**Implementation**:
```go
// Validation fails for missing field
if _, exists := instance[fieldName]; !exists {
    return ValidationError{
        Field: fieldName,
        Message: "Required field missing",
    }
}
```

**Use Cases**:
- No safe default exists
- Want explicit review of each instance
- Data quality is critical
- Field is context-dependent

#### 4. Migration Window (Grace Period)

**Policy**: `migration_window`

**Behavior**:
- Window period: Instances can be updated gradually (validation passes)
- After window: Validation fails for missing field
- Migration: Gradual or batch update during window

**Implementation**:
```go
// Check if within migration window
windowEnd := spec.FieldAddedAt.Add(options.MigrationWindow)
if time.Now().Before(windowEnd) {
    // Within window - validation passes (with warning)
    if !hasField {
        logWarning("Field will be required after migration window")
        return nil // Pass validation
    }
} else {
    // Window expired - strict validation
    if !hasField {
        return ValidationError{...}
    }
}
```

**Use Cases**:
- Need time to update instances
- Want to enforce eventually
- Gradual migration preferred
- Team coordination required

### Policy Selection Guidance

```bash
$ zqk spec update criteria --add-field priority --required
Field addition policy:
  [1] only_new_instances - Grandfather existing (recommended for optional fields)
  [2] provide_defaults - Auto-fix with default (recommended if safe default exists)
  [3] force_failure - Strict validation (recommended for critical fields)
  [4] migration_window - Grace period (recommended for large migrations)
  
Select policy: [2]

Default value: [medium]
Migration strategy: [auto_fix]
```

## Related

- [Spec Persistence Gap Analysis](./spec-persistence-gap-analysis.md)
- [Graph Spec Storage](./graph-spec-storage-v1.0.md)
- [Hash Registry Design](./hash-registry-design-v1.0.md) - Similar pattern for unified persistence

