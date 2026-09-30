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
	"path_exists":                           {},
	"content_hash_matches":                  {},
	"content_size_positive":                 {},
	"shovel_ready":                          {},
	"tdd_test_red_phase":                    {},
	"criteria_active_test_case":             {},
	"ready_backlog_references_plan":         {},
	"linked_backlog_ready_or_later":         {},
	"linked_backlog_all_terminal":           {},
	"no_linked_backlog_in_progress_or_complete": {},
	"workflow_constraints_if_set":           {},
	"priority_plan_validated":               {},
	"team_or_persona_dispatch_refs":         {},
	"git_mutation_evidence_present":         {},
	"branch_is_ancestor_of_trunk":           {},
	"machine_checkable_closure_evidence":    {},
	"linked_criteria_validated_or_complete": {},
	"priority_plan_archived_when_set":       {},
	"priority_plan_execution_facing":        {},
	"work_done":                             {},
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
		case ';':
			if !inQuote {
				part := strings.TrimSpace(cur.String())
				if part != "" {
					preds = append(preds, part)
				}
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		case ',':
			if !inQuote && !strings.HasPrefix(strings.TrimSpace(cur.String()), "any_nonempty:") {
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

	case "field_cleared":
		// Syntax: field_cleared:[id:]field
		if strings.Contains(arg, ":") {
			parts := strings.Split(arg, ":")
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return fmt.Errorf("field_cleared with object ID must be 'field_cleared:<object_id>:<field>'")
			}
			return nil
		}
		return validateIdentifier(arg, "field_cleared")

	case "any_nonempty":
		fields := strings.FieldsFunc(arg, func(r rune) bool { return r == ',' || r == ':' })
		if len(fields) == 0 {
			return fmt.Errorf("any_nonempty requires at least one field identifier")
		}
		for _, f := range fields {
			if err := validateIdentifier(strings.TrimSpace(f), "any_nonempty"); err != nil {
				return err
			}
		}
		return nil

	case "role_is":
		return validateIdentifier(arg, "role_is")

	case "active_ref":
		return validateIdentifier(arg, "active_ref")

	case "link_back":
		// Syntax: link_back:<subject_field>:<target_field>
		parts := strings.Split(arg, ":")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("link_back requires 'link_back:<subject_field>:<target_field>'")
		}
		if err := validateIdentifier(parts[0], "link_back subject"); err != nil {
			return err
		}
		return validateIdentifier(parts[1], "link_back target")

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

	case "at_least":
		countStr, field, ok := strings.Cut(arg, ":")
		if !ok {
			return fmt.Errorf("at_least predicate requires 'at_least:<count>:<field>', got %q", arg)
		}
		count, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil || count < 0 {
			return fmt.Errorf("at_least count must be non-negative integer, got %q", countStr)
		}
		return validateIdentifier(strings.TrimSpace(field), "at_least")

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

	// 7. Role postcondition: "role is <role>"
	if strings.HasPrefix(lower, "role is ") {
		role := strings.TrimSpace(strings.TrimPrefix(lower, "role is "))
		if role != "" && !strings.Contains(role, " ") {
			return "role_is:" + role, true
		}
	}

	// 8. Field cleared postcondition: "<field> is cleared", "<field> is unset", "clear <field>", "unset <field>"
	if strings.HasSuffix(lower, " is cleared") {
		f := strings.TrimSpace(strings.TrimSuffix(lower, " is cleared"))
		if f != "" && !strings.Contains(f, " ") {
			return "field_cleared:" + f, true
		}
	}
	if strings.HasSuffix(lower, " is unset") {
		f := strings.TrimSpace(strings.TrimSuffix(lower, " is unset"))
		if f != "" && !strings.Contains(f, " ") {
			return "field_cleared:" + f, true
		}
	}
	if strings.HasPrefix(lower, "clear ") {
		f := strings.TrimSpace(strings.TrimPrefix(lower, "clear "))
		if f != "" && !strings.Contains(f, " ") {
			return "field_cleared:" + f, true
		}
	}
	if strings.HasPrefix(lower, "unset ") {
		f := strings.TrimSpace(strings.TrimPrefix(lower, "unset "))
		if f != "" && !strings.Contains(f, " ") {
			return "field_cleared:" + f, true
		}
	}

	// 9. Field is set / not empty: "<field> is set", "<field> is not empty"
	if strings.HasSuffix(lower, " is set") {
		f := strings.TrimSpace(strings.TrimSuffix(lower, " is set"))
		if f != "" && !strings.Contains(f, " ") {
			return "field_nonempty:" + f, true
		}
	}
	if strings.HasSuffix(lower, " is not empty") {
		f := strings.TrimSpace(strings.TrimSuffix(lower, " is not empty"))
		if f != "" && !strings.Contains(f, " ") {
			return "field_nonempty:" + f, true
		}
	}

	// 10. Owner identified / set
	if lower == "owner identified" || lower == "owner is set" {
		return "field_nonempty:owner_ref", true
	}

	// 11. Priority assigned
	if lower == "priority assigned" {
		return "field_matches:priority:^(high|medium|low)$", true
	}

	// 12. Problem statement and acceptance considerations populated
	if strings.Contains(lower, "problem statement") && strings.Contains(lower, "acceptance") {
		return "field_nonempty:problem_statement;field_nonempty:acceptance_considerations", true
	}

	// 13. Terminal work done
	if lower == "work_done" || lower == "work is done" {
		return "work_done", true
	}

	// 14. Shovel ready
	if lower == "shovel ready" || lower == "shovel_ready" {
		return "shovel_ready", true
	}

	// 15. TDD red phase
	if strings.HasPrefix(lower, "all linked criteria_refs bound to active test_case_refs") || lower == "tdd red phase" {
		return "tdd_test_red_phase", true
	}

	// 16. Criteria linked to active test case
	if strings.HasPrefix(lower, "must link to an active test_case") || strings.HasPrefix(lower, "at least one active test_case_ref linked") {
		return "criteria_active_test_case", true
	}

	// 17. Priority plan child / status gates
	if strings.HasPrefix(lower, "at least one ready backlog_item references this plan") {
		return "ready_backlog_references_plan", true
	}
	if strings.HasPrefix(lower, "all linked backlog_items referencing this plan are ready or later") {
		return "linked_backlog_ready_or_later", true
	}
	if strings.HasPrefix(lower, "all linked backlog_items referencing this plan are terminal") {
		return "linked_backlog_all_terminal", true
	}
	if strings.HasPrefix(lower, "no linked backlog_items referencing this plan are in progress or complete") {
		return "no_linked_backlog_in_progress_or_complete", true
	}
	if strings.HasPrefix(lower, "workflow constraints validated") {
		return "workflow_constraints_if_set", true
	}
	if lower == "priority plan validated" {
		return "priority_plan_validated", true
	}
	if strings.HasPrefix(lower, "at least one team_configuration_ref or persona_refs") {
		return "team_or_persona_dispatch_refs", true
	}

	// 18. Evidence gates
	if strings.HasPrefix(lower, "commit_hashes have git mutation evidence") || strings.HasPrefix(lower, "commit_refs have git mutation evidence") {
		return "git_mutation_evidence_present", true
	}
	if strings.HasPrefix(lower, "branch_name is an ancestor of trunk") || strings.HasPrefix(lower, "branch_ref is an ancestor of trunk") {
		return "branch_is_ancestor_of_trunk", true
	}
	if strings.HasPrefix(lower, "machine-checkable evidence with green scheduler fingerprint") {
		return "machine_checkable_closure_evidence", true
	}

	// 19. Ref status rules
	if strings.HasPrefix(lower, "all linked criteria_refs are validated or complete") {
		return "linked_criteria_validated_or_complete", true
	}
	if strings.HasPrefix(lower, "linked priority_plan is archived when priority_plan_ref is set") {
		return "priority_plan_archived_when_set", true
	}
	if strings.HasPrefix(lower, "priority_plan_ref target must be in active or in_progress status") {
		return "priority_plan_execution_facing", true
	}

	// 20. Active refs
	if strings.Contains(lower, "at least one active ") && strings.Contains(lower, "linked") {
		for _, w := range strings.Fields(lower) {
			clean := strings.Trim(w, ",.()[]{}'")
			if strings.HasSuffix(clean, "_ref") || strings.HasSuffix(clean, "_refs") {
				return "active_ref:" + clean, true
			}
		}
	}

	// 21. Link back alignment
	if strings.Contains(lower, "link back to") || strings.Contains(lower, "links back to") || strings.Contains(lower, "belongs to") {
		if subj, tgt, ok := parseLinkBackTokens(lower); ok {
			return "link_back:" + subj + ":" + tgt, true
		}
	}

	// 22. Disjunctions: "at least one X or Y linked" / "at least one X or Y"
	if strings.HasPrefix(lower, "at least ") && strings.Contains(lower, " or ") {
		var fields []string
		for _, w := range strings.Fields(lower) {
			clean := strings.Trim(w, ",.()[]{}'")
			if strings.HasSuffix(clean, "_ref") || strings.HasSuffix(clean, "_refs") {
				fields = append(fields, clean)
			}
		}
		if len(fields) >= 2 {
			return "any_nonempty:" + strings.Join(fields, ","), true
		}
	}

	// 23. Generic at least <N> <field> linked
	if strings.HasPrefix(lower, "at least ") {
		for _, w := range strings.Fields(lower) {
			clean := strings.Trim(w, ",.()[]{}'")
			if strings.HasSuffix(clean, "_ref") || strings.HasSuffix(clean, "_refs") {
				count := 1
				words := strings.Fields(lower)
				if len(words) >= 3 {
					if n, err := strconv.Atoi(words[2]); err == nil && n > 0 {
						count = n
					}
				}
				return fmt.Sprintf("at_least:%d:%s", count, clean), true
			}
		}
	}

	return "", false
}

func parseLinkBackTokens(p string) (subject, target string, ok bool) {
	delimiter := "link back to"
	if !strings.Contains(p, delimiter) {
		delimiter = "links back to"
	}
	if !strings.Contains(p, delimiter) {
		delimiter = "belongs to"
	}
	parts := strings.Split(p, delimiter)
	if len(parts) != 2 {
		return "", "", false
	}
	for _, w := range strings.Fields(parts[0]) {
		clean := strings.Trim(w, ",.()[]{}'")
		if strings.Contains(clean, "ref") {
			subject = clean
			break
		}
	}
	for _, w := range strings.Fields(parts[1]) {
		clean := strings.Trim(w, ",.()[]{}'")
		if strings.Contains(clean, "ref") {
			target = clean
			break
		}
	}
	if subject == "" || target == "" {
		return "", "", false
	}
	return subject, target, true
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
