package workflow

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

var (
	// ErrSuperficialContent is returned when content fails anti-superficiality checks.
	ErrSuperficialContent = errors.New("anti-superficiality violation: content contains vague or unmeasurable directives")

	// ErrThreeFoldFormulaUnsatisfied is returned when requirements lack 3-fold proof criteria (invariant, dynamic, adversarial).
	ErrThreeFoldFormulaUnsatisfied = errors.New("ontological validation failed: requirement does not satisfy the Three-Fold Proof Formula")
)

// Banned vague phrases that signal superficial requirements or criteria.
var bannedVagueRegexps = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:make\s+the\s+)?code\s+(?:is\s+)?better\b`),
	regexp.MustCompile(`(?i)\b(?:make\s+it\s+)?cleaner\b`),
	regexp.MustCompile(`(?i)\bcode\s+(?:has\s+)?changed\b`),
	regexp.MustCompile(`(?i)\bimprove\s+(?:the\s+)?code\b`),
	regexp.MustCompile(`(?i)\bfix\s+things\b`),
	regexp.MustCompile(`(?i)\bvarious\s+improvements\b`),
	regexp.MustCompile(`(?i)\bgood\s+enough\b`),
	regexp.MustCompile(`(?i)\bworks\s+fine\b`),
}

// FormulaCategory classifies a criteria object into the Three-Fold Formula.
type FormulaCategory string

const (
	FormulaInvariant   FormulaCategory = "invariant"
	FormulaDynamic     FormulaCategory = "dynamic"
	FormulaAdversarial FormulaCategory = "adversarial"
	FormulaUnknown     FormulaCategory = "unknown"
)

// LintViolation describes a single quality defect detected by the linter.
type LintViolation struct {
	ObjectID string `json:"object_id"`
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "blocker" or "warning"
}

// LintResult contains the aggregate outcome of a linting pass.
type LintResult struct {
	Passed     bool            `json:"passed"`
	Violations []LintViolation `json:"violations"`
}

// ClassifyFormulaCategory inspects a criteria object to determine its 3-Fold proof category.
func ClassifyFormulaCategory(crit map[string]any) FormulaCategory {
	if crit == nil {
		return FormulaUnknown
	}

	// 1. Check explicit field "formula_type" or "category"
	for _, field := range []string{"formula_type", "category", "proof_type", "type"} {
		if val, ok := crit[field].(string); ok {
			norm := strings.ToLower(strings.TrimSpace(val))
			switch norm {
			case "invariant", "state_invariant", "static":
				return FormulaInvariant
			case "dynamic", "dynamic_behavior", "operational", "functional":
				return FormulaDynamic
			case "adversarial", "negative_invariant", "fail_closed", "boundary":
				return FormulaAdversarial
			}
		}
	}

	// 2. Fall back to title / ID / description heuristics
	var corpus strings.Builder
	for _, field := range []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyDescription, "statement"} {
		if val, ok := crit[field].(string); ok {
			corpus.WriteString(" ")
			corpus.WriteString(strings.ToLower(val))
		}
	}
	text := corpus.String()

	if strings.Contains(text, "adversarial") || strings.Contains(text, "fail-closed") || strings.Contains(text, "negative") ||
		strings.Contains(text, "reject") || strings.Contains(text, "disallow") || strings.Contains(text, "malformed") {
		return FormulaAdversarial
	}
	if strings.Contains(text, "invariant") || strings.Contains(text, "schema") || strings.Contains(text, "predicate") ||
		strings.Contains(text, "conforms") || strings.Contains(text, "static") {
		return FormulaInvariant
	}
	if strings.Contains(text, "dynamic") || strings.Contains(text, "behavior") || strings.Contains(text, "execut") ||
		strings.Contains(text, "runs") || strings.Contains(text, "assert") {
		return FormulaDynamic
	}

	return FormulaUnknown
}

// LintText scans text for banned superficial phrases.
func LintText(text string) (string, bool) {
	for _, re := range bannedVagueRegexps {
		if loc := re.FindString(text); loc != "" {
			return loc, true
		}
	}
	return "", false
}

// LintObject evaluates a single kernel object for superficial content.
func LintObject(obj map[string]any) *LintResult {
	res := &LintResult{Passed: true}
	if obj == nil {
		return res
	}

	id, _ := obj[objects.FieldKeyID].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)

	fieldsToCheck := []string{
		objects.FieldKeyTitle,
		objects.FieldKeyDescription,
		"statement",
		"acceptance_criteria",
		"rationale",
	}

	for _, f := range fieldsToCheck {
		if val, ok := obj[f].(string); ok {
			if matched, found := LintText(val); found {
				res.Passed = false
				res.Violations = append(res.Violations, LintViolation{
					ObjectID: id,
					Kind:     kind,
					Field:    f,
					Rule:     "banned_vague_phrase",
					Message:  fmt.Sprintf("field %q contains banned superficial phrase %q", f, matched),
					Severity: "blocker",
				})
			}
		}
	}

	return res
}

// LintRequirementWithCriteria rigorously evaluates a requirement and its linked criteria
// against the Three-Fold Proof Formula (Invariant, Dynamic, Adversarial).
func LintRequirementWithCriteria(req map[string]any, criteria []map[string]any) *LintResult {
	res := LintObject(req)
	reqID, _ := req[objects.FieldKeyID].(string)

	// Check criteria text for superficial phrases
	for _, c := range criteria {
		cRes := LintObject(c)
		if !cRes.Passed {
			res.Passed = false
			res.Violations = append(res.Violations, cRes.Violations...)
		}
	}

	// Shovel-Ready Gate requires at least 3 criteria matching the Three-Fold Formula
	if len(criteria) < 3 {
		res.Passed = false
		res.Violations = append(res.Violations, LintViolation{
			ObjectID: reqID,
			Kind:     "requirement",
			Field:    objects.FieldKeyCriteriaRefs,
			Rule:     "three_fold_formula_count",
			Message:  fmt.Sprintf("requirement %s has only %d criteria; requires at least 3 matching the Three-Fold Formula", reqID, len(criteria)),
			Severity: "blocker",
		})
	}

	// Classify coverage
	hasInvariant := false
	hasDynamic := false
	hasAdversarial := false

	for _, c := range criteria {
		cat := ClassifyFormulaCategory(c)
		switch cat {
		case FormulaInvariant:
			hasInvariant = true
		case FormulaDynamic:
			hasDynamic = true
		case FormulaAdversarial:
			hasAdversarial = true
		}
	}

	var missing []string
	if !hasInvariant {
		missing = append(missing, string(FormulaInvariant))
	}
	if !hasDynamic {
		missing = append(missing, string(FormulaDynamic))
	}
	if !hasAdversarial {
		missing = append(missing, string(FormulaAdversarial))
	}

	if len(missing) > 0 {
		res.Passed = false
		res.Violations = append(res.Violations, LintViolation{
			ObjectID: reqID,
			Kind:     "requirement",
			Field:    "three_fold_formula",
			Rule:     "three_fold_formula_missing_dimensions",
			Message:  fmt.Sprintf("requirement %s is missing 3-Fold dimensions: %s", reqID, strings.Join(missing, ", ")),
			Severity: "blocker",
		})
	}

	return res
}

// CheckShovelReadyQuality validates that a requirement and its linked criteria satisfy
// the Anti-Superficiality Gate before promotion to shovel_ready.
func CheckShovelReadyQuality(req map[string]any, criteria []map[string]any) error {
	result := LintRequirementWithCriteria(req, criteria)
	if !result.Passed {
		var msgs []string
		for _, v := range result.Violations {
			if v.Severity == "blocker" {
				msgs = append(msgs, fmt.Sprintf("[%s:%s] %s", v.ObjectID, v.Field, v.Message))
			}
		}
		return fmt.Errorf("%w: %s", ErrThreeFoldFormulaUnsatisfied, strings.Join(msgs, "; "))
	}
	return nil
}
