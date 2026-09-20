package migration

import (
	"time"
)

// Spec represents a migration specification
type Spec struct {
	SchemaVersion         string           `yaml:"schema_version"`
	ID                    string           `yaml:"id"`
	Name                  string           `yaml:"name"`
	Description           string           `yaml:"description,omitempty"`
	From                  StateDescription `yaml:"from"`
	To                    StateDescription `yaml:"to"`
	Prerequisites         []Prerequisite   `yaml:"prerequisites,omitempty"`
	Steps                 []Step           `yaml:"steps"`
	Rollback              *RollbackConfig  `yaml:"rollback,omitempty"`
	Validation            []ValidationRule `yaml:"validation,omitempty"`
	Options               Options          `yaml:"options,omitempty"`
	SnapshotCompatible    bool             `yaml:"snapshot_compatible,omitempty"`
	PreMigrationSnapshot  *SnapshotConfig  `yaml:"pre_migration_snapshot,omitempty"`
	PostMigrationSnapshot *SnapshotConfig  `yaml:"post_migration_snapshot,omitempty"`
	Tracking              *TrackingConfig  `yaml:"tracking,omitempty"`
	Coherence             *CoherenceConfig `yaml:"coherence,omitempty"`
}

// StateDescription describes a system state
type StateDescription struct {
	State       string `yaml:"state"`
	Description string `yaml:"description,omitempty"`
}

// Prerequisite defines a prerequisite condition
type Prerequisite struct {
	Type            string `yaml:"type"` // kind, directory, config
	Kind            string `yaml:"kind,omitempty"`
	Registered      bool   `yaml:"registered,omitempty"`
	InKindMapper    bool   `yaml:"in_kind_mapper,omitempty"`
	InOnDemandKinds bool   `yaml:"in_on_demand_kinds,omitempty"`
	Directory       string `yaml:"directory,omitempty"`
	Exists          *bool  `yaml:"exists,omitempty"` // nil = don't check, true = must exist, false = must not exist
	OnDemand        bool   `yaml:"on_demand,omitempty"`
}

// Step represents a migration step
type Step struct {
	ID          string              `yaml:"id"`
	Type        string              `yaml:"type"` // scan_files, transform, create_objects, etc.
	Description string              `yaml:"description,omitempty"`
	DependsOn   []string            `yaml:"depends_on,omitempty"`
	ForEach     string              `yaml:"for_each,omitempty"` // Reference to previous step
	Config      map[string]any      `yaml:"config,omitempty"`
	Atomic      bool                `yaml:"atomic,omitempty"`
	Checkpoint  *CheckpointConfig   `yaml:"checkpoint,omitempty"`
	Tracking    *StepTrackingConfig `yaml:"tracking,omitempty"`
}

// CheckpointConfig defines checkpoint behavior
type CheckpointConfig struct {
	Interval      int  `yaml:"interval,omitempty"`       // Create checkpoint every N objects
	Snapshot      bool `yaml:"snapshot,omitempty"`       // Create snapshot at checkpoint
	RollbackPoint bool `yaml:"rollback_point,omitempty"` // Can rollback to checkpoint
}

// StepTrackingConfig defines step-level tracking
type StepTrackingConfig struct {
	LocationMapping bool           `yaml:"location_mapping,omitempty"`
	FieldMapping    bool           `yaml:"field_mapping,omitempty"`
	Mappings        map[string]any `yaml:"mappings,omitempty"`
}

// RollbackConfig defines rollback strategy
type RollbackConfig struct {
	Strategy string                  `yaml:"strategy,omitempty"` // snapshot_restore, step_by_step
	Fallback string                  `yaml:"fallback,omitempty"`
	Snapshot *SnapshotRollbackConfig `yaml:"snapshot,omitempty"`
	Steps    []Step                  `yaml:"steps,omitempty"`
}

// SnapshotRollbackConfig defines snapshot-based rollback
type SnapshotRollbackConfig struct {
	Tag                   string `yaml:"tag"`
	ApplyLocationMappings bool   `yaml:"apply_location_mappings,omitempty"`
	ApplyFieldMappings    bool   `yaml:"apply_field_mappings,omitempty"`
}

// ValidationRule defines a validation check
type ValidationRule struct {
	Type        string         `yaml:"type"`
	Description string         `yaml:"description,omitempty"`
	Config      map[string]any `yaml:"config,omitempty"`
}

// Options defines migration options
type Options struct {
	DryRun             bool `yaml:"dry_run,omitempty"`
	Force              bool `yaml:"force,omitempty"`
	BatchSize          int  `yaml:"batch_size,omitempty"`
	ContinueOnError    bool `yaml:"continue_on_error,omitempty"`
	CheckpointInterval int  `yaml:"checkpoint_interval,omitempty"`
}

// SnapshotConfig defines snapshot creation configuration
type SnapshotConfig struct {
	Required        bool     `yaml:"required,omitempty"`
	AutoCreate      bool     `yaml:"auto_create,omitempty"`
	Tags            []string `yaml:"tags,omitempty"`
	ValidateRestore bool     `yaml:"validate_restore,omitempty"`
}

// TrackingConfig defines tracking configuration
type TrackingConfig struct {
	ObjectMapping bool `yaml:"object_mapping,omitempty"`
	FieldMapping  bool `yaml:"field_mapping,omitempty"`
	StateChanges  bool `yaml:"state_changes,omitempty"`
}

// CoherenceConfig defines coherence requirements
type CoherenceConfig struct {
	AtomicSteps           bool `yaml:"atomic_steps,omitempty"`
	CheckpointInterval    int  `yaml:"checkpoint_interval,omitempty"`
	ValidateAfterEachStep bool `yaml:"validate_after_each_step,omitempty"`
}

// StepResult represents the result of executing a step
type StepResult struct {
	StepID   string
	Success  bool
	Output   map[string]any
	Error    error
	Metadata map[string]any
}

// ExecutionResult represents the result of executing a migration
type ExecutionResult struct {
	MigrationID      string
	Status           string
	StepsCompleted   int
	StepsTotal       int
	ObjectsCreated   int
	ObjectsUpdated   int
	ObjectsDeleted   int
	Errors           []error
	PreSnapshotID    string
	PostSnapshotID   string
	LocationMappings map[string]string
	FieldMappings    map[string]map[string]string
	StepResults      []StepResult
	StartedAt        time.Time
	CompletedAt      time.Time
	Duration         time.Duration
}
