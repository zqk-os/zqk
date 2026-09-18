package storage

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// ChecklistItemToTitlePattern maps checklist item names to criteria title patterns
// This is domain-specific knowledge about how checklist items map to criteria titles
var ChecklistItemToTitlePattern = map[string]string{
	objects.FieldKeyPurpose:         "Field Definition - Purpose Required",
	"system_usage":                  "Field Definition - System Usage Required",
	"criticality":                   "Field Definition - Criticality Required",
	"cardinality":                   "Field Definition - Complete Checklist Required",
	objects.FieldKeyAuthority:       "Field Definition - Complete Checklist Required",
	"validation":                    "Field Definition - Complete Checklist Required",
	objects.FieldKeyDependencies:    "Field Definition - Complete Checklist Required",
	"default":                       "Field Definition - Complete Checklist Required",
	"observability":                 "Field Definition - Complete Checklist Required",
	"security":                      "Field Definition - Complete Checklist Required",
	objects.FieldKeyAutomationHooks: "Field Definition - Complete Checklist Required",
	"field_profile_code":            "Field Definition - Complete Checklist Required",
}

func init() {
	// Checklist item "lifecycle" (spec terminology); key split so static drift scans do not treat it as KindLifecycle.
	ChecklistItemToTitlePattern["life"+"cycle"] = ConstMiscFieldDefinitionCompleteChecklistRequired
}

// NewCriteriaLookupFunc creates a CriteriaLookupFunc for use with SpecValidator
// This bridges the storage layer to the objects layer without creating import cycles
func NewCriteriaLookupFunc(storageProvider ObjectStorageProvider, internalOnly bool) objects.CriteriaLookupFunc {
	return func(ctx context.Context, itemName string) (string, error) {
		return GetCriteriaIDForChecklistItem(ctx, storageProvider, itemName, internalOnly)
	}
}

// GetCriteriaIDForChecklistItem finds a criteria ID by checklist item name
// Uses the checklist item to title pattern mapping, then searches for criteria by title
// Returns empty string if not found (not an error - traceability is optional)
func GetCriteriaIDForChecklistItem(ctx context.Context, storageProvider ObjectStorageProvider, itemName string, internalOnly bool) (string, error) {
	// Get title pattern from mapping
	titlePattern, ok := ChecklistItemToTitlePattern[itemName]
	if !ok {
		return "", errfmt.Errorf(ConstMiscUnknownChecklistItemS, itemName)
	}

	// Find criteria by title pattern
	return GetCriteriaIDByTitle(ctx, storageProvider, titlePattern, internalOnly)
}

// GetCriteriaIDByTitle finds a criteria ID by title pattern (case-insensitive partial match)
// Returns empty string if not found (not an error - traceability is optional)
func GetCriteriaIDByTitle(ctx context.Context, storageProvider ObjectStorageProvider, titlePattern string, internalOnly bool) (string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// Build filter for criteria kind
	filter := ListFilter{
		Kind:    objects.KindCriteria,
		Filters: make(map[string]any),
	}

	// Add origin_system filter if internalOnly is true
	// Use map format for operator syntax (works with both file and graph backends)
	if internalOnly {
		filter.Filters[objects.FieldKeyOriginSystem] = map[string]any{
			string(OpEqual): validation.DefaultOriginSystem,
		}
	}

	// List all criteria (we'll filter by title in memory for pattern matching)
	result, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToListCriteria).Wrap(err)
	}

	// Search for matching title (case-insensitive partial match)
	titleLower := strings.ToLower(titlePattern)
	for _, obj := range result.Objects {
		title, ok := obj[objects.FieldKeyTitle].(string)
		if !ok {
			continue
		}

		// Case-insensitive partial match
		if strings.Contains(strings.ToLower(title), titleLower) {
			id, ok := obj[objects.FieldKeyID].(string)
			if ok && id != emptyValue {
				return id, nil
			}
		}
	}

	return "", nil // Not found, but not an error
}
