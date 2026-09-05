package validation

import (
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// lockLoggerSystem returns the lock-aware logger for the system profile.
// Centralizes profile string per CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
func lockLoggerSystem() concurrency.LockLogger {
	return logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
}
