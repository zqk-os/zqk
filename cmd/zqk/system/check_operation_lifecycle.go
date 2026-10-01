package system

import (
	"errors"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
)

// reportSystemCheckCallback emits the high-level system_check complete or error
// callback. duration is wall-clock of the check; callers must not pass a placeholder zero.
func reportSystemCheckCallback(cb concurrency.OperationCallback, operationID string, checkErr error, duration time.Duration) {
	if cb == nil {
		return
	}
	if checkErr != nil {
		var exitCoder interface{ ExitCode() int }
		if errors.As(checkErr, &exitCoder) && exitCoder.ExitCode() == 3 {
			cb.OnComplete(operationID, map[string]any{"verdict": "warnings"}, duration)
			return
		}
		cb.OnError(operationID, checkErr)
		return
	}
	cb.OnComplete(operationID, nil, duration)
}

// emitSystemCheckOperationResult reports follower / non-blocking check finish via coordinator.
func emitSystemCheckOperationResult(
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID, profile string,
	checkErr error,
	duration time.Duration,
) {
	if projectRoot == emptyValue || storageProvider == nil {
		return
	}
	cb := coordination.NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		projectRoot,
		storageProvider,
		eventTypeSystemCheck,
		profile,
	)
	reportSystemCheckCallback(cb, operationID, checkErr, duration)
}
