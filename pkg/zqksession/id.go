package zqksession

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// CanonicalIDPrefix is the primary zqk_session id prefix from kernel id_prefixes_config.
const CanonicalIDPrefix = objects.SessionIDPrefix

const hyphen = "-"

// LooksLikeID reports whether raw is a session object id (optional |persona suffix).
// Prefixes come from kernel id config plus live/default brand leftovers — not a product literal in callers.
func LooksLikeID(raw string) bool {
	id := idCore(raw)
	if id == "" {
		return false
	}
	for _, prefix := range IDPrefixes() {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// IDPrefixes returns recognized session id prefixes, each ending in "-".
func IDPrefixes() []string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	add := func(prefix string) {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return
		}
		if !strings.HasSuffix(prefix, hyphen) {
			prefix += hyphen
		}
		if _, ok := seen[prefix]; ok {
			return
		}
		seen[prefix] = struct{}{}
		out = append(out, prefix)
	}
	if cfg := validation.GetGlobalIDPrefixesConfig(); cfg != nil {
		for _, prefix := range cfg.GetPrefixesForKind(objects.KindZqkSession) {
			add(prefix)
		}
	}
	add(CanonicalIDPrefix)
	add(brand.EnvPrefix() + hyphen)
	add(brand.DefaultEnvPrefix + hyphen)
	return out
}

func idCore(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '|'); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}
