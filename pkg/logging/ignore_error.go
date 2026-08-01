package logging

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// LogSwallowedError logs an error that is intentionally swallowed to the system logger.
// This provides a consistent abstraction to satisfy the "error_handling_debt" QA rule
// without violating the "POLICY-CODE-007" logging rule (which forbids direct fmt.Printf).
func LogSwallowedError(err error) {
	if err != nil {
		logger := GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if logger != nil {
			Fluent(logger).Error("swallowed_error", err).Log()
		}
	}
}
