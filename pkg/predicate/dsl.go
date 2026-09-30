package predicate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// StandaloneGatePredicates lists canonical zero-argument predicate gates.
var StandaloneGatePredicates = map[string]struct{}{
	"tests_ok_per_customization":            {},
	"lint_ok_per_customization":             {},
	"criteria_linked_or_acceptance_present": {},
	"git_diff_nonempty_or_waiver":           {},
	"smoke_or_integration_evidence_present": {},
	"ci_required_checks_green_or_na":        {},
	"security_gate_ok_or_na":                {},
	"performance_gate_ok_or_na":             {},
	"publish_ack_present_if_public":         {},
	"standard_checks_pass":                  {},
	"title_body_cohesion":                   {},
}

var (
	metricRegex = regexp.MustCompile(`^([a-zA-Z0-9_]+)\s*(==|!=|<=|<|>=|>)\s*(-?[0-9]+(?:\.[0-9]+)?)$`)
)

// ValidatePredicateSyntax checks whether an expression conforms to the Kernel Predicate DSL grammar
// (or is a known legacy prose overlay expression that can be compiled to it).
func ValidatePredicateSyntax(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("empty predicate expression")
	}

	// 1. Check legacy prose overlay expressions (pre-split)
	if _, handled := CompilePrecondition(expr); handled {
		return nil
	}

	// 2. Support compound statements delimited by ';' or ','
	predicates, err := SplitPredicates(expr)
	if err != nil {
		return err
	}

	for _, p := range predicates {
		if err := validateSinglePredicate(p); err != nil {
			return err
		}
	}
	return nil
}

// SplitPredicates splits a compound predicate expression by ';' or ',' while preserving quoted strings.
func SplitPredicates(expr string) ([]string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty predicate expression")
	}

	var preds []string
	var cur strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range expr {
		switch r {
		case '\'', '"', '`':
			if inQuote && r == quoteChar {
				inQuote = false
			} else if !inQuote {
				inQuote = true
				quoteChar = r
			}
			cur.WriteRune(r)
		case ';', ',':
			if !inQuote {
				part := strings.TrimSpace(cur.String())
				if part != "" {
					preds = append(preds, part)
				}
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	part := strings.TrimSpace(cur.String())
	if part != "" {
		preds = append(preds, part)
	}

	if len(preds) == 0 {
		return nil, fmt.Errorf("no predicates found in expression: %q", expr)
	}
	return preds, nil
}

func validateSinglePredicate(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("empty predicate")
	}

	// 1. Check standalone gates
	if _, ok := StandaloneGatePredicates[p]; ok {
		return nil
	}

	// 2. Check legacy prose overlay expressions (compile check)
	if _, handled := CompilePrecondition(p); handled {
		return nil
	}

	// 3. Colon-delimited parameterized predicates
	name, arg, hasArg := strings.Cut(p, ":")
	name = strings.TrimSpace(name)
	arg = strings.TrimSpace(arg)

	if !hasArg || arg == "" {
		return fmt.Errorf("predicate %q requires argument", name)
	}

	switch name {
	case "object_exists":
		return validateIdentifier(arg, "object_exists")

	case "field_nonempty":
		// Syntax: field_nonempty:[id:]field
		if strings.Contains(arg, ":") {
			parts := strings.Split(arg, ":")
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return fmt.Errorf("field_nonempty with object ID must be 'field_nonempty:<object_id>:<field>'")
			}
			return nil
		}
		return validateIdentifier(arg, "field_nonempty")

	case "field_matches":
		// Syntax: field_matches:<field>:<regex>
		field, pattern, ok := strings.Cut(arg, ":")
		if !ok || strings.TrimSpace(field) == "" || strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("field_matches requires 'field_matches:<field>:<regex>'")
		}
		if _, err := regexp.Compile(strings.TrimSpace(pattern)); err != nil {
			return fmt.Errorf("field_matches regex invalid: %w", err)
		}
		return nil

	case "path_exists", "content_hash_matches":
		if len(arg) == 0 {
			return fmt.Errorf("%s requires non-empty path", name)
		}
		return nil

	case "content_size_positive":
		return validatePathOrIdentifier(arg, "content_size_positive")

	case "query_metric":
		if !metricRegex.MatchString(arg) {
			return fmt.Errorf("query_metric syntax must be 'query_metric:<metric_name><op><value>', got %q", arg)
		}
		return nil

	case "command_exit_code":
		if len(arg) == 0 {
			return fmt.Errorf("command_exit_code requires command string")
		}
		return nil

	case "ast_semantic_match":
		// Syntax: ast_semantic_match:<path>:<constraint>
		parts := strings.Split(arg, ":")
		if len(parts) < 2 {
			return fmt.Errorf("ast_semantic_match requires 'ast_semantic_match:<path>:<constraint>'")
		}
		constraint := strings.TrimSpace(parts[len(parts)-2])
		if constraint != "symbol_present" && constraint != "symbol_absent" && constraint != "type_implements" && constraint != "no_raw_panics" {
			// check if last part is no_raw_panics
			if strings.TrimSpace(parts[len(parts)-1]) == "no_raw_panics" {
				return nil
			}
			return fmt.Errorf("ast_semantic_match unknown constraint: %s", constraint)
		}
		return nil

	case "criteria_linked_or_acceptance_present":
		return nil

	case "title_body_cohesion":
		if _, err := strconv.Atoi(arg); err != nil {
			return fmt.Errorf("title_body_cohesion requires integer minimum shared stems, got %q", arg)
		}
		return nil

	default:
		return fmt.Errorf("unknown predicate %q; must match kernel_predicate_dsl.ebnf", name)
	}
}

