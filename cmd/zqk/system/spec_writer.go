package system

import (
	"fmt"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/yaml"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SpecWriter handles writing and updating object specifications with field versioning
type SpecWriter struct {
	specsDir         string
	specLoader       *objects.SpecLoader
	logger           logging.Logger
	currentUser      string
	metricsCollector *storage.SpecMetricsCollector
	writer           *yaml.YAMLWriter[*objects.Spec]
}

// NewSpecWriter creates a new spec writer
func NewSpecWriter(projectRoot, currentUser string, storageProvider storage.ObjectStorageProvider) (*SpecWriter, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(specsDir); fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("specs directory not found: %s", specsDir)
	}

	return &SpecWriter{
		specsDir:         specsDir,
		specLoader:       objects.GetGlobalSpecLoader(),
		logger:           logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		currentUser:      currentUser,
		metricsCollector: storage.NewSpecMetricsCollector(storageProvider),
		writer:           yaml.NewYAMLWriter[*objects.Spec](),
	}, nil
}

// FieldOperation represents an operation on a field
type FieldOperation struct {
	Operation      string         // FieldOp* or CLI synonym (define/add); use ResolvedOperation / ResolveSpecFieldOperation
	FieldName      string         // Name of the field
	FieldDef       map[string]any // New field definition (for create/modify)
	BreakingChange bool           // Whether this is a breaking change
	Reason         string         // Reason for the change
	MigrationNotes string         // Migration guidance
	ReplacedBy     string         // Field that replaces this (for deprecate/delete)
}

// WriteSpec writes a spec to disk with field versioning
// Uses YAMLWriter from specbuilder for consistent formatting
func (sw *SpecWriter) WriteSpec(ontology string, spec *objects.Spec) error {
	specPath := filepath.Join(sw.specsDir, ontology+".yaml")

	// Use YAMLWriter from specbuilder for consistent formatting
	if err := sw.writer.WriteToFile(spec, specPath); err != nil {
		return errfmt.Newf("failed to write spec file").Wrap(err)
	}

	logging.Fluent(sw.logger).Info("Spec written").
		String("ontology", ontology).
		Path(specPath).
		Log()
	return nil
}

// ApplyFieldOperations applies field operations to a spec. Each FieldOperation.Operation may be a
// CLI synonym (define/add) or canonical FieldOp*; dispatch uses ResolvedOperation.
func (sw *SpecWriter) ApplyFieldOperations(ontology string, operations []FieldOperation) error {
	startTime := time.Now()

	// Load current spec
	spec, err := sw.specLoader.LoadSpecWithInheritance(ontology + ".yaml")
	if err != nil {
		return errfmt.Newf("failed to load spec").Wrap(err)
	}

	// Ensure Fields map exists
	if spec.Fields == nil {
		spec.Fields = make(map[string]any)
	}

	// Apply all operations
	breakingChangeCount, err := sw.applyFieldOperationsBatch(spec, operations)
	if err != nil {
		return err
	}

	// Write updated spec
	err = sw.WriteSpec(ontology, spec)
	if err != nil {
		return err
	}

	// Field-vetting validation uses objects.ValidateLoadedSpec (same as update-specs --validate).
	// Callers should run `zqk system update-specs --files <ontology>.yaml --validate` after edits,
	// or add an explicit post-write ValidateLoadedSpec gate once legacy checklist debt is cleared.

	// Record metrics
	sw.recordSpecChangeMetrics(ontology, len(operations), breakingChangeCount, time.Since(startTime))

	return nil
}

// createField creates a new field in the spec
func (sw *SpecWriter) createField(spec *objects.Spec, op *FieldOperation) error {
	if spec.Fields == nil {
		spec.Fields = make(map[string]any)
	}

	// Check if field already exists
	if _, exists := spec.Fields[op.FieldName]; exists {
		return errfmt.Errorf("field %s already exists", op.FieldName)
	}

	// Set field definition
	fieldDefMap := op.FieldDef
	if fieldDefMap == nil {
		fieldDefMap = make(map[string]any)
	}
	spec.Fields[op.FieldName] = fieldDefMap

	objects.MarkFieldCreated(fieldDefMap, sw.currentUser)

	logging.Fluent(sw.logger).Info("Field created").
		String("ontology", spec.Ontology).
		String("field", op.FieldName).
		Log()

	return nil
}

