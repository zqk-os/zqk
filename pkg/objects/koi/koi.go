package koi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Status returns the status string from an object map, or empty string if absent or nil.
func Status(m map[string]any) string {
	return GetString(m, objects.FieldKeyStatus)
}

// IsStatus reports whether m has the specified status (case-insensitive comparison).
func IsStatus(m map[string]any, expected string) bool {
	st := Status(m)
	return strings.EqualFold(st, expected)
}

// InProgress reports whether the object status is "in_progress".
func InProgress(m map[string]any) bool {
	return IsStatus(m, objects.ObjectStatusInProgress)
}

// NotInProgress reports whether the object status is NOT "in_progress".
func NotInProgress(m map[string]any) bool {
	return !InProgress(m)
}

// IsTerminal reports whether the object has reached a terminal lifecycle state,
// using the standard project-provided StatusChecker.
func IsTerminal(m map[string]any) bool {
	kind := Kind(m)
	st := Status(m)
	if kind == "" || st == "" {
		return false
	}
	return objects.GetGlobalStatusChecker().IsTerminal(kind, st)
}

// IsWorkDone reports whether the status is a work-interval finish (not archive),
// using the standard project-provided StatusChecker.
func IsWorkDone(m map[string]any) bool {
	kind := Kind(m)
	st := Status(m)
	if kind == "" || st == "" {
		return false
	}
	return objects.GetGlobalStatusChecker().IsWorkDone(kind, st)
}

// IsPreliminary reports whether the status is preliminary/draft/origin,
// using the standard project-provided StatusChecker.
func IsPreliminary(m map[string]any) bool {
	kind := Kind(m)
	st := Status(m)
	if kind == "" || st == "" {
		return false
	}
	return objects.GetGlobalStatusChecker().IsPreliminary(kind, st)
}

// IsArchive reports whether the status represents an archived/inactive state,
// using the standard project-provided StatusChecker.
func IsArchive(m map[string]any) bool {
	kind := Kind(m)
	st := Status(m)
	if kind == "" || st == "" {
		return false
	}
	return objects.GetGlobalStatusChecker().IsArchive(kind, st)
}

// IsSatisfied reports whether a satisfiable kind's predicate holds at this status,
// using the standard project-provided StatusChecker.
func IsSatisfied(m map[string]any) bool {
	kind := Kind(m)
	st := Status(m)
	if kind == "" || st == "" {
		return false
	}
	return objects.GetGlobalStatusChecker().IsSatisfied(kind, st)
}

// ID returns the object id from m, or empty string if absent or nil.
func ID(m map[string]any) string {
	return GetString(m, objects.FieldKeyID)
}

// Kind returns the object kind from m, or empty string if absent or nil.
func Kind(m map[string]any) string {
	return GetString(m, objects.FieldKeyKind)
}

// Title returns the object title from m, or empty string if absent or nil.
func Title(m map[string]any) string {
	return GetString(m, objects.FieldKeyTitle)
}

// Namespace returns the namespace_id from m, or empty string if absent or nil.
func Namespace(m map[string]any) string {
	return GetString(m, objects.FieldKeyNamespaceID)
}

// GetString returns the string value for key, or empty string if missing or non-string.
func GetString(m map[string]any, key string) string {
	return GetStringOr(m, key, "")
}

// GetStringOr returns the string value for key, or fallback if missing or non-string.
func GetStringOr(m map[string]any, key string, fallback string) string {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	if s, ok := v.(string); ok {
		return s
	}
	if fmtStr, ok := v.(fmt.Stringer); ok {
		return fmtStr.String()
	}
	return fallback
}

// GetStringSlice extracts a slice of strings from key, converting []any or []string elements.
func GetStringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		out := make([]string, len(s))
		copy(out, s)
		return out
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if item == nil {
				continue
			}
			if str, ok := item.(string); ok {
				out = append(out, str)
			} else {
				out = append(out, fmt.Sprint(item))
			}
		}
		return out
	case string:
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}

// GetMap returns a map[string]any for key, or nil if missing or non-map.
func GetMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	if subMap, ok := v.(map[string]any); ok {
		return subMap
	}
	return nil
}

// GetIntOr returns an int value for key, coercing numeric types and strings, or fallback on failure.
func GetIntOr(m map[string]any, key string, fallback int) int {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case int32:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return i
		}
	}
	return fallback
}

// GetInt64Or returns an int64 value for key, coercing numeric types and strings, or fallback on failure.
func GetInt64Or(m map[string]any, key string, fallback int64) int64 {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

// GetBoolOr returns a bool value for key, coercing strings ("true", "1") or fallback on failure.
func GetBoolOr(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(val)); err == nil {
			return b
		}
	case int:
		return val != 0
	case int64:
		return val != 0
	}
	return fallback
}

