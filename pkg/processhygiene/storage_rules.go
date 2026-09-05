package processhygiene

import (
	"context"
	"fmt"
	"sort"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const kindProcessHygieneRule = "process_hygiene_rule"

type ruleConfigOrdered struct {
	cfg   RuleConfig
	order int
}

// loadRulesFromStorage lists enabled process_hygiene_rule objects and compiles them to [Rule].
func loadRulesFromStorage(
	ctx context.Context,
	provider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
) ([]Rule, error) {
	storageCtx := pkgctx.NewStorageContext()
	qr, err := provider.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:  kindProcessHygieneRule,
		Limit: 0,
	})
	if err != nil {
		return nil, errfmt.Newf("list process_hygiene_rule").Wrap(err)
	}
	var rows []ruleConfigOrdered
	for _, obj := range qr.Objects {
		if !asBoolField(obj, objects.FieldKeyEnabled, true) {
			continue
		}
		rc, err := ruleConfigFromObject(obj)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ruleConfigOrdered{
			cfg:   rc,
			order: asIntField(obj, objects.FieldKeySortOrder, 0),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rows[i].cfg.ID < rows[j].cfg.ID
	})
	out := make([]Rule, 0, len(rows))
	for i, row := range rows {
		rule, err := compileRule(row.cfg)
		if err != nil {
			return nil, errfmt.Errorf("process_hygiene_rule[%d] (%s): %w", i, row.cfg.ID, err)
		}
		out = append(out, rule)
	}
	return out, nil
}

func ruleConfigFromObject(obj map[string]any) (RuleConfig, error) {
	id := asStringField(obj, objects.FieldKeyRuleID)
	if id == emptyValue {
		return RuleConfig{}, errfmt.Errorf("process_hygiene_rule missing rule_id")
	}
	field := asStringField(obj, objects.FieldKeyMatchField)
	if field == emptyValue {
		return RuleConfig{}, errfmt.Errorf("process_hygiene_rule %s: missing match_field", id)
	}
	prefix := asStringField(obj, objects.FieldKeyMatchPrefix)
	suffix := asStringField(obj, objects.FieldKeyMatchSuffix)
	equals := asStringField(obj, objects.FieldKeyMatchEquals)
	regex := asStringField(obj, objects.FieldKeyMatchRegex)
	n := 0
	if prefix != emptyValue {
		n++
	}
	if suffix != emptyValue {
		n++
	}
	if equals != emptyValue {
		n++
	}
	if regex != emptyValue {
		n++
	}
	if n != 1 {
		return RuleConfig{}, errfmt.Errorf("process_hygiene_rule %s: exactly one of match_prefix, match_suffix, match_equals, match_regex must be set", id)
	}
	return RuleConfig{
		ID:          id,
		Description: asStringField(obj, objects.FieldKeyDescription),
		Match: Match{
			Field:  field,
			Prefix: prefix,
			Suffix: suffix,
			Equals: equals,
			Regex:  regex,
		},
	}, nil
}

func asStringField(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok {
		return emptyValue
	}
	v, ok = nildecode.DecodeNonNilPayload[any](v)
	if !ok {
		return emptyValue
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func asIntField(obj map[string]any, key string, def int) int {
	v, ok := obj[key]
	if !ok {
		return def
	}
	v, ok = nildecode.DecodeNonNilPayload[any](v)
	if !ok {
		return def
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return def
	}
}

func asBoolField(obj map[string]any, key string, def bool) bool {
	v, ok := obj[key]
	if !ok {
		return def
	}
	v, ok = nildecode.DecodeNonNilPayload[any](v)
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}
