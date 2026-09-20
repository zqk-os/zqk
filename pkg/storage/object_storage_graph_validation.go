package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// validateObject validates an object against its spec, lifecycle, and references
func (g *GraphObjectStorage) validateObject(ctx context.Context, obj map[string]any, kind, currentState string) error {
	// 1. Spec validation using GoValidator
	// BLI-621 / KMP: --force skips lifecycle only with audited break_glass reason for critical kinds.
	// TRACK: BLI-1785784867143912000-635942fb
	if pkgctx.IsLifecycleBreakGlass(ctx) && IsCoreKernelKind(kind) {
		if !pkgctx.GetAllowCoreObjectDelete(ctx) && zqkenv.TestRoot().Get() == "" {
			return errfmt.Errorf("break_glass requires --reason-code for critical kind %s (Kernel Mutation Pipeline DECIDE)", kind)
		}
	}
	// Break-glass and trusted shockwave writes still validate lifecycle. The
	// validator skips auto-only *edges* under those overrides; skipping here
	// dropped criteria/complete holds and allowed false-complete BLIs.
	// TRACK: BLI-1785784867143912000-635942fb
	options := &validation.ValidationOptions{
		CurrentState:          currentState,
		ValidateLifecycle:     true,
		ValidateSemanticTypes: true,
		// Same contract as file validation: evidence checks this storage tree.
		// TRACK: BLI-1787131824765736000-312b6c71
		ProjectRoot: g.projectRoot,
	}
	options.ObjectLookup = func(id string) (map[string]any, error) {
		secCtx := pkgctx.NewSystemSecurityContext()
		return g.Read(ctx, secCtx, id)
	}
	options.ObjectStatusLookup = func(id string) (string, error) {
		obj, err := options.ObjectLookup(id)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}
	options.DependentsLookup = func(id string) []string {
		return DependentsForID(ctx, g, id)
	}

	result, err := g.validator.Validate(ctx, obj, kind, options)
	if err != nil {
		useMockGraph := config.StorageMockGraph().OrDefault(false) || config.StorageAdminMockGraph().OrDefault(false)
		if useMockGraph {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn("spec validation failed in mock graph mode, proceeding with write").WithError(err).Log()
		} else {
			return errfmt.Newf(ConstStreamSpecValidationFailed).Wrap(err)
		}
	}

	if result != nil && !result.IsValid {
		useMockGraph := config.StorageMockGraph().OrDefault(false) || config.StorageAdminMockGraph().OrDefault(false)
		if useMockGraph {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn("spec validation invalid in mock graph mode, proceeding with write").Log()
		} else if currentState == "" {
			// Per user directive: don't fail 'create' unless vital info missing.
			// Invalid create statuses are coerced to lifecycle origin before validate.
			var errorMessages []string
			for _, validationErr := range result.Errors {
				errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message))
			}
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn("schema validation errors downgraded to warnings during creation").
				String("errors", strings.Join(errorMessages, "; ")).
				Log()
		} else {
			var errorMessages []string
			for _, validationErr := range result.Errors {
				errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message))
			}
			return errfmt.Errorf(ConstStreamValidationErrorsStr, strings.Join(errorMessages, "; "))
		}
	}

	// 2. Reference validation (for graph, we can query the graph directly)
	// Skip reference validation if the object status is 'error' to avoid validation deadlock on invalid objects
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != "error" {
		if err := g.validateReferences(ctx, obj, kind); err != nil {
			useMockGraph := config.StorageMockGraph().OrDefault(false) || config.StorageAdminMockGraph().OrDefault(false)
			if useMockGraph {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn("reference validation failed in mock graph mode, proceeding with write").WithError(err).Log()
			} else {
				return errfmt.Newf(ConstStreamReferenceValidationFailed).Wrap(err)
			}
		}
	}

	return nil
}

// validateReferences validates that all referenced objects exist in the graph
// validateReferences validates all references in an object
//
//nolint:gocyclo // Function orchestrates reference validation; complexity reduced via helper methods
func (g *GraphObjectStorage) validateReferences(ctx context.Context, obj map[string]any, kind string) error {
	// Extract reference fields
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)

	// Track which objects we've already checked
	checkedRefs := make(map[string]bool)

	// Validate each reference field
	for fieldName, refValue := range refFields {
		if refValue == nil {
			continue
		}

		refIDs := g.extractReferenceIDs(refValue)
		if err := g.validateReferenceIDs(ctx, refIDs, fieldName, kind, checkedRefs); err != nil {
			return err
		}
	}

	return nil
}

// extractReferenceIDs extracts reference IDs from various formats
func (g *GraphObjectStorage) extractReferenceIDs(refValue any) []string {
	var refIDs []string

	switch v := refValue.(type) {
	case string:
		if v != emptyValue {
			refIDs = []string{v}
		}
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok && str != emptyValue {
				refIDs = append(refIDs, str)
			}
		}
	case []string:
		refIDs = v
	}

	return refIDs
}

