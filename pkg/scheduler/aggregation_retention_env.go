package scheduler

import (
	"strconv"
	"time"
)

// parseDurationOrHoursSuffix parses a duration with time.ParseDuration. If that fails, retries with
// the string suffixed by "h" so legacy env values like "48" are interpreted as 48 hours.
func parseDurationOrHoursSuffix(s string) (time.Duration, bool) {
	if s == emptyValue {
		return 0, false
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, true
	}
	if d, err := time.ParseDuration(s + "h"); err == nil {
		return d, true
	}
	return 0, false
}

// retentionDaysFromJobEnv parses RETENTION_DAYS (integer days) or, if that key is absent or empty,
// RETENTION_DURATION (Go duration string, or with "h" suffix). Returns defaultDays when env is nil,
// keys are missing, or values are unparseable. If RETENTION_DAYS is set non-empty but not a valid
// integer, defaultDays is returned and RETENTION_DURATION is not consulted (matches prior handler behavior).
func retentionDaysFromJobEnv(env map[string]string, defaultDays int) int {
	if env == nil {
		return defaultDays
	}
	if retStr, ok := env[EnvKeyRetentionDays]; ok && retStr != emptyValue {
		if days, err := strconv.Atoi(retStr); err == nil {
			return days
		}
		return defaultDays
	}
	if retStr, ok := env[EnvKeyRetentionDuration]; ok && retStr != emptyValue {
		if d, ok := parseDurationOrHoursSuffix(retStr); ok {
			return int(d.Hours() / 24)
		}
	}
	return defaultDays
}
