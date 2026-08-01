package system

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/interactive"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// UpdateLoopProcessor manages the validation loop for template-based object updates
type UpdateLoopProcessor struct {
	generator       *interactive.TemplateGenerator
	storageProvider storage.ObjectStorageProvider
	fieldFilter     *interactive.UpdateFieldFilter
}

// NewUpdateLoopProcessor creates a new update loop processor
func NewUpdateLoopProcessor(
	generator *interactive.TemplateGenerator,
	storageProvider storage.ObjectStorageProvider,
) *UpdateLoopProcessor {
	return &UpdateLoopProcessor{
		generator:       generator,
		storageProvider: storageProvider,
		fieldFilter:     interactive.NewUpdateFieldFilter(),
	}
}

// UpdateLoopState represents the state of the update validation loop
type UpdateLoopState struct {
	ObjectID             string            // Object ID being updated
	Kind                 string            // Object kind
	ExistingObject       map[string]any    // Current object state
	FieldsToUpdate       []string          // Fields to update (if specified, empty = all mutable fields)
	ProvidedValues       map[string]any    // New values provided by user
	ChangedFields        []string          // Fields that have changed (detected)
	ValidationErrors     map[string]string // Field-level validation errors
	LifecycleErrors      []string          // Lifecycle transition errors
	IsValid              bool              // Whether all changes are valid
	PreviewDiff          string            // Diff/preview of changes
	AwaitingConfirmation bool              // Whether waiting for confirmation
	CurrentValues        map[string]any    // Current values for fields being updated (for template)
}

// ProcessUpdateLoop processes one iteration of the update validation loop
func (ulp *UpdateLoopProcessor) ProcessUpdateLoop(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	objectID string,
	fieldsToUpdate []string,
	providedValues map[string]any,
) (*UpdateLoopState, error) {
	// Load existing object
	existingObj, err := ulp.storageProvider.Read(ctx, secCtx, objectID)
	if err != nil {
		return nil, errfmt.Errorf("failed to read object %s: %w", objectID, err)
	}

	kind, ok := existingObj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return nil, errfmt.Errorf("object %s missing or invalid kind field", objectID)
	}

	// Build current values map for fields to update
	currentValues := ulp.buildCurrentValues(existingObj, fieldsToUpdate)

	// Detect changed fields
	changedFields := ulp.detectChangedFields(existingObj, providedValues, fieldsToUpdate)

	// Validate changes (placeholder - will be enhanced)
	validationErrors := make(map[string]string)
	lifecycleErrors := []string{}

	// Check if valid (no errors)
	isValid := len(validationErrors) == 0 && len(lifecycleErrors) == 0

	// Generate preview diff if valid
	previewDiff := ""
	if isValid && len(changedFields) > 0 {
		previewDiff = ulp.generatePreviewDiff(existingObj, providedValues, changedFields)
	}

	// Determine if awaiting confirmation
	// If user provided values and changes are valid, await confirmation
	awaitingConfirmation := len(providedValues) > 0 && isValid && len(changedFields) > 0

	return &UpdateLoopState{
		ObjectID:             objectID,
		Kind:                 kind,
		ExistingObject:       existingObj,
		FieldsToUpdate:       fieldsToUpdate,
		ProvidedValues:       providedValues,
		ChangedFields:        changedFields,
		ValidationErrors:     validationErrors,
		LifecycleErrors:      lifecycleErrors,
		IsValid:              isValid,
		PreviewDiff:          previewDiff,
		AwaitingConfirmation: awaitingConfirmation,
		CurrentValues:        currentValues,
	}, nil
}

// buildCurrentValues builds a map of current values for fields to update
func (ulp *UpdateLoopProcessor) buildCurrentValues(
	existingObj map[string]any,
	fieldsToUpdate []string,
) map[string]any {
	currentValues := make(map[string]any)

	if len(fieldsToUpdate) > 0 {
		// Only include specified fields
		for _, fieldName := range fieldsToUpdate {
			if ulp.fieldFilter.IsFieldUpdatable(fieldName) {
				if value, exists := existingObj[fieldName]; exists {
					currentValues[fieldName] = value
				}
			}
		}
	} else {
		// Include all updatable fields
		for fieldName, value := range existingObj {
			if ulp.fieldFilter.IsFieldUpdatable(fieldName) {
				currentValues[fieldName] = value
			}
		}
	}

	return currentValues
}

