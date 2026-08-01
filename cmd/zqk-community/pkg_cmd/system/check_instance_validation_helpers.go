package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
)

// storageProviderForInstanceValidation returns storage for auto-fix rule lookup and related paths.
// For temp/test project roots, uses NewFileObjectStorageForTest (no write-behind/WAL, no global wiring)
// so parallel tests and t.TempDir cleanup do not race with background CAS/index workers.
func storageProviderForInstanceValidation(projectRoot string) storage.ObjectStorageProvider {
	if projectRoot == emptyValue {
		return nil
	}
	if storage.IsTestOrTempProjectRoot(projectRoot) {
		f, err := storage.NewFileObjectStorageForTest(projectRoot)
		if f != nil {
			defer func() { _ = f.Shutdown(context.Background()) }()
		}
		if err != nil {
			return nil
		}
		return f
	}
	storageFactory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	if err != nil {
		return nil
	}
	return storageFactory.GetStorage()
}

// InstanceValidationContext groups state for instance validation
type InstanceValidationContext struct {
	Ctx             *cli.Context
	StdCtx          context.Context // Standard context.Context for cancellation/timeouts
	Obj             *parser.ParsedObject
	Kind            string
	Validator       validation.Validator
	ObjMap          map[string]any
	CurrentState    string
	IsDebugObject   bool
	ProjectRoot     string
	StorageProvider storage.ObjectStorageProvider
}

// initializeInstanceValidationContext sets up the instance validation context
func initializeInstanceValidationContext(ctx *cli.Context, stdCtx context.Context, obj *parser.ParsedObject, kind string) (*InstanceValidationContext, []Issue) {
	var issues []Issue

	validatorName := "" // Empty = use default
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get(validatorName)
	if validator == nil {
		return nil, []Issue{
			{
				Tier:     2,
				Category: "instance_validation",
				Message:  "No validator available",
			},
		}
	}

	objMap := parseObjectMapForValidation(obj)
	currentState := getCurrentStateFromObject(objMap)

	isDebugObject := obj.ID == "POLICY-DEBUG-001"

	if isDebugObject {
		// Get project root and storage provider for coordinator
		projectRoot := ctx.ProjectRoot
		projectRoot = ProjectRootOrResolve(projectRoot)
		storageProvider := storageProviderForInstanceValidation(projectRoot)

		profile := profileOrDefault(ctx.Profile, systemProfileSystem)

		emitValidationInputDebugViaCoordinator(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			obj.ID,
			objMap,
			obj.FilePath,
			profile,
		)
	}

	// Use provided context or create system context if not provided
	if stdCtx == nil {
		stdCtx = pkgctx.NewSystemContext()
	}

	// Get project root and storage provider for coordinator
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	storageProvider := storageProviderForInstanceValidation(projectRoot)

	return &InstanceValidationContext{
		Ctx:             ctx,
		StdCtx:          stdCtx,
		Obj:             obj,
		Kind:            kind,
		Validator:       validator,
		ObjMap:          objMap,
		CurrentState:    currentState,
		IsDebugObject:   isDebugObject,
		ProjectRoot:     projectRoot,
		StorageProvider: storageProvider,
	}, issues
}

// parseObjectMapForValidation parses object map from file or uses Properties
func parseObjectMapForValidation(obj *parser.ParsedObject) map[string]any {
	var objMap map[string]any

	if obj.FilePath != emptyValue {
		content, err := os.ReadFile(obj.FilePath)
		if err == nil && len(content) > 0 {
			var parsedMap map[string]any
			if err := yaml.Unmarshal(content, &parsedMap); err == nil && len(parsedMap) > 0 {
				objMap = parsedMap
			} else if obj.Properties != nil {
				objMap = obj.Properties
			}
		} else if obj.Properties != nil {
			objMap = obj.Properties
		}
	} else if obj.Properties != nil {
		objMap = obj.Properties
	}

	if objMap == nil {
		objMap = make(map[string]any)
		if obj.Properties != nil {
			objMap = obj.Properties
		}
	}

	return objMap
}

