package quality

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ParseColumnValuePairs parses repeatable "column=value" or "column:value" flags (e.g. --filter, --set).
// When requireAtLeastOne is true, returns an error if no non-empty pairs were parsed.
func ParseColumnValuePairs(pairs []string, requireAtLeastOne bool, flagName string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sep := strings.IndexAny(p, "=:")
		if sep < 0 {
			return nil, errfmt.Errorf("invalid %s %q (use column=value)", flagName, p)
		}
		col := strings.TrimSpace(p[:sep])
		val := strings.TrimSpace(p[sep+1:])
		if col == "" {
			return nil, errfmt.Errorf("invalid %s %q (empty column name)", flagName, p)
		}
		out[col] = val
	}
	if requireAtLeastOne && len(out) == 0 {
		return nil, errfmt.Errorf("at least one %s column=value is required", flagName)
	}
	return out, nil
}