// validateReferenceIDs validates a list of reference IDs
func (g *GraphObjectStorage) validateReferenceIDs(ctx context.Context, refIDs []string, fieldName, kind string, checkedRefs map[string]bool) error {
	for _, refID := range refIDs {
		// Skip if we've already checked this reference
		if checkedRefs[refID] {
			continue
		}
		checkedRefs[refID] = true

		// Handle special cases
		if g.shouldSkipReference(fieldName, kind, refID) {
			continue
		}

		// Validate reference
		if err := g.validateSingleReference(ctx, refID, fieldName); err != nil {
			return err
		}
	}

	return nil
}

// shouldSkipReference checks if a reference should be skipped
func (g *GraphObjectStorage) shouldSkipReference(fieldName, kind, refID string) bool {
	_ = refID // Reserved for future use
	// Skip reference validation for commit_refs / commit_hashes (Git commit hashes, not object IDs)
	if objects.IsLeftoverNonKernelRefField(fieldName) || fieldName == objects.FieldKeyCommitHashes {
		return true
	}

	// For code_reference objects, be lenient about missing work item references
	if kind == objects.KindCodeReference && g.isWorkItemReference(fieldName) {
		return true // Skip validation - work items may be created later
	}

	return false
}

// isWorkItemReference checks if a field is a work item reference
func (g *GraphObjectStorage) isWorkItemReference(fieldName string) bool {
	workItemFields := []string{
		objects.FieldKeyBacklogItemRefs,
		objects.FieldKeyMilestoneRefs,
		objects.FieldKeyGoalRefs,
		objects.FieldKeyWorkstreamRefs,
		objects.FieldKeyRequirementRefs,
	}
	for _, field := range workItemFields {
		if fieldName == field {
			return true
		}
	}
	return false
}

// validateSingleReference validates a single reference
func (g *GraphObjectStorage) validateSingleReference(ctx context.Context, refID, fieldName string) error {
	refKind, actualRefID, err := g.inferReferenceKind(refID)
	if err != nil {
		return errfmt.Errorf(ConstStreamCannotDetermineObjectKindForReferenceStrInField, refID, fieldName, err)
	}

	// Check if referenced object exists in graph
	refLabel := toLabel(refKind)
	_, err = g.conn.GetNode(ctx, actualRefID, []string{refLabel, "Entity"})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.validateSingleReference GetNode", err).Log()
		}
		return errfmt.Errorf(ConstStreamReferencedObjectStrKindStrLabelStrInFieldStrDoes, refID, refKind, refLabel, fieldName, err, refID)
	}

	return nil
}

// inferReferenceKind infers the object kind from a reference ID, handling various formats
func (g *GraphObjectStorage) inferReferenceKind(refID string) (refKind, actualRefID string, err error) {
	actualRefID = refID // Use the full reference string by default

	// First, check if this is a namespaced reference (e.g., "zqk:kernel:goal:GOAL-123")
	parsed := validation.ParseNamespace(refID)
	if parsed != nil && parsed.ObjectType != emptyValue {
		return parsed.ObjectType, parsed.ObjectID, nil
	}

	// Handle account reference (special case: "account:username" format)
	if strings.HasPrefix(refID, "account:") {
		return objects.KindAccount, strings.TrimPrefix(refID, "account:"), nil
	}

	// Handle "kind:id" format or short namespace format
	if strings.Contains(refID, ":") {
		return g.inferKindFromColonFormat(refID)
	}

	// Legacy format - no namespace, just ID
	refKind = g.idValidator.InferKindFromID(refID)
	if refKind == emptyValue {
		return "", "", errfmt.Errorf(ConstStreamCannotInferKindFromId)
	}

	return refKind, actualRefID, nil
}

// inferKindFromColonFormat infers kind from "kind:id" format
func (g *GraphObjectStorage) inferKindFromColonFormat(refID string) (refKind, actualRefID string, err error) {
	parts := strings.SplitN(refID, ":", 2)
	if len(parts) != 2 {
		// Fallback to inferring from ID
		refKind = g.idValidator.InferKindFromID(refID)
		if refKind == emptyValue {
			return "", "", errfmt.Errorf(ConstStreamCannotInferKindFromId)
		}
		return refKind, refID, nil
	}

	refKind = parts[0]
	actualRefID = parts[1]

	// Validate that this is a valid kind (not a namespace layer)
	// Use configurable namespace layers instead of hardcoded values
	config := validation.GetGlobalNamespacesConfig()
	isValidLayer := false
	if config != nil {
		isValidLayer = config.IsValidNamespaceLayer(refKind)
	} else {
		// Fallback for backward compatibility
		isValidLayer = (refKind == "zqk" || refKind == "domain" || refKind == "integration")
	}
	if !isValidLayer {
		// Likely "kind:id" format, use as-is
		return refKind, actualRefID, nil
	}

	// This might be a namespace layer, fall back to inferring from ID
	refKind = g.idValidator.InferKindFromID(refID)
	if refKind == emptyValue {
		return "", "", errfmt.Errorf(ConstStreamCannotInferKindFromId)
	}

	return refKind, refID, nil
}