// getCurrentStateFromObject extracts current state from object map
func getCurrentStateFromObject(objMap map[string]any) string {
	if status, ok := objMap[objects.FieldKeyStatus].(string); ok {
		return status
	}
	return ""
}

// performValidation performs the actual validation
func performValidation(valCtx *InstanceValidationContext) (*validation.ValidationResult, error) {
	options := validation.DefaultValidationOptions()
	options.CurrentState = valCtx.CurrentState

	// Use context from validation context, or system context if not set
	ctx := valCtx.StdCtx
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	result, err := valCtx.Validator.Validate(ctx, valCtx.ObjMap, valCtx.Kind, options)

	if valCtx.IsDebugObject {
		profile := valCtx.Ctx.Profile
		if profile == emptyValue {
			profile = systemProfileSystem
		}

		// Convert validation errors to map format for coordinator
		validationErrors := make([]map[string]any, 0, len(result.Errors))
		for _, validationError := range result.Errors {
			validationErrors = append(validationErrors, map[string]any{
				objects.FieldKeyField: validationError.Field,
				"message":             validationError.Message,
				"validation_rule":     validationError.Rule,
			})
		}

		emitValidationResultDebugViaCoordinator(
			valCtx.StdCtx,
			valCtx.ProjectRoot,
			valCtx.StorageProvider,
			valCtx.Obj.ID,
			err,
			len(result.Errors),
			len(result.Warnings),
			validationErrors,
			profile,
		)
	}

	return result, err
}