// detectChangedFields detects which fields have changed
func (ulp *UpdateLoopProcessor) detectChangedFields(
	existingObj map[string]any,
	providedValues map[string]any,
	fieldsToUpdate []string,
) []string {
	changedFields := []string{}

	// Fields to check
	fieldsToCheck := fieldsToUpdate
	if len(fieldsToCheck) == 0 {
		// Check all fields that were provided
		for fieldName := range providedValues {
			if ulp.fieldFilter.IsFieldUpdatable(fieldName) {
				fieldsToCheck = append(fieldsToCheck, fieldName)
			}
		}
	}

	for _, fieldName := range fieldsToCheck {
		if ulp.fieldFilter.IsFieldImmutable(fieldName) {
			continue // Skip immutable fields
		}

		newValue, provided := providedValues[fieldName]
		if !provided {
			continue // Field not provided, no change
		}

		existingValue, exists := existingObj[fieldName]

		// Compare values
		if !exists {
			// Field didn't exist, now it does (change)
			changedFields = append(changedFields, fieldName)
		} else if !valuesEqual(existingValue, newValue) {
			// Values differ (change)
			changedFields = append(changedFields, fieldName)
		}
		// If values are equal, no change
	}

	return changedFields
}

// valuesEqual compares two values for equality
func valuesEqual(a, b any) bool {
	// Use reflect.DeepEqual for complex types
	if reflect.DeepEqual(a, b) {
		return true
	}

	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// String comparison for simple cases
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

// generatePreviewDiff generates a human-readable diff preview
func (ulp *UpdateLoopProcessor) generatePreviewDiff(
	existingObj map[string]any,
	providedValues map[string]any,
	changedFields []string,
) string {
	if len(changedFields) == 0 {
		return "No changes"
	}

	var diffLines []string
	diffLines = append(diffLines, "Changes to be applied:")
	diffLines = append(diffLines, "")

	for _, fieldName := range changedFields {
		existingValue := existingObj[fieldName]
		newValue := providedValues[fieldName]

		existingStr := formatValue(existingValue)
		newStr := formatValue(newValue)

		if existingValue == nil {
			diffLines = append(diffLines, fmt.Sprintf("  %s: (none) -> %s", fieldName, newStr))
		} else {
			diffLines = append(diffLines, fmt.Sprintf("  %s: %s -> %s", fieldName, existingStr, newStr))
		}
	}

	return fmt.Sprintf("%s\n", strings.Join(diffLines, "\n"))
}

// formatValue formats a value for display
func formatValue(value any) string {
	if value == nil {
		return "(none)"
	}

	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []any:
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]any:
		return "{object}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetFieldsToElicit returns the fields that need to be elicited from the client
func (ulp *UpdateLoopProcessor) GetFieldsToElicit(loopState *UpdateLoopState) []*interactive.FieldTokenInfo {
	fieldsToElicit := []*interactive.FieldTokenInfo{}

	// Get field registry from generator (via reflection or add getter)
	// For now, we'll need to access it - let's add a getter to StreamingTemplateGenerator
	fieldRegistry := ulp.generator.GetFieldRegistry()

	// Determine which fields to elicit
	fieldsToCheck := loopState.FieldsToUpdate
	if len(fieldsToCheck) == 0 {
		// Elicit all updatable fields that don't have values yet
		for fieldName := range loopState.CurrentValues {
			if _, provided := loopState.ProvidedValues[fieldName]; !provided {
				fieldsToCheck = append(fieldsToCheck, fieldName)
			}
		}
	} else {
		// Only elicit specified fields that don't have values yet
		for _, fieldName := range loopState.FieldsToUpdate {
			if _, provided := loopState.ProvidedValues[fieldName]; !provided {
				fieldsToCheck = append(fieldsToCheck, fieldName)
			}
		}
	}

	// Get field info for each field
	for _, fieldName := range fieldsToCheck {
		if !ulp.fieldFilter.IsFieldUpdatable(fieldName) {
			continue
		}

		kindFields, err := fieldRegistry.GetFieldsForKind(loopState.Kind)
		if err != nil {
			continue
		}

		// Find field info
		for i := range kindFields.AllFields {
			field := &kindFields.AllFields[i]
			if field.Name == fieldName {
				tokenInfo := &interactive.FieldTokenInfo{
					Name:         field.Name,
					Type:         field.Type,
					Required:     field.Required,
					Description:  field.Description,
					EnumValues:   field.EnumValues,
					SemanticType: field.SemanticType,
				}
				fieldsToElicit = append(fieldsToElicit, tokenInfo)
				break
			}
		}
	}

	return fieldsToElicit
}
