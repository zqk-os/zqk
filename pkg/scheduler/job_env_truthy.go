package scheduler

import "strings"

// jobEnvAffirmative reports whether job env key is set to an affirmative token (1, true, yes; case-insensitive).
// Unset or empty is false — for opt-in flags that default off.
func jobEnvAffirmative(job *ScheduledJob, key string) bool {
	v := strings.TrimSpace(envLookup(job, key))
	if v == emptyValue {
		return false
	}
	return strings.EqualFold(v, "1") || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
