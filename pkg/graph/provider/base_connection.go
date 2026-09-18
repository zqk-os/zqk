package provider

import (
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
)

// BaseConnection provides shared connection state management that can be embedded
// by provider implementations. It handles transaction state tracking and reset logic.
type BaseConnection struct {
	mu     sync.RWMutex
	openTx GraphTransaction
}

// HasOpenTransaction checks if there's an open transaction
func (bc *BaseConnection) HasOpenTransaction() bool {
	var hasTx bool
	_ = concurrency.RunInRLockWithLogger(
		&bc.mu, LockNameBaseConnectionHasTx, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			hasTx = bc.openTx != nil
			return nil
		},
	)
	return hasTx
}

// GetOpenTransaction returns the open transaction if any
func (bc *BaseConnection) GetOpenTransaction() GraphTransaction {
	var tx GraphTransaction
	_ = concurrency.RunInRLockWithLogger(
		&bc.mu, LockNameBaseConnectionGetTx, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tx = bc.openTx
			return nil
		},
	)
	return tx
}

// SetOpenTransaction sets the open transaction
func (bc *BaseConnection) SetOpenTransaction(tx GraphTransaction) {
	_ = concurrency.RunInLockWithLogger(
		&bc.mu, LockNameBaseConnectionSetTx, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			bc.openTx = tx
			return nil
		},
	)
}

// Reset resets the connection state (called when returning to pool)
func (bc *BaseConnection) Reset() {
	_ = concurrency.RunInLockWithLogger(
		&bc.mu, LockNameBaseConnectionReset, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			bc.openTx = nil
			return nil
		},
	)
}

// ClearTransaction clears the transaction reference (called after commit/rollback)
func (bc *BaseConnection) ClearTransaction() {
	_ = concurrency.RunInLockWithLogger(
		&bc.mu, LockNameBaseConnectionClearTx, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			bc.openTx = nil
			return nil
		},
	)
}
