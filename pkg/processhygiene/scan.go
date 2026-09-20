package processhygiene

import "fmt"

const (
	emptyValue      = ""
	fieldID         = "id"
	fieldKind       = "kind"
	defaultMaxValue = 500
	valueEllipsis   = "..."
)

// Finding is one object/rule match (same object may appear multiple times for different rules).
type Finding struct {
	RuleID          string `json:"rule_id"`
	RuleDescription string `json:"rule_description,omitempty"`
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Field           string `json:"field"`
	Value           string `json:"value"`
	Detail          string `json:"detail,omitempty"`
}

// ScanObjects applies all rules to each object and returns findings (order: rules file order, then object list order).
func ScanObjects(objects []map[string]any, rules []Rule) []Finding {
	var out []Finding
	for _, obj := range objects {
		for _, rule := range rules {
			if detail, ok := rule.Evaluate(obj); ok {
				id, _ := obj[fieldID].(string)
				kind, _ := obj[fieldKind].(string)
				field := rule.MatchedField()
				val := emptyValue
				if field != emptyValue {
					if raw, has := obj[field]; has && raw != nil {
						val = trimPreview(fmt.Sprint(raw), defaultMaxValue)
					}
				}
				out = append(out, Finding{
					RuleID:          rule.ID(),
					RuleDescription: rule.Description(),
					ID:              id,
					Kind:            kind,
					Field:           field,
					Value:           val,
					Detail:          detail,
				})
			}
		}
	}
	return out
}

func trimPreview(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + valueEllipsis
}

// CountByRule returns finding counts per rule_id.
func CountByRule(findings []Finding) map[string]int {
	m := make(map[string]int)
	for _, f := range findings {
		m[f.RuleID]++
	}
	return m
}
