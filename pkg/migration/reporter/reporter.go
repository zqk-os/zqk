package reporter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MigrationReport contains migration statistics and results
type MigrationReport struct {
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
	ObjectsScanned    int
	ObjectsMigrated   int
	DocumentsCreated  int
	EntitiesCreated   int
	EdgesCreated      int
	Errors            []ReportError
	Warnings          []string
	ValidationResults *ValidationReport
}

// ReportError represents an error during migration
type ReportError struct {
	Type       string // "parse", "validation", "reference", "write"
	ObjectID   string
	ObjectType string
	Message    string
	FilePath   string
}

// ValidationReport contains validation results
type ValidationReport struct {
	UnresolvedReferences  []string
	OrphanedEntities      []string
	DuplicateIDs          []string
	InvalidEdgeTypes      []string
	MissingRequiredFields []ReportError
}

// Reporter generates migration reports
type Reporter struct {
	report *MigrationReport
}

// NewReporter creates a new reporter
func NewReporter() *Reporter {
	return &Reporter{
		report: &MigrationReport{
			StartTime: time.Now(),
			Errors:    make([]ReportError, 0),
			Warnings:  make([]string, 0),
			ValidationResults: &ValidationReport{
				UnresolvedReferences:  make([]string, 0),
				OrphanedEntities:      make([]string, 0),
				DuplicateIDs:          make([]string, 0),
				InvalidEdgeTypes:      make([]string, 0),
				MissingRequiredFields: make([]ReportError, 0),
			},
		},
	}
}

// Start begins the migration report
func (r *Reporter) Start() {
	r.report.StartTime = time.Now()
}

// Finish completes the migration report
func (r *Reporter) Finish() {
	r.report.EndTime = time.Now()
	r.report.Duration = r.report.EndTime.Sub(r.report.StartTime)
}

// RecordObjectScanned records a scanned object
func (r *Reporter) RecordObjectScanned() {
	r.report.ObjectsScanned++
}

// RecordObjectMigrated records a successfully migrated object
func (r *Reporter) RecordObjectMigrated() {
	r.report.ObjectsMigrated++
}

// RecordDocumentCreated records a created document node
func (r *Reporter) RecordDocumentCreated() {
	r.report.DocumentsCreated++
}

// RecordEntityCreated records a created entity node
func (r *Reporter) RecordEntityCreated() {
	r.report.EntitiesCreated++
}

// RecordEdgeCreated records a created edge
func (r *Reporter) RecordEdgeCreated() {
	r.report.EdgesCreated++
}

// RecordError records an error
func (r *Reporter) RecordError(errType, objectID, objectType, message, filePath string) {
	r.report.Errors = append(r.report.Errors, ReportError{
		Type:       errType,
		ObjectID:   objectID,
		ObjectType: objectType,
		Message:    message,
		FilePath:   filePath,
	})
}

// RecordWarning records a warning
func (r *Reporter) RecordWarning(message string) {
	r.report.Warnings = append(r.report.Warnings, message)
}

// RecordUnresolvedReference records an unresolved reference
func (r *Reporter) RecordUnresolvedReference(refID string) {
	r.report.ValidationResults.UnresolvedReferences = append(
		r.report.ValidationResults.UnresolvedReferences, refID)
}

// RecordOrphanedEntity records an orphaned entity
func (r *Reporter) RecordOrphanedEntity(entityID string) {
	r.report.ValidationResults.OrphanedEntities = append(
		r.report.ValidationResults.OrphanedEntities, entityID)
}

// RecordDuplicateID records a duplicate ID
func (r *Reporter) RecordDuplicateID(id string) {
	r.report.ValidationResults.DuplicateIDs = append(
		r.report.ValidationResults.DuplicateIDs, id)
}

// RecordInvalidEdgeType records an invalid edge type
func (r *Reporter) RecordInvalidEdgeType(edgeType string) {
	r.report.ValidationResults.InvalidEdgeTypes = append(
		r.report.ValidationResults.InvalidEdgeTypes, edgeType)
}

// RecordMissingRequiredField records a missing required field
func (r *Reporter) RecordMissingRequiredField(objectID, objectType, field, filePath string) {
	r.report.ValidationResults.MissingRequiredFields = append(
		r.report.ValidationResults.MissingRequiredFields,
		ReportError{
			Type:       "validation",
			ObjectID:   objectID,
			ObjectType: objectType,
			Message:    fmt.Sprintf("missing required field: %s", field),
			FilePath:   filePath,
		})
}