// modifyField modifies an existing field
func (sw *SpecWriter) modifyField(spec *objects.Spec, op *FieldOperation) error {
	startTime := time.Now()
	success := false
	var breakingChange bool
	var reasons []string

	defer func() {
		// Record metric asynchronously
		if sw.metricsCollector != nil {
			systemCtx := pkgctx.NewSystemContext()
			sw.metricsCollector.RecordFieldOperation(
				systemCtx,
				spec.Ontology,
				op.FieldName,
				FieldOpModify,
				breakingChange,
				time.Since(startTime),
				success,
			)

			// Also record breaking change detection if applicable
			if breakingChange && len(reasons) > 0 {
				sw.metricsCollector.RecordBreakingChangeDetection(
					systemCtx,
					spec.Ontology,
					op.FieldName,
					reasons,
				)
			}
		}
	}()

	existingFieldMap, err := sw.getFieldMap(spec, op.FieldName)
	if err != nil {
		return err
	}

	// Detect breaking changes
	breakingChange, reasons = objects.DetectBreakingChange(existingFieldMap, op.FieldDef)
	if breakingChange && !op.BreakingChange {
		logging.Fluent(sw.logger).Warn("Breaking change detected but not marked").
			String("field", op.FieldName).
			String("reasons", fmt.Sprintf("%v", reasons)).
			Log()
		// Still proceed, but log warning
	}

	// Update field definition
	for k, v := range op.FieldDef {
		existingFieldMap[k] = v
	}

	// Mark field as modified
	objects.MarkFieldModified(existingFieldMap, sw.currentUser, breakingChange || op.BreakingChange, op.Reason, op.MigrationNotes)

	success = true
	logging.Fluent(sw.logger).Info("Field modified").
		String("ontology", spec.Ontology).
		String("field", op.FieldName).
		String("breaking", fmt.Sprintf("%v", breakingChange || op.BreakingChange)).
		Log()

	return nil
}

func (sw *SpecWriter) getFieldMap(spec *objects.Spec, fieldName string) (map[string]any, error) {
	if spec.Fields == nil {
		return nil, errfmt.Errorf("spec has no fields")
	}

	field, exists := spec.Fields[fieldName]
	if !exists {
		return nil, errfmt.Errorf("field %s does not exist", fieldName)
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("field %s is not a map", fieldName)
	}

	return fieldMap, nil
}

// deprecateField deprecates a field
func (sw *SpecWriter) deprecateField(spec *objects.Spec, op *FieldOperation) error {
	fieldMap, err := sw.getFieldMap(spec, op.FieldName)
	if err != nil {
		return err
	}

	// Mark field as deprecated
	objects.MarkFieldDeprecated(fieldMap, sw.currentUser, op.ReplacedBy, op.Reason)

	logging.Fluent(sw.logger).Info("Field deprecated").
		String("ontology", spec.Ontology).
		String("field", op.FieldName).
		String("replaced_by", op.ReplacedBy).
		Log()

	return nil
}

// archiveField archives a field
func (sw *SpecWriter) archiveField(spec *objects.Spec, op *FieldOperation) error {
	fieldMap, err := sw.getFieldMap(spec, op.FieldName)
	if err != nil {
		return err
	}

	// Mark field as archived
	objects.MarkFieldArchived(fieldMap, sw.currentUser, op.Reason)

	logging.Fluent(sw.logger).Info("Field archived").
		String("ontology", spec.Ontology).
		String("field", op.FieldName).
		Log()

	return nil
}

// deleteField deletes a field (marks as deleted, keeps in spec for history)
func (sw *SpecWriter) deleteField(spec *objects.Spec, op *FieldOperation) error {
	fieldMap, err := sw.getFieldMap(spec, op.FieldName)
	if err != nil {
		return err
	}

	// Mark field as deleted (but keep in spec for history)
	objects.MarkFieldDeleted(fieldMap, sw.currentUser, op.ReplacedBy, op.Reason)

	logging.Fluent(sw.logger).Info("Field deleted").
		String("ontology", spec.Ontology).
		String("field", op.FieldName).
		String("replaced_by", op.ReplacedBy).
		Log()

	return nil
}

// GetFieldChanges compares two specs and returns a list of field changes
func (sw *SpecWriter) GetFieldChanges(oldSpec, newSpec *objects.Spec) ([]objects.FieldChange, error) {
	var changes []objects.FieldChange

	// Get all field names from both specs
	allFields := make(map[string]bool)
	if oldSpec.Fields != nil {
		for fieldName := range oldSpec.Fields {
			allFields[fieldName] = true
		}
	}
	if newSpec.Fields != nil {
		for fieldName := range newSpec.Fields {
			allFields[fieldName] = true
		}
	}

	// Check each field
	for fieldName := range allFields {
		oldField, oldExists := oldSpec.Fields[fieldName]
		newField, newExists := newSpec.Fields[fieldName]

		if !oldExists && newExists {
			// Field created
			changes = append(changes, objects.FieldChange{
				FieldName:  fieldName,
				ChangeType: objects.FieldChangeCreated,
				NewValue:   newField,
			})
		} else if oldExists && !newExists {
			// Field deleted
			oldFieldMap, _ := oldField.(map[string]any)
			versionInfo := objects.GetFieldVersionInfo(oldFieldMap)
			changes = append(changes, objects.FieldChange{
				FieldName:  fieldName,
				ChangeType: objects.FieldChangeDeleted,
				OldValue:   oldField,
				ReplacedBy: versionInfo.ReplacedBy,
			})
		} else if oldExists && newExists {
			// Field modified
			oldFieldMap, _ := oldField.(map[string]any)
			newFieldMap, _ := newField.(map[string]any)
			breakingChange, reasons := objects.DetectBreakingChange(oldFieldMap, newFieldMap)
			change := objects.FieldChange{
				FieldName:      fieldName,
				ChangeType:     objects.FieldChangeModified,
				OldValue:       oldField,
				NewValue:       newField,
				BreakingChange: breakingChange,
			}
			if breakingChange {
				change.MigrationNotes = fmt.Sprintf("Breaking changes: %v", reasons)
			}
			changes = append(changes, change)
		}
	}

	return changes, nil
}
