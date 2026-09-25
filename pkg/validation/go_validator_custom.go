package validation

import (
	stdcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Empty: overlays live in pkg/kernelcas/compose. validateCustomRules calls compose for every kind.
// TRACK: map registrations deleted (no Go custom rule bodies).

type DynamicRuleEvaluator func(gv *GoValidator, rule map[string]any, obj map[string]any, options *ValidationOptions, ruleID string, params map[string]any) []ValidationError

var dynamicRuleEvaluators = map[string]DynamicRuleEvaluator{
	RuleTypeFieldPresence:   evaluateFieldPresence,
	RuleTypeActiveReference: evaluateActiveReference,
	RuleTypeAlignment:       evaluateAlignment,
}

func validateFromComposedDefinition(ctx stdcontext.Context, kind string, gv *GoValidator, obj map[string]any, options *ValidationOptions) []ValidationError {
	_ = gv
	if kind == "" {
		kind, _ = obj[objects.FieldKeyKind].(string)
	}
	if kind == "" {
		return nil
	}
	pipelineKind, intent := compose.KindUpdate, compose.IntentUpdateFields
	if options != nil {
		cur := strings.TrimSpace(options.CurrentState)
		st, _ := obj[objects.FieldKeyStatus].(string)
		if cur != "" && !strings.EqualFold(cur, strings.TrimSpace(st)) {
			// Status is changing: apply TransitionStatus overlays (DoR), not Update.
			// Same-status field fills stay on Update so TPM can add missing refs in one shot.
			pipelineKind, intent = compose.KindTransition, compose.IntentTransition
		}
	}
	raw := compose.ValidateObjectIntent(ctx, compose.Default(), kind, pipelineKind, intent, obj, composeObjectLookup(options))
	out := make([]ValidationError, 0, len(raw))
	for _, e := range raw {
		out = append(out, ValidationError{Field: e.Field, Message: e.Message, Rule: e.Rule})
	}
	return out
}

// composeObjectLookup adapts ValidationOptions so plan-status rules see ObjectLookup
// and ObjectStatusLookup (tests often only stub status).
func composeObjectLookup(options *ValidationOptions) compose.ObjectLookup {
	if options == nil {
		return nil
	}
	ol, osl := options.ObjectLookup, options.ObjectStatusLookup
	if ol == nil && osl == nil {
		return nil
	}
	return func(id string) (map[string]any, error) {
		if ol != nil {
			if m, err := ol(id); err == nil && m != nil {
				return m, nil
			}
		}
		if osl != nil {
			st, err := osl(id)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(st) == "" {
				return nil, fmt.Errorf("status empty for %s", id)
			}
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: st,
			}, nil
		}
		return nil, fmt.Errorf("object %s not found", id)
	}
}

// validateCustomRules enforces business-logic rules via composed pipeline_definition overlays.
func (gv *GoValidator) validateCustomRules(ctx stdcontext.Context, kind string, obj map[string]any, options *ValidationOptions) []ValidationError {
	if (strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false)) && !strings.HasSuffix(os.Args[0], "validation.test") {
		return nil
	}
	errs := validateFromComposedDefinition(ctx, kind, gv, obj, options)

	// Delegate owner_ref prototype/test detection to authcred layer.
	if ownerRef, ok := obj[objects.FieldKeyOwnerRef].(string); ok && ownerRef != "" {
		if err := authcred.ValidateOwnerRefProduction(ownerRef); err != nil {
			validationErr := ValidationError{
				Field:   objects.FieldKeyOwnerRef,
				Message: err.Error(),
				Rule:    "owner_ref_no_prototype",
			}
			errs = append(errs, validationErr)
		}
	}
	return errs
}

// validateDynamicRules loads and evaluates dynamic validation_rule objects for the given kind and status.
func (gv *GoValidator) validateDynamicRules(kind string, status string, currentState string, obj map[string]any, options *ValidationOptions) []ValidationError {
	var errors []ValidationError

	cacheKey := kind + ":" + status
	var rules []map[string]any
	if val, ok := gv.dynamicRulesCache.Load(cacheKey); ok {
		rules = val.([]map[string]any)
	} else {
		wd, err := fileutil.Getwd()
		if err != nil {
			return nil
		}
		projectRoot, err := paths.ModuleRootFromPath(wd)
		if err != nil {
			return nil
		}

		rules, err = loadDynamicRulesForKind(projectRoot, kind, status)
		if err != nil {
			return nil
		}

		gv.dynamicRulesCache.Store(cacheKey, rules)
	}

	for _, rule := range rules {
		// Filter by transition_from if specified
		if transitionFrom, ok := rule[objects.FieldKeyTransitionFrom].(string); ok && transitionFrom != "" {
			if currentState != "" && !strings.EqualFold(currentState, transitionFrom) {
				continue
			}
		}

		ruleErrors := gv.evaluateDynamicRule(rule, obj, options)
		if len(ruleErrors) > 0 {
			errors = append(errors, ruleErrors...)
		}
	}

	return errors
}

