package objects

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// ResolveAndValidateKindForProject trims rawArg, resolves synonym aliases via GetCanonicalKind,
// and when a materialized spec index exists for projectRoot, rejects kinds not present in the index
// before expensive storage work.
func ResolveAndValidateKindForProject(projectRoot, rawArg string) (string, error) {
	raw := strings.TrimSpace(rawArg)
	if raw == "" {
		return "", errfmt.Errorf("kind argument is empty")
	}
	canonical := GetCanonicalKind(raw)
	if canonical == "" {
		return "", errfmt.Errorf("invalid kind %q", rawArg)
	}
	if idx := TryLoadSpecIndexForProjectRoot(projectRoot); idx != nil {
		if _, ok := idx.GetKindSummary(canonical); !ok {
			if strings.EqualFold(raw, canonical) {
				return "", errfmt.Errorf("unknown kind %q: not in this project's object spec index (update specs or use a registered kind name)", canonical)
			}
			return "", errfmt.Errorf("unknown kind %q (from %q): not in this project's object spec index (update specs or use a registered kind name)", canonical, raw)
		}
	}
	return canonical, nil
}

// ResolveAndValidateKindsCommaSeparated splits a comma-separated kind argument, validates each
// segment with [ResolveAndValidateKindForProject], and returns canonical names in order.
func ResolveAndValidateKindsCommaSeparated(projectRoot, kindArg string) ([]string, error) {
	kindStrs := strings.Split(kindArg, ",")
	kinds := make([]string, 0, len(kindStrs))
	for _, k := range kindStrs {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		canonical, err := ResolveAndValidateKindForProject(projectRoot, k)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, canonical)
	}
	if len(kinds) == 0 {
		return nil, errfmt.Errorf("no valid kinds specified")
	}
	return kinds, nil
}