// convertValidationResultsToIssues converts validation errors and warnings to issues
// Generates fix commands at error-reporting time for auto-fixable issues.
// objMap is used to extract contextual information (category, tags, status, title) for refining fix commands.
// When projectRoot and storageProvider are set, auto_fix_rule objects are consulted first; if a matching
// rule exists its fix_command_template (with placeholders substituted) is used; otherwise generateFixCommand is used.
func convertValidationResultsToIssues(result *validation.ValidationResult, err error, objID, kind string, objMap map[string]any, projectRoot string, storageProvider storage.ObjectStorageProvider) []Issue {
	var issues []Issue

	if err != nil {
		return []Issue{
			{
				Tier:     2,
				Category: "instance_validation",
				Message:  fmt.Sprintf("Instance validation error: %v", err),
			},
		}
	}

	loader := GetAutoFixRuleLoader()
	for _, validationError := range result.Errors {
		tier := 2 // Default to warning
		if validationError.Rule == "required" || validationError.Rule == objects.KindLifecycle {
			tier = 1 // Blocking for required fields and lifecycle violations
		}

		issue := Issue{
			Tier:     tier,
			Category: "instance_validation",
			Message:  fmt.Sprintf("%s: %s", validationError.Field, validationError.Message),
		}

		// Try auto_fix_rule lookup first when storage is available; else use generated fix command
		fixCmd := ""
		if projectRoot != emptyValue && storageProvider != nil {
			fixCmd, _ = loader.GetFixCommand(projectRoot, storageProvider, kind, "instance_validation", tier, validationError.Rule, validationError.Message, objID, validationError.Field, objMap)
		}
		if fixCmd == emptyValue {
			fixCmd = generateFixCommand(objID, kind, validationError.Field, validationError.Message, validationError.Rule, objMap)
		}
		if fixCmd != emptyValue {
			issue.FixCommand = fixCmd
			issue.AutoFixable = true
		} else {
			// Mark errors as auto-fixable if they can be fixed via spec-based fixes
			// (type coercion, pattern fixes, enum fixes, defaults for required fields, etc.)
			// Check both Rule field and message content for better detection
			msgLower := strings.ToLower(validationError.Message)
			ruleLower := strings.ToLower(validationError.Rule)

			// Tier 1: Required fields can be auto-fixed with defaults
			if tier == 1 {
				if ruleLower == "required" || ruleLower == "mincount" || ruleLower == "min_count" {
					// Check if message indicates a required field that can have a default
					if strings.Contains(msgLower, "required") || strings.Contains(msgLower, "is required") || strings.Contains(msgLower, "mincount") {
						issue.AutoFixable = true
					}
				}
			}

			// Tier 2: Type coercion, pattern fixes, enum fixes, length constraints
			if tier == 2 {

				// Check rule type first (most reliable)
				// Also check lowercase version in case Rule has different casing
				switch ruleLower {
				case "type", "datatype", "pattern", "enum", "in", "mincount", "min_count", "minlength", "min_length", "maxlength", "max_length":
					issue.AutoFixable = true
				}

				// Also check message content for patterns that indicate fixability
				// This is a fallback in case Rule field isn't set correctly
				// Always check message content to catch all fixable issues
				// Use separate if statements to ensure we check all patterns
				if strings.Contains(msgLower, "pattern") || strings.Contains(msgLower, "does not match") {
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "datatype") || (strings.Contains(msgLower, "invalid type") && strings.Contains(msgLower, "expected")) || (strings.Contains(msgLower, "expected") && strings.Contains(msgLower, "got")) {
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "enum") || strings.Contains(msgLower, "invalid value") || strings.Contains(msgLower, "must be one of") {
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "required") && (strings.Contains(msgLower, "is required") || strings.Contains(msgLower, "mincount")) {
					// Required field issues can sometimes be fixed with defaults
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "too short") || strings.Contains(msgLower, "minimum length") || strings.Contains(msgLower, "requires at least") {
					// min_length violations can be fixed by padding (strings or arrays)
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "too long") || strings.Contains(msgLower, "maximum length") || strings.Contains(msgLower, "exceeds maximum") {
					// max_length violations can be fixed by truncation (strings or arrays)
					issue.AutoFixable = true
				}
				if strings.Contains(msgLower, "items") && (strings.Contains(msgLower, "requires at least") || strings.Contains(msgLower, "exceeds maximum")) {
					// Array/list length violations
					issue.AutoFixable = true
				}
			}
		}

		issues = append(issues, issue)
	}

	for _, validationWarning := range result.Warnings {
		// Skip display_length warnings by default (suppressed - display_length is for UI formatting, not data validation)
		// Users can opt-in by setting ValidateDisplayLength=true in ValidationOptions
		ruleLower := strings.ToLower(validationWarning.Rule)
		if ruleLower == "display_length" {
			continue // Suppress display_length warnings by default
		}

		issue := Issue{
			Tier:     3,
			Category: "instance_validation",
			Message:  fmt.Sprintf("%s: %s", validationWarning.Field, validationWarning.Message),
		}

		// Try auto_fix_rule lookup first when storage is available; else use generated fix command
		fixCmd := ""
		if projectRoot != emptyValue && storageProvider != nil {
			fixCmd, _ = loader.GetFixCommand(projectRoot, storageProvider, kind, "instance_validation", 3, validationWarning.Rule, validationWarning.Message, objID, validationWarning.Field, objMap)
		}
		if fixCmd == emptyValue {
			fixCmd = generateFixCommand(objID, kind, validationWarning.Field, validationWarning.Message, validationWarning.Rule, objMap)
		}
		if fixCmd != emptyValue {
			issue.FixCommand = fixCmd
			// Mark tier 3 warnings as auto-fixable if they have fix commands
			// This enables auto-fix for semantic_type, etc.
			issue.AutoFixable = true
		} else {
			// Even without fix commands, some tier 3 issues can be auto-fixed
			// (e.g., type coercion, length constraints)
			switch ruleLower {
			case "semantic_type", "min_length", "minlength", "max_length", "maxlength":
				issue.AutoFixable = true
			}
		}

		issues = append(issues, issue)
	}

	return issues
}

