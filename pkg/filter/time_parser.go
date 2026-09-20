package filter

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	lastNRegex = regexp.MustCompile(`(?i)^last\s+(\d+)\s+(second|minute|hour|day|month|year)s?$`)
)

// ResolveTimeTokens converts relative time strings (like "yesterday", "last 2 hours") into ISO 8601 UTC strings.
// If the input is not a recognized time token, it returns the input unchanged.
func ResolveTimeTokens(val any) any {
	switch v := val.(type) {
	case string:
		return resolveTimeString(v)
	case map[string]any:
		for k, mapVal := range v {
			v[k] = ResolveTimeTokens(mapVal)
		}
		return v
	case []any:
		for i, arrVal := range v {
			v[i] = ResolveTimeTokens(arrVal)
		}
		return v
	default:
		return val
	}
}

func resolveTimeString(s string) string {
	lower := strings.TrimSpace(strings.ToLower(s))
	now := time.Now().UTC()

	switch lower {
	case "now":
		return now.Format(time.RFC3339)
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	case "yesterday":
		return time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	case "last_week", "last week":
		return now.AddDate(0, 0, -7).Format(time.RFC3339)
	case "last_month", "last month":
		return now.AddDate(0, -1, 0).Format(time.RFC3339)
	case "last_year", "last year":
		return now.AddDate(-1, 0, 0).Format(time.RFC3339)
	}

	matches := lastNRegex.FindStringSubmatch(lower)
	if len(matches) == 3 {
		n, err := strconv.Atoi(matches[1])
		if err != nil {
			return s
		}
		unit := matches[2]

		switch unit {
		case "second":
			return now.Add(-time.Duration(n) * time.Second).Format(time.RFC3339)
		case "minute":
			return now.Add(-time.Duration(n) * time.Minute).Format(time.RFC3339)
		case "hour":
			return now.Add(-time.Duration(n) * time.Hour).Format(time.RFC3339)
		case "day":
			return now.AddDate(0, 0, -n).Format(time.RFC3339)
		case "month":
			return now.AddDate(0, -n, 0).Format(time.RFC3339)
		case "year":
			return now.AddDate(-n, 0, 0).Format(time.RFC3339)
		}
	}

	return s
}
