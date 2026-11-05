package objects

import (
	"fmt"
	"strings"
)

// GetString safely extracts a string value from a map[string]any,
// returning the empty string if the key does not exist or the value is not a string.
func GetString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// ExtractGoalRefs extracts goal references from an object map in a type-safe, deduplicated manner.
// It inspects goal_refs, goal_ref, blocking_goal_refs, linked_goal_refs, and related_object_refs.
func ExtractGoalRefs(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	var res []string
	seen := make(map[string]bool)
	add := func(val string) {
		val = strings.TrimSpace(val)
		val = strings.TrimPrefix(val, "goal:")
		if val != "" && val != "<nil>" && !seen[val] {
			seen[val] = true
			res = append(res, val)
		}
	}
	parseAny := func(v any) {
		if v == nil {
			return
		}
		switch val := v.(type) {
		case string:
			for _, part := range strings.Split(val, ",") {
				add(part)
			}
		case []string:
			for _, s := range val {
				add(s)
			}
		case []any:
			for _, item := range val {
				if item != nil {
					if s, ok := item.(string); ok {
						add(s)
					} else {
						add(fmt.Sprint(item))
					}
				}
			}
		}
	}

	if grefs, ok := obj[FieldKeyGoalRefs]; ok && grefs != nil {
		parseAny(grefs)
	}
	if len(res) == 0 {
		if grefs, ok := obj["goal_refs"]; ok && grefs != nil {
			parseAny(grefs)
		}
	}
	if len(res) == 0 {
		if gref, ok := obj["goal_ref"]; ok && gref != nil {
			parseAny(gref)
		}
	}
	if len(res) == 0 {
		if grefs, ok := obj[FieldKeyBlockingGoalRefs]; ok && grefs != nil {
			parseAny(grefs)
		}
	}
	if len(res) == 0 {
		if grefs, ok := obj[FieldKeyLinkedGoalRefs]; ok && grefs != nil {
			parseAny(grefs)
		}
	}
	if len(res) == 0 {
		if rel, ok := obj[FieldKeyRelatedObjectRefs]; ok && rel != nil {
			switch val := rel.(type) {
			case []string:
				for _, r := range val {
					if strings.HasPrefix(strings.ToUpper(r), "GOAL-") || strings.HasPrefix(strings.ToLower(r), "goal:") {
						add(r)
					}
				}
			case []any:
				for _, r := range val {
					s := fmt.Sprint(r)
					if strings.HasPrefix(strings.ToUpper(s), "GOAL-") || strings.HasPrefix(strings.ToLower(s), "goal:") {
						add(s)
					}
				}
			case string:
				for _, part := range strings.Split(val, ",") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(strings.ToUpper(part), "GOAL-") || strings.HasPrefix(strings.ToLower(part), "goal:") {
						add(part)
					}
				}
			}
		}
	}
	return res
}
