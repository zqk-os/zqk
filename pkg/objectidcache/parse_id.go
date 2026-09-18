package objectidcache

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// parseReferenceID extracts kind from a cache/pending id. Mirrors cmd/zqk/system.parseReferenceID
// for drainObjectIDCachePending without importing the CLI package.
func parseReferenceID(refID string) (refKind string, actualRefID string) {
	actualRefID = refID
	accountRefPrefix := objects.KindAccount + ":"
	if strings.HasPrefix(refID, accountRefPrefix) {
		return objects.KindAccount, strings.TrimPrefix(refID, accountRefPrefix)
	}
	if strings.HasPrefix(refID, "domain:organizational:") {
		parts := strings.SplitN(refID, ":", 4)
		if len(parts) >= 4 {
			return parts[2], parts[3]
		}
	}
	if strings.Contains(refID, ":") {
		parts := strings.SplitN(refID, ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	if i := strings.Index(refID, "-"); i > 0 {
		return strings.ToLower(refID[:i]), refID
	}
	return "", refID
}