// generateFixCommand generates a fix command from a validation error
// Uses objMap to extract contextual information (category, tags, status, title) for refining commands
// Returns empty string if no command can be generated
// The command may include placeholders (e.g., <MILESTONE_ID>) that can be resolved later using contextual queries
//
// Finite State Possibilities:
// 1. Empty objID -> return "" (FAIL-FAST, State 1)
// 2. Unsupported rule type -> return "" (FAIL-FAST, State 4)
// 3. Lifecycle violation with valid pattern -> generate fix command (State 2)
// 4. Required field violation -> generate generic fix command (State 3)
// 5. Unknown pattern/no match -> return "" (FAIL-FAST, State 4)
func generateFixCommand(objID, kind, field, message, rule string, objMap map[string]any) string {
	// State 1: FAIL-FAST - Empty objID (non-determinable)
	if objID == emptyValue {
		return ""
	}

	// State 4: FAIL-FAST - Unsupported rule types (non-determinable)
	// Only lifecycle and required rules are supported
	if rule != objects.KindLifecycle && rule != "required" {
		return ""
	}

	// Extract contextual information from object for refining queries
	// These can be used to build boolean filter chains (category AND tags AND status AND title)
	category, _ := objMap[objects.FieldKeyCategory].(string)
	status, _ := objMap[objects.FieldKeyStatus].(string)
	title, _ := objMap[objects.FieldKeyTitle].(string)
	var tags []string
	if tagsRaw, ok := objMap[objects.FieldKeyTags]; ok {
		if tagsList, ok := tagsRaw.([]any); ok {
			for _, tag := range tagsList {
				if tagStr, ok := tag.(string); ok {
					tags = append(tags, tagStr)
				}
			}
		} else if tagsList, ok := tagsRaw.([]string); ok {
			tags = tagsList
		}
	}

	// State 2: Lifecycle violation with valid precondition pattern
	// FAIL-FAST: Check precondition marker before processing
	if rule == objects.KindLifecycle {
		// State 4: FAIL-FAST - Missing precondition marker (non-determinable)
		if !strings.Contains(message, "Precondition not met") {
			return ""
		}

		// Match message against dynamically built precondition patterns
		pattern, matched := matchLifecyclePreconditionPattern(message)
		if !matched {
			// State 4: FAIL-FAST - Unknown lifecycle precondition pattern (non-determinable)
			// None of the known patterns matched, return empty string
			return ""
		}

		// Generate fix command based on matched pattern
		queryHint := buildQueryHint(pattern.TargetKind, category, tags, status, title)
		switch pattern.Operation {
		case "append":
			// Array field: use += operator
			return fmt.Sprintf("%s object update %s --field %s+=<%s:%s>", paths.CLICommandName, objID, pattern.FieldName, pattern.PlaceholderID, queryHint)
		case "set":
			// Single value field: use = operator
			return fmt.Sprintf("%s object update %s --field %s=<%s:%s>", paths.CLICommandName, objID, pattern.FieldName, pattern.PlaceholderID, queryHint)
		default:
			// Unknown operation type
			return ""
		}
	}

	// State 3: Required field violation
	// Format: "field_name: field is required"
	if rule == "required" {
		// For required fields, we'd need spec-based defaults
		// For now, just generate a placeholder command
		return fmt.Sprintf("%s object update %s --field %s=<VALUE>", paths.CLICommandName, objID, field)
	}

	// State 4: FAIL-FAST - Unreachable (rule type already checked above, but defensive)
	return ""
}

// buildQueryHint builds a query hint string from contextual information
// Format: "category=X&tags=Y,Z&status=W&title_pattern=T"
// These hints can be used later to build boolean filter chains for query resolution
func buildQueryHint(targetKind, category string, tags []string, status, title string) string {
	var parts []string
	if category != emptyValue {
		parts = append(parts, fmt.Sprintf("category=%s", category))
	}
	if len(tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags=%s", strings.Join(tags, ",")))
	}
	if status != emptyValue {
		when.When(func() bool { return targetKind == objects.KindPriorityPlan }).Then(func() {
			parts = append(parts, "status=active")
		}).OrElse(func() {
			parts = append(parts, fmt.Sprintf("status=%s", status))
		}).Run()
	}
	if title != emptyValue {
		// Use title pattern matching (substring match)
		parts = append(parts, fmt.Sprintf("title_pattern=%s", title))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "&")
}