// GetTime parses a time.Time value from key (supporting time.Time or RFC3339 strings).
func GetTime(m map[string]any, key string) (time.Time, bool) {
	if m == nil {
		return time.Time{}, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return time.Time{}, false
	}
	switch val := v.(type) {
	case time.Time:
		return val, true
	case string:
		trimmed := strings.TrimSpace(val)
		if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return t, true
		}
		if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// SetStatus sets the status field on m. Safe on nil map (no-op).
func SetStatus(m map[string]any, status string) {
	if m == nil {
		return
	}
	m[objects.FieldKeyStatus] = status
}

// Set sets a key-value pair on m. Safe on nil map (no-op).
func Set(m map[string]any, key string, val any) {
	if m == nil {
		return
	}
	m[key] = val
}

// Inspector wraps a map[string]any with nil-tolerant fluent inspector methods.
type Inspector struct {
	raw map[string]any
}

// Wrap wraps an object map with fluent inspection and assertion methods.
func Wrap(m map[string]any) Inspector {
	return Inspector{raw: m}
}

// Raw returns the underlying raw map.
func (i Inspector) Raw() map[string]any {
	return i.raw
}

// IsNil reports whether the underlying map is nil.
func (i Inspector) IsNil() bool {
	return i.raw == nil
}

// ID returns the object id.
func (i Inspector) ID() string {
	return ID(i.raw)
}

// Kind returns the object kind.
func (i Inspector) Kind() string {
	return Kind(i.raw)
}

// Title returns the object title.
func (i Inspector) Title() string {
	return Title(i.raw)
}

// Status returns the object status.
func (i Inspector) Status() string {
	return Status(i.raw)
}

// Namespace returns the namespace_id.
func (i Inspector) Namespace() string {
	return Namespace(i.raw)
}

// IsStatus reports whether status matches expected (case-insensitive).
func (i Inspector) IsStatus(expected string) bool {
	return IsStatus(i.raw, expected)
}

// InProgress reports whether status is "in_progress".
func (i Inspector) InProgress() bool {
	return InProgress(i.raw)
}

// NotInProgress reports whether status is not "in_progress".
func (i Inspector) NotInProgress() bool {
	return NotInProgress(i.raw)
}

// IsTerminal reports whether status is in a terminal state.
func (i Inspector) IsTerminal() bool {
	return IsTerminal(i.raw)
}

// IsWorkDone reports whether status is a work-interval finish.
func (i Inspector) IsWorkDone() bool {
	return IsWorkDone(i.raw)
}

// IsPreliminary reports whether status is preliminary/draft/origin.
func (i Inspector) IsPreliminary() bool {
	return IsPreliminary(i.raw)
}

// IsArchive reports whether status is archived/inactive.
func (i Inspector) IsArchive() bool {
	return IsArchive(i.raw)
}

// IsSatisfied reports whether status is satisfied.
func (i Inspector) IsSatisfied() bool {
	return IsSatisfied(i.raw)
}

// GetString returns string value or empty string.
func (i Inspector) GetString(key string) string {
	return GetString(i.raw, key)
}

// GetStringOr returns string value or fallback.
func (i Inspector) GetStringOr(key string, fallback string) string {
	return GetStringOr(i.raw, key, fallback)
}

// GetStringSlice extracts string slice.
func (i Inspector) GetStringSlice(key string) []string {
	return GetStringSlice(i.raw, key)
}

// GetMap extracts nested map.
func (i Inspector) GetMap(key string) map[string]any {
	return GetMap(i.raw, key)
}

// GetIntOr extracts int with fallback.
func (i Inspector) GetIntOr(key string, fallback int) int {
	return GetIntOr(i.raw, key, fallback)
}

// GetInt64Or extracts int64 with fallback.
func (i Inspector) GetInt64Or(key string, fallback int64) int64 {
	return GetInt64Or(i.raw, key, fallback)
}

// GetBoolOr extracts bool with fallback.
func (i Inspector) GetBoolOr(key string, fallback bool) bool {
	return GetBoolOr(i.raw, key, fallback)
}

// GetTime extracts parsed time.Time.
func (i Inspector) GetTime(key string) (time.Time, bool) {
	return GetTime(i.raw, key)
}

// SetStatus sets status and returns the inspector for method chaining.
func (i Inspector) SetStatus(status string) Inspector {
	SetStatus(i.raw, status)
	return i
}

// Set sets key-value pair and returns the inspector for method chaining.
func (i Inspector) Set(key string, val any) Inspector {
	Set(i.raw, key, val)
	return i
}