func validateIdentifier(id, predType string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%s argument cannot be empty", predType)
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("%s argument %q contains invalid characters", predType, id)
		}
	}
	return nil
}

func validatePathOrIdentifier(val, predType string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return fmt.Errorf("%s argument cannot be empty", predType)
	}
	for _, r := range val {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '/' || r == '.') {
			return fmt.Errorf("%s argument %q contains invalid characters", predType, val)
		}
	}
	return nil
}

// CompilePrecondition compiles legacy prose into canonical Kernel Predicate DSL expressions.
// Returns the canonical predicate string and true if handled, or ("", false) if not recognized.
func CompilePrecondition(p string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(p))
	if lower == "" {
		return "", true
	}

	// 1. Standard checks pass
	if strings.Contains(lower, "standard checks pass") {
		return "standard_checks_pass", true
	}

	// 2. Doc entry file reachability
	if strings.Contains(lower, "target document file exists and is reachable on disk") ||
		strings.Contains(lower, "target file reachable and readable") {
		return "path_exists:file_path", true
	}

	// 3. Doc entry metadata populated
	if strings.Contains(lower, "title, summary, and path are populated") {
		return "field_nonempty:title;field_nonempty:summary;field_nonempty:path", true
	}

	// 4. Content hash checks
	if strings.Contains(lower, "cryptographic content_hash computed and sealed") {
		return "field_nonempty:content_hash", true
	}
	if strings.Contains(lower, "cryptographic content_hash matches target file on disk") {
		return "content_hash_matches:file_path", true
	}

	// 5. Content size measured
	if strings.Contains(lower, "document content_size measured") || strings.Contains(lower, "content_size measured") {
		return "content_size_positive:content_size", true
	}

	// 6. Generic field population patterns: "<field> is populated", "<f1>, <f2> are populated"
	if strings.HasSuffix(lower, "is populated") || strings.HasSuffix(lower, "are populated") {
		clause := strings.TrimSuffix(lower, "is populated")
		clause = strings.TrimSuffix(clause, "are populated")
		clause = strings.ReplaceAll(clause, " and ", ",")
		parts := strings.Split(clause, ",")
		var canonicals []string
		for _, part := range parts {
			f := strings.TrimSpace(part)
			if f != "" {
				canonicals = append(canonicals, "field_nonempty:"+f)
			}
		}
		if len(canonicals) > 0 {
			return strings.Join(canonicals, ";"), true
		}
	}

	return "", false
}

// ParseMetricPredicate extracts the metric name, operator, and threshold from a query_metric predicate argument.
func ParseMetricPredicate(arg string) (name, op string, val float64, err error) {
	m := metricRegex.FindStringSubmatch(strings.TrimSpace(arg))
	if len(m) != 4 {
		return "", "", 0, fmt.Errorf("invalid query_metric syntax: %q", arg)
	}
	f, err := strconv.ParseFloat(m[3], 64)
	if err != nil {
		return "", "", 0, err
	}
	return m[1], m[2], f, nil
}
