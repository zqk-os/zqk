package storage

import (
	"fmt"
	"time"
)

// getGranularityFormat returns the Go time format string for a predefined granularity
//
//nolint:unused,deadcode // Helper function - reserved for future use
func getGranularityFormat(granularity string) string {
	switch granularity {
	case "monthly":
		return "2006-01" // YYYY-MM
	case "weekly":
		return "2006-W01" // ISO week format (YYYY-Www) - requires special handling
	case "daily":
		return "2006-01-02" // YYYY-MM-DD
	case "hourly":
		return "2006-01-02T15" // YYYY-MM-DDTHH
	case "half_hourly":
		return ConstMisc20060102t1504 // YYYY-MM-DDTHH:MM (rounded to 30 min)
	case "qtr_hourly":
		return ConstMisc20060102t150405 // YYYY-MM-DDTHH:MM:SS (rounded to 15 min)
	case "tenths":
		return ConstMisc20060102t1504059 // YYYY-MM-DDTHH:MM:SS.M (6 minute windows)
	default:
		return "2006-01" // Default to monthly
	}
}

// formatTimeByGranularity formats a time according to the specified granularity
// Handles special cases like weekly (ISO week) and rounding for sub-hourly granularities
func formatTimeByGranularity(t time.Time, granularity string) string {
	switch granularity {
	case "monthly":
		return t.Format("2006-01") // YYYY-MM

	case "weekly":
		// ISO week format: YYYY-Www
		year, week := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)

	case "daily":
		return t.Format("2006-01-02") // YYYY-MM-DD

	case "hourly":
		return t.Format("2006-01-02T15") // YYYY-MM-DDTHH

	case "half_hourly":
		// Round to nearest 30 minutes
		rounded := t.Truncate(30 * time.Minute)
		return rounded.Format(ConstMisc20060102t1504) // YYYY-MM-DDTHH:MM

	case "qtr_hourly":
		// Round to nearest 15 minutes
		rounded := t.Truncate(15 * time.Minute)
		return rounded.Format(ConstMisc20060102t150405) // YYYY-MM-DDTHH:MM:SS

	case "tenths":
		// Round to nearest 6 minutes (1/10 of an hour)
		rounded := t.Truncate(6 * time.Minute)
		return rounded.Format(ConstMisc20060102t1504059) // YYYY-MM-DDTHH:MM:SS.M

	default:
		return t.Format("2006-01") // Default to monthly
	}
}
