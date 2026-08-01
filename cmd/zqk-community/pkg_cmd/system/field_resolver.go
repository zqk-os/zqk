package system

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// FieldResolver resolves field values from constraints/hints (reverse of validation)
// Follows the same pattern as validators: route by semantic type
type FieldResolver struct {
	specLoader      *objects.SpecLoader
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
}

// NewFieldResolver creates a new field resolver
func NewFieldResolver(projectRoot string, specLoader *objects.SpecLoader, storageProvider storage.ObjectStorageProvider) *FieldResolver {
	return &FieldResolver{
		specLoader:      specLoader,
		storageProvider: storageProvider,
		projectRoot:     projectRoot,
	}
}

// ResolveField resolves a field value using constraints/hints
// This is the reverse of validation: given field + constraints → resolve to value
// Routes to appropriate resolver based on semantic type
func (fr *FieldResolver) ResolveField(
	ctx context.Context,
	objKind, fieldName string,
	fieldDef map[string]any,
	queryHints map[string]any, // Constraints/hints (category, tags, status, title, etc.)
) ([]string, error) {
	// Get semantic type from field definition
	semanticType, _ := fieldDef["semantic_type"].(string)
	if semanticType == emptyValue {
		return nil, errfmt.Errorf("field %s has no semantic_type, cannot resolve", fieldName)
	}

	// Route to appropriate resolver based on semantic type
	switch semanticType {
	case "reference":
		return fr.resolveReference(ctx, objKind, fieldName, fieldDef, queryHints)
	case "enum":
		return fr.resolveEnum(ctx, objKind, fieldName, fieldDef, queryHints)
	// Future: other semantic types
	// case "statement":
	// case "quantity":
	// case "comparison":
	default:
		return nil, errfmt.Errorf("no resolver for semantic_type %s (field: %s)", semanticType, fieldName)
	}
}

// resolveReference resolves a reference field using query hints
// Examples:
//   - milestone_refs (list) + hints → resolve to milestone IDs
//   - priority_plan_ref (string) + hints → resolve to priority_plan ID
func (fr *FieldResolver) resolveReference(
	ctx context.Context,
	objKind, fieldName string,
	fieldDef map[string]any,
	queryHints map[string]any,
) ([]string, error) {
	// Extract target kind from field name
	// milestone_refs → milestone
	// goal_refs → goal
	// priority_plan_ref → priority_plan
	targetKind := extractTargetKindFromFieldName(fieldName)
	if targetKind == emptyValue {
		return nil, errfmt.Errorf("cannot infer target kind from field name: %s", fieldName)
	}

	// Build ListFilter from query hints (reuse existing parseQueryHintToFilter logic)
	// Convert queryHints map back to query hint string for existing parser
	queryHintStr := buildQueryHintFromMap(queryHints)
	filter := parseQueryHintToFilter(targetKind, queryHintStr)

	// Query storage provider
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	result, err := fr.storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query %s objects", targetKind).Wrap(err)
	}

	// Extract candidate IDs
	candidates := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok {
			candidates = append(candidates, id)
		}
	}

	return candidates, nil
}

// resolveEnum resolves an enum field using constraints
// Future implementation: given enum constraints, resolve to enum value
func (fr *FieldResolver) resolveEnum(
	ctx context.Context,
	objKind, fieldName string,
	fieldDef map[string]any,
	queryHints map[string]any,
) ([]string, error) {
	// TODO: Implement enum resolution
	// For now, return empty (enum fields are typically not resolved from hints)
	return nil, errfmt.Errorf("enum resolution not yet implemented")
}

// extractTargetKindFromFieldName extracts the target kind from a reference field name
// Examples:
//   - milestone_refs → milestone
//   - goal_refs → goal
//   - priority_plan_ref → priority_plan
//   - workstream_ref → workstream
func extractTargetKindFromFieldName(fieldName string) string {
	// Handle _refs suffix (plural references)
	if strings.HasSuffix(fieldName, "_refs") {
		kind := strings.TrimSuffix(fieldName, "_refs")
		return kind
	}

	// Handle _ref suffix (singular reference)
	if strings.HasSuffix(fieldName, "_ref") {
		kind := strings.TrimSuffix(fieldName, "_ref")
		return kind
	}

	return ""
}

// ResolveFieldFromSpec resolves a field using the object spec
// Loads field definition from spec and routes to appropriate resolver
func (fr *FieldResolver) ResolveFieldFromSpec(
	ctx context.Context,
	objKind, fieldName string,
	queryHints map[string]any,
) ([]string, error) {
	// Load object spec
	spec, err := fr.specLoader.LoadSpecWithInheritance(objKind + ".yaml")
	if err != nil {
		return nil, errfmt.Newf("failed to load spec for %s", objKind).Wrap(err)
	}

	// Get field definition from resolved fields
	fieldDef, ok := spec.ResolvedFields[fieldName]
	if !ok {
		return nil, errfmt.Errorf("field %s not found in spec for %s", fieldName, objKind)
	}

	fieldDefMap, ok := fieldDef.(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("field definition for %s is not a map", fieldName)
	}

	// Resolve using field definition
	return fr.ResolveField(ctx, objKind, fieldName, fieldDefMap, queryHints)
}
