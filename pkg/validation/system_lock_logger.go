package validation

import (
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// lockLoggerSystem returns the lock-aware logger for the system profile.
// Centralizes profile string per CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
func lockLoggerSystem() concurrency.LockLogger {
	return logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
}
