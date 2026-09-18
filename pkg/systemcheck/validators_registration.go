package systemcheck

import (
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TRACK: BLI-CEF-ARCH-SYSTEM-TRANCHE1 — registration/lifecycle check helpers
// extracted from cmd/zqk/system (F-ARCH-001).

const emptyValue = ""

// CheckRegistration validates id/kind/format and hand-CAS provenance on a parsed object.
func CheckRegistration(obj *parser.ParsedObject, kind string) []Issue {
	var issues []Issue

	if obj.ID == emptyValue {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "registration",
			Message:  "Missing required field: id",
		})
	}

	if obj.Kind == emptyValue {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "registration",
			Message:  "Missing required field: kind",
		})
	}

	objKindValue := strings.TrimSpace(obj.Kind)
	separators := []string{" - ", " -", "- "}
	for _, sep := range separators {
		if idx := strings.Index(objKindValue, sep); idx > 0 {
			objKindValue = strings.TrimSpace(objKindValue[:idx])
			break
		}
	}
	objKindNormalized := strings.ToLower(objKindValue)
	expectedKindNormalized := strings.ToLower(strings.TrimSpace(kind))

	if objKindNormalized == expectedKindNormalized {
		if strings.TrimSpace(obj.Kind) != strings.TrimSpace(kind) {
			issues = append(issues, Issue{
				Tier:     3,
				Category: "registration",
				Message:  fmt.Sprintf("Kind field contains extra text: %s (expected: %s)", obj.Kind, kind),
			})
		}
	} else if objKindNormalized != emptyValue {
		objKindDir := objects.GetDirectoryFromKind(objKindValue)
		expectedKindDir := objects.GetDirectoryFromKind(kind)
		if objKindDir == emptyValue || objKindDir != expectedKindDir {
			issues = append(issues, Issue{
				Tier:     2,
				Category: "registration",
				Message:  fmt.Sprintf("Kind mismatch: expected %s, got %s", kind, obj.Kind),
			})
		}
	}

	idValidator := validation.GetIDValidator()
	_ = idValidator.LoadPatterns() //nolint:errcheck // defaults if load fails
	valid, err := idValidator.ValidateID(obj.ID, kind)
	if err != nil {
		issues = append(issues, Issue{
			Tier:     2,
			Category: "registration",
			Message:  fmt.Sprintf("ID validation error: %v", err),
		})
	} else if !valid {
		validPrefixes := idValidator.GetValidPrefixes(kind)
		prefixMsg := ""
		if len(validPrefixes) > 0 {
			prefixMsg = fmt.Sprintf(" (valid prefixes: %v)", validPrefixes)
		}
		issues = append(issues, Issue{
			Tier:     2,
			Category: "registration",
			Message:  fmt.Sprintf("ID format does not match kind %s%s", kind, prefixMsg),
		})
	}

	if LooksHandCASMaterialized(obj) {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "registration",
			Message:  "Object appears to be hand-CAS materialized (missing created_by and updated_by metadata). Use CLI to create/update objects.",
		})
	}

	return issues
}

// LooksHandCASMaterialized reports whether an object carries neither created_by nor updated_by.
func LooksHandCASMaterialized(obj *parser.ParsedObject) bool {
	return !hasNonEmptyStringProperty(obj, objects.FieldKeyCreatedBy) &&
		!hasNonEmptyStringProperty(obj, objects.FieldKeyUpdatedBy)
}

func hasNonEmptyStringProperty(obj *parser.ParsedObject, key string) bool {
	if obj == nil {
		return false
	}
	value, ok := obj.Properties[key].(string)
	return ok && value != emptyValue
}

// CheckLifecycle validates status presence and focused lifecycle rules via the global validator.
func CheckLifecycle(obj *parser.ParsedObject, kind string) []Issue {
	var issues []Issue

	status, ok := obj.Properties[objects.FieldKeyStatus].(string)
	if !ok || status == emptyValue {
		issues = append(issues, Issue{
			Tier:     2,
			Category: objects.KindLifecycle,
			Message:  "Missing or invalid status field",
		})
		return issues
	}

	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")
	if validator != nil {
		objMap := make(map[string]any)
		if obj.Properties != nil {
			objMap = obj.Properties
		}

		options := validation.DefaultValidationOptions()
		options.ValidateLifecycle = true
		options.CurrentState = status

		result, err := validator.Validate(pkgctx.NewSystemContext(), objMap, kind, options)
		if err == nil {
			for _, validationError := range result.Errors {
				if validationError.Rule == objects.KindLifecycle {
					issues = append(issues, Issue{
						Tier:     1,
						Category: objects.KindLifecycle,
						Message:  fmt.Sprintf("%s: %s", validationError.Field, validationError.Message),
					})
				}
			}
		}
	}

	return issues
}

// CheckLifecycleWithLoader validates status against a LifecycleLoader and suggests fixes.
func CheckLifecycleWithLoader(obj *parser.ParsedObject, kind string, lifecycleLoader *objects.LifecycleLoader) []Issue {
	var issues []Issue

	status, ok := obj.Properties[objects.FieldKeyStatus].(string)
	if !ok || status == emptyValue {
		issues = append(issues, Issue{
			Tier:     2,
			Category: objects.KindLifecycle,
			Message:  "Missing or invalid status field",
		})
		return issues
	}

	valid, err := lifecycleLoader.IsValidStatus(kind, status)
	if err == nil && !valid {
		issue := Issue{
			Tier:     1,
			Category: objects.KindLifecycle,
			Message:  fmt.Sprintf("Invalid lifecycle status '%s' for kind '%s'", status, kind),
		}
		if obj.ID != emptyValue {
			suggested, _ := lifecycleLoader.GetOriginStatus(kind)
			if suggested == emptyValue {
				if lc, loadErr := lifecycleLoader.LoadLifecycle(kind); loadErr == nil && len(lc.Statuses) > 0 {
					suggested = lc.Statuses[0].Value
				}
			}
			if suggested != emptyValue {
				issue.FixCommand = fmt.Sprintf("%s object update %s --field status=%s", paths.CLICommandName, obj.ID, suggested)
				issue.AutoFixable = true
			}
		}
		issues = append(issues, issue)
	}

	return issues
}
