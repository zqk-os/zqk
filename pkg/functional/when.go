package functional

import "github.com/zqk-os/zqk/pkg/when"

// When starts a when.Chain: when cond() is true, the next Then(fn) will run that fn.
// Re-exported from pkg/when so callers can use functional.When without importing when.
// pkg/when has no dependencies and can be used from logging; functional adds Result/Apply which use coordination.
func When(cond func() bool) *when.Builder {
	return when.When(cond)
}
