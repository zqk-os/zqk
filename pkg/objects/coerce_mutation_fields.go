package objects

import (
	"encoding/json"
	"fmt"
	"strings"
)

// listFieldTypeNames are spec Type values that must persist as a list.
var listFieldTypeNames = map[string]struct{}{
	"list":      {},
	"array":     {},
	"string[]":  {},
	"[]string":  {},
	"[]any":     {},
	"list[str]": {},
}

// CoerceMutationFields rewrites construction footguns before validate/persist:
// a bare string on a *_refs (or spec list) field becomes a one-element list, and
// criteria type= / criteria_type= is remapped onto category (CanonicalizeCriteriaFieldKeys).
// Kind policy stays here and in kernelcas overlays / the CAS membrane — not in
// generic object create/update CLI.
//
// Hot-path safe: never calls LoadFields. Spec list types are used only when the
// field registry is already loaded.
func CoerceMutationFields(kind string, obj map[string]any) {
	if obj == nil {
		return
	}
	if kind == KindCriteria {
		CanonicalizeCriteriaFieldKeys(obj)
	}
	names := refListFieldNames(obj)
	if fr := GetGlobalFieldRegistry(); fr != nil {
		if kf, ok := fr.GetFieldsForKindIfLoaded(kind); ok && kf != nil {
			for i := range kf.AllFields {
				if isListFieldType(kf.AllFields[i].Type) {
					names[kf.AllFields[i].Name] = struct{}{}
				}
			}
		}
	}
	for name := range names {
		if v, ok := obj[name]; ok {
			obj[name] = CoerceToStringList(v)
		}
	}
}

// CoerceToStringList turns a string (or JSON array text) into []string. Already-list
// values are normalized to []string. Unknown types are left unchanged.
func CoerceToStringList(v any) any {
	if v == nil {
		return v
	}
	if sl, ok := asStringList(v); ok {
		return sl
	}
	return v
}

// FieldLooksLikeRefList reports that name is a *_refs field or current is already a list.
func FieldLooksLikeRefList(name string, current any) bool {
	if strings.HasSuffix(name, "_refs") {
		return true
	}
	switch current.(type) {
	case []string, []any:
		return true
	default:
		return false
	}
}

// AppendRefList appends added onto current as a string list (CLI field+= for refs).
func AppendRefList(current, added any) []string {
	out := stringListFromAny(current)
	for _, s := range stringListFromAny(added) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// RequireCriteriaCategory fails closed when kind is criteria and category is empty
// after CoerceMutationFields. Call from the CAS membrane (hash persist / promote),
// not from create-validate — incomplete creates park on the draft plane.
func RequireCriteriaCategory(kind string, obj map[string]any) error {
	if kind != KindCriteria {
		return nil
	}
	if strings.TrimSpace(GetString(obj, FieldKeyCategory)) != "" {
		return nil
	}
	return fmt.Errorf("criteria requires category (functional|non-functional|acceptance|test|performance|security|compliance)")
}

func refListFieldNames(obj map[string]any) map[string]struct{} {
	names := make(map[string]struct{})
	for k := range obj {
		if strings.HasSuffix(k, "_refs") {
			names[k] = struct{}{}
		}
	}
	return names
}

func isListFieldType(t string) bool {
	_, ok := listFieldTypeNames[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

func asStringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		if anyListHasStructuredItems(t) {
			return nil, false
		}
		return stringifyAnyList(t), true
	case string:
		return splitRefString(t), true
	default:
		return nil, false
	}
}

func stringListFromAny(v any) []string {
	if sl, ok := asStringList(v); ok {
		return sl
	}
	return nil
}

func anyListHasStructuredItems(items []any) bool {
	for _, item := range items {
		switch item.(type) {
		case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			continue
		default:
			return true
		}
	}
	return false
}

func stringifyAnyList(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch x := item.(type) {
		case string:
			if s := strings.TrimSpace(x); s != "" {
				out = append(out, s)
			}
		case nil:
			continue
		default:
			if s := strings.TrimSpace(toRefString(x)); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func splitRefString(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	if strings.HasPrefix(s, "[") {
		var arr []any
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return stringifyAnyList(arr)
		}
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func toRefString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
