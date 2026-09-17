package object

import (
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// slowCLIObjectMutationFlushThreshold is when we log that post-mutation durability work took
// long enough to notice (bounded by DurabilityFlushContext, not an indefinite hang).
const slowCLIObjectMutationFlushThreshold = 2 * time.Second

// logSlowCLIObjectMutationFlush logs at Info when ensureVisible+listingIndexFlush meets or exceeds
// slowCLIObjectMutationFlushThreshold. listingIndexFlush is zero when the path only runs
// EnsureCLIObjectMutationVisibleForProvider (e.g. create/update).
func logSlowCLIObjectMutationFlush(
	el *logging.EventLogger,
	operation, objectID string,
	kinds []string,
	ensureVisible, listingIndexFlush time.Duration,
) {
	if el == nil {
		return
	}
	total := ensureVisible + listingIndexFlush
	if total < slowCLIObjectMutationFlushThreshold {
		return
	}
	kindStr := strings.Join(kinds, ",")
	if kindStr == "" {
		kindStr = "(none)"
	}
	id := objectID
	if id == "" {
		id = "(n/a)"
	}
	listing := listingIndexFlush.String()
	if listingIndexFlush == 0 {
		listing = "0s"
	}
	logging.FluentEvent(el).Info("object CLI: durability flush slower than threshold (disk busy or WAL backlog; bounded by context timeout)").
		String("operation", operation).
		String("id", id).
		String("kinds", kindStr).
		String("ensure_visible", ensureVisible.String()).
		String("listing_index_flush", listing).
		String("total", total.String()).
		Log()
}