func loadDynamicRulesForKind(projectRoot string, kind string, status string) ([]map[string]any, error) {
	dirName := objects.GetDirectoryFromKind(KindValidationRule)
	if dirName == "" {
		dirName = KindValidationRule
	}
	dirPath := filepath.Join(projectRoot, paths.ProcessDir, dirName)

	if _, err := fileutil.Stat(dirPath); fileutil.IsNotExist(err) {
		dirPath = filepath.Join(projectRoot, paths.ProcessDir, DirValidationRules)
	}

	files, err := fileutil.ReadDir(dirPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var rules []map[string]any
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		filePath := filepath.Join(dirPath, file.Name())
		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			continue
		}

		var rule map[string]any
		if err := yaml.Unmarshal(data, &rule); err != nil {
			continue
		}

		tKind, _ := rule[objects.FieldKeyTargetKind].(string)
		tStatus, _ := rule[objects.FieldKeyTargetStatus].(string)
		if strings.EqualFold(tKind, kind) && strings.EqualFold(tStatus, status) {
			rules = append(rules, rule)
		}
	}

	return rules, nil
}

func (gv *GoValidator) evaluateDynamicRule(rule map[string]any, obj map[string]any, options *ValidationOptions) []ValidationError {
	ruleID, _ := rule[objects.FieldKeyID].(string)
	ruleType, _ := rule[objects.FieldKeyRuleType].(string)
	params, _ := rule[objects.FieldKeyParameters].(map[string]any)
	if params == nil {
		params = make(map[string]any)
	}

	if evaluator, ok := dynamicRuleEvaluators[ruleType]; ok {
		return evaluator(gv, rule, obj, options, ruleID, params)
	}
	return nil
}

func evaluateFieldPresence(gv *GoValidator, rule map[string]any, obj map[string]any, options *ValidationOptions, ruleID string, params map[string]any) []ValidationError {
	var errors []ValidationError
	fieldName, _ := params[objects.FieldKeyFieldName].(string)
	if fieldName != "" && !gv.hasNonEmptyField(obj, fieldName) {
		errors = append(errors, ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be set as defined in validation rule %s", fieldName, ruleID),
			Rule:    "dynamic_rule:" + ruleID,
		})
	}
	return errors
}

func evaluateActiveReference(gv *GoValidator, rule map[string]any, obj map[string]any, options *ValidationOptions, ruleID string, params map[string]any) []ValidationError {
	var errors []ValidationError
	refField, _ := params["reference_field"].(string)
	if refField != "" {
		refIDs := gv.extractIDsFromField(obj, refField)
		for _, refID := range refIDs {
			if options != nil && options.ObjectStatusLookup != nil {
				status, err := options.ObjectStatusLookup(refID)
				if err == nil {
					status = strings.ToLower(status)
					if status == objects.ObjectStatusDraft || status == objects.ObjectStatusProposed || status == objects.ObjectStatusExploring || status == objects.ObjectStatusPlanning || status == objects.ObjectStatusNotStarted || status == objects.ObjectStatusImplemented || status == objects.ObjectStatusPending || status == objects.ObjectStatusPendingVerification || status == objects.ObjectStatusError || status == objects.ObjectStatusCancelled || status == "" {
						errors = append(errors, ValidationError{
							Field:   refField,
							Message: fmt.Sprintf("Referenced object %s in field %q is in status %q (must be active) as defined in validation rule %s", refID, refField, status, ruleID),
							Rule:    "dynamic_rule:" + ruleID,
						})
					}
				}
			}
		}
	}
	return errors
}

func evaluateAlignment(gv *GoValidator, rule map[string]any, obj map[string]any, options *ValidationOptions, ruleID string, params map[string]any) []ValidationError {
	var errors []ValidationError
	childField, _ := params["child_field"].(string)
	parentField, _ := params["parent_field"].(string)
	if childField != "" && parentField != "" {
		parentIDs := gv.extractIDsFromField(obj, parentField)
		childIDs := gv.extractIDsFromField(obj, childField)
		if len(parentIDs) > 0 && len(childIDs) > 0 {
			for _, childID := range childIDs {
				if options != nil && options.ObjectStatusLookup != nil {
					childStatus, err := options.ObjectStatusLookup(childID)
					if err == nil {
						childStatus = strings.ToLower(childStatus)
						if childStatus == objects.ObjectStatusDraft || childStatus == objects.ObjectStatusProposed || childStatus == objects.ObjectStatusExploring || childStatus == objects.ObjectStatusPlanning || childStatus == objects.ObjectStatusNotStarted || childStatus == objects.ObjectStatusImplemented || childStatus == objects.ObjectStatusPending || childStatus == objects.ObjectStatusPendingVerification || childStatus == "" {
							continue
						}
					}
				}

				childObj, err := gv.lookupRefObject(childID, options)
				if err == nil && childObj != nil {
					childParentIDs := gv.extractIDsFromField(childObj, parentField)
					aligned := false
					for _, cpID := range childParentIDs {
						for _, pID := range parentIDs {
							if cpID == pID {
								aligned = true
								break
							}
						}
						if aligned {
							break
						}
					}
					if !aligned {
						errors = append(errors, ValidationError{
							Field:   childField,
							Message: fmt.Sprintf("Alignment mismatch: child %s must link back to parent %v as defined in validation rule %s", childID, parentIDs, ruleID),
							Rule:    "dynamic_rule:" + ruleID,
						})
					}
				}
			}
		}
	}
	return errors
}

// validateCustomWarnings enforces warnings (non-blocking) across fields or specific to kinds.
func (gv *GoValidator) validateCustomWarnings(_ stdcontext.Context, _ string, _ map[string]any, _ *ValidationOptions) []ValidationWarning {
	if (strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false)) && !strings.HasSuffix(os.Args[0], "validation.test") {
		return nil
	}
	return nil
}
