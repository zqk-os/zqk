// Package zqktime provides UTC timestamp formatting for persisted and operator-visible data.
//
// Policy: use these helpers (or time.ParseInLocation with explicit UTC) instead of ad hoc
// time.Now().Format(time.RFC3339), which uses local zone. See docs/enforcement/AGENT_GUIDELINES.md
// and ITEM-EXAMPLE.
//
// This package must not import other zqk packages to avoid cycles.
package zqktime

import "time"

// Common layouts for UTC formatting (pair with FormatLayoutUTC / NowLayoutUTC).
const (
	// LayoutDateTimeSpace is a human-readable local-neutral stamp for titles and summaries (UTC wall clock).
	LayoutDateTimeSpace = "2006-01-02 15:04:05"
	// LayoutDateTimeMillis includes fractional seconds (UTC wall clock); used for trace-style lines.
	LayoutDateTimeMillis = "2006-01-02 15:04:05.000"
	// LayoutObjectDateTimeZ is a fixed-offset Z stamp used in many object YAML fields.
	LayoutObjectDateTimeZ = "2006-01-02T15:04:05Z"
	// LayoutDate is the calendar date in UTC (directory bucketing, day keys).
	LayoutDate = "2006-01-02"
	// LayoutLogRotateStamp is a compact UTC stamp for rotated log filenames.
	LayoutLogRotateStamp = "20060102-150405"
)

// FormatRFC3339UTC renders t as RFC3339 in UTC (Z or offset +00:00).
func FormatRFC3339UTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// NowRFC3339UTC is shorthand for FormatRFC3339UTC(time.Now()).
func NowRFC3339UTC() string {
	return FormatRFC3339UTC(time.Now())
}

// FormatRFC3339UTCPtr formats *t in UTC; returns empty string if t is nil.
func FormatRFC3339UTCPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return FormatRFC3339UTC(*t)
}

// FormatRFC3339NanoUTC renders t as RFC3339Nano in UTC.
func FormatRFC3339NanoUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// NowRFC3339NanoUTC is shorthand for FormatRFC3339NanoUTC(time.Now()).
func NowRFC3339NanoUTC() string {
	return FormatRFC3339NanoUTC(time.Now())
}

// FormatLayoutUTC renders t using layout with the instant interpreted in UTC wall clock.
func FormatLayoutUTC(t time.Time, layout string) string {
	return t.UTC().Format(layout)
}

// NowLayoutUTC is shorthand for FormatLayoutUTC(time.Now(), layout).
func NowLayoutUTC(layout string) string {
	return FormatLayoutUTC(time.Now(), layout)
}
