package processhygiene

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/nildecode"
)

// Rule evaluates one configured check against a whole object map.
type Rule interface {
	ID() string
	Description() string
	MatchedField() string
	Evaluate(obj map[string]any) (detail string, matched bool)
}

type declarativeRule struct {
	id          string
	description string
	field       string
	prefix      string
	suffix      string
	equals      string
	re          *regexp.Regexp
}

func (r *declarativeRule) ID() string           { return r.id }
func (r *declarativeRule) Description() string  { return r.description }
func (r *declarativeRule) MatchedField() string { return r.field }
func (r *declarativeRule) Evaluate(obj map[string]any) (string, bool) {
	if obj == nil {
		return emptyValue, false
	}
	raw, ok := obj[r.field]
	if !ok {
		return emptyValue, false
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return emptyValue, false
	}
	s := strings.TrimSpace(fmt.Sprint(raw))
	if s == emptyValue {
		return emptyValue, false
	}

	if r.equals != emptyValue {
		if s == r.equals {
			return fmt.Sprintf("%s equals configured literal", r.field), true
		}
		return emptyValue, false
	}
	if r.prefix != emptyValue {
		if strings.HasPrefix(s, r.prefix) {
			return fmt.Sprintf("%s has prefix %q", r.field, r.prefix), true
		}
		return emptyValue, false
	}
	if r.suffix != emptyValue {
		if strings.HasSuffix(s, r.suffix) {
			return fmt.Sprintf("%s has suffix %q", r.field, r.suffix), true
		}
		return emptyValue, false
	}
	if r.re != nil {
		if loc := r.re.FindStringSubmatchIndex(s); loc != nil {
			return fmt.Sprintf("%s matches pattern", r.field), true
		}
		return emptyValue, false
	}
	return emptyValue, false
}
