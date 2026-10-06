package audit

import (
	"fmt"
	"strings"
	"time"
)

// CASIDIndex lists IDs from a content-addressable index. Storage wires the
// FileObjectStorage CAS handle; this package does not import storage.
type CASIDIndex interface {
	ListIDs() ([]string, error)
}

// NumericPrefixedIDs keeps IDs matching prefix + digits (e.g. AUD + "AUD-001").
// prefix may be "AUD" or "AUD-"; a trailing dash is added when missing.
func NumericPrefixedIDs(prefix string, ids []string) []string {
	p := prefix
	if p == "" || p[len(p)-1] != '-' {
		p += "-"
	}
	out := make([]string, 0)
	for _, id := range ids {
		if !strings.HasPrefix(id, p) {
			continue
		}
		if !allDigits(id[len(p):]) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ListNumericPrefixedIDs reads idx and filters. List errors are best-effort
// (empty, nil) so ID generation can fall back to a file scan.
func ListNumericPrefixedIDs(idx CASIDIndex, prefix string) ([]string, error) {
	if idx == nil {
		return nil, nil
	}
	all, err := idx.ListIDs()
	if err != nil {
		return nil, nil
	}
	return NumericPrefixedIDs(prefix, all), nil
}

// TimestampID is prefix + UnixNano. Used when CAS or ID generation cannot sequence.
func TimestampID(prefix string, now time.Time) string {
	return fmt.Sprintf("%s%d", prefix, now.UnixNano())
}

// ChooseMetricID returns generated when set, otherwise TimestampID.
func ChooseMetricID(generated, prefix string, now time.Time) string {
	if generated != "" {
		return generated
	}
	return TimestampID(prefix, now)
}

// IndexEmpty reports whether a CAS listing has no IDs (scan needed).
func IndexEmpty(ids []string) bool {
	return len(ids) == 0
}

// ExpandIDRange expands an ID range like "AUD-1..AUD-165" into individual IDs.
// prefix and separator must match the storage constants (PrefixAudit / SeparatorRange).
func ExpandIDRange(prefix, separator, rangeStr string) []string {
	if !strings.Contains(rangeStr, separator) {
		return []string{rangeStr}
	}

	parts := strings.Split(rangeStr, separator)
	if len(parts) != 2 {
		return []string{rangeStr}
	}

	startStr := strings.TrimPrefix(parts[0], prefix)
	endStr := strings.TrimPrefix(parts[1], prefix)

	var start, end int
	if _, err := fmt.Sscanf(startStr, "%d", &start); err != nil {
		return []string{rangeStr}
	}
	if _, err := fmt.Sscanf(endStr, "%d", &end); err != nil {
		return []string{rangeStr}
	}

	if start < 0 || start > end || (end-start) >= 50000 {
		return []string{rangeStr}
	}

	ids := make([]string, 0)
	for i := start; i <= end; i++ {
		ids = append(ids, fmt.Sprintf("%s%d", prefix, i))
	}
	return ids
}

// ExpandIDRanges expands a list of ID ranges and individual IDs into a flat list.
func ExpandIDRanges(prefix, separator string, idRanges []string) []string {
	var allIDs []string
	for _, idRange := range idRanges {
		allIDs = append(allIDs, ExpandIDRange(prefix, separator, idRange)...)
	}
	return allIDs
}