// GetReport returns the current report
func (r *Reporter) GetReport() *MigrationReport {
	return r.report
}

// GenerateJSON generates a JSON report
func (r *Reporter) GenerateJSON(outputPath string) error {
	data, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal report").Wrap(err)
	}

	if err := fileutil.WriteFile(outputPath, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write report").Wrap(err)
	}

	return nil
}

// GenerateMarkdown generates a markdown report
func (r *Reporter) GenerateMarkdown(outputPath string) error {
	var report strings.Builder

	report.WriteString("# Migration Report\n\n")
	fmt.Fprintf(&report, "**Start Time**: %s\n", r.report.StartTime.Format(time.RFC3339))
	fmt.Fprintf(&report, "**End Time**: %s\n", r.report.EndTime.Format(time.RFC3339))
	fmt.Fprintf(&report, "**Duration**: %s\n\n", r.report.Duration.Round(time.Second))

	report.WriteString("## Statistics\n\n")
	report.WriteString("| Metric | Count |\n")
	report.WriteString("|--------|-------|\n")
	fmt.Fprintf(&report, "| Objects Scanned | %d |\n", r.report.ObjectsScanned)
	fmt.Fprintf(&report, "| Objects Migrated | %d |\n", r.report.ObjectsMigrated)
	fmt.Fprintf(&report, "| Documents Created | %d |\n", r.report.DocumentsCreated)
	fmt.Fprintf(&report, "| Entities Created | %d |\n", r.report.EntitiesCreated)
	fmt.Fprintf(&report, "| Edges Created | %d |\n", r.report.EdgesCreated)
	fmt.Fprintf(&report, "| Errors | %d |\n", len(r.report.Errors))
	fmt.Fprintf(&report, "| Warnings | %d |\n\n", len(r.report.Warnings))

	if len(r.report.Errors) > 0 {
		report.WriteString("## Errors\n\n")
		report.WriteString("| Type | Object ID | Object Type | Message | File Path |\n")
		report.WriteString("|------|-----------|-------------|---------|-----------|\n")
		for _, err := range r.report.Errors {
			fmt.Fprintf(&report, "| %s | %s | %s | %s | %s |\n",
				err.Type, err.ObjectID, err.ObjectType, err.Message, err.FilePath)
		}
		report.WriteString("\n")
	}

	if len(r.report.Warnings) > 0 {
		report.WriteString("## Warnings\n\n")
		for _, warning := range r.report.Warnings {
			fmt.Fprintf(&report, "- %s\n", warning)
		}
		report.WriteString("\n")
	}

	if r.report.ValidationResults != nil {
		report.WriteString("## Validation Results\n\n")

		if len(r.report.ValidationResults.UnresolvedReferences) > 0 {
			report.WriteString("### Unresolved References\n\n")
			for _, ref := range r.report.ValidationResults.UnresolvedReferences {
				fmt.Fprintf(&report, "- %s\n", ref)
			}
			report.WriteString("\n")
		}

		if len(r.report.ValidationResults.OrphanedEntities) > 0 {
			report.WriteString("### Orphaned Entities\n\n")
			for _, entity := range r.report.ValidationResults.OrphanedEntities {
				fmt.Fprintf(&report, "- %s\n", entity)
			}
			report.WriteString("\n")
		}

		if len(r.report.ValidationResults.DuplicateIDs) > 0 {
			report.WriteString("### Duplicate IDs\n\n")
			for _, id := range r.report.ValidationResults.DuplicateIDs {
				fmt.Fprintf(&report, "- %s\n", id)
			}
			report.WriteString("\n")
		}

		if len(r.report.ValidationResults.InvalidEdgeTypes) > 0 {
			report.WriteString("### Invalid Edge Types\n\n")
			for _, edgeType := range r.report.ValidationResults.InvalidEdgeTypes {
				fmt.Fprintf(&report, "- %s\n", edgeType)
			}
			report.WriteString("\n")
		}

		if len(r.report.ValidationResults.MissingRequiredFields) > 0 {
			report.WriteString("### Missing Required Fields\n\n")
			for _, err := range r.report.ValidationResults.MissingRequiredFields {
				fmt.Fprintf(&report, "- %s (%s): %s\n", err.ObjectID, err.ObjectType, err.Message)
			}
			report.WriteString("\n")
		}
	}

	if err := fileutil.WriteFile(outputPath, []byte(report.String()), paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write report").Wrap(err)
	}

	return nil
}
