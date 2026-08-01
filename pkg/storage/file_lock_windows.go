//go:build windows

package storage

import (
	"context"
	"os"
	"time"
)

type FileLock struct{}

func NewFileLock(lockPath string) (*FileLock, error) {
	return &FileLock{}, nil
}

func AcquireFileLock(ctx context.Context, lockPath string) (*FileLock, error) {
	return &FileLock{}, nil
}

func (fl *FileLock) Acquire(ctx context.Context) error {
	return nil
}

func (fl *FileLock) LockWithTimeout(timeout time.Duration) error {
	return nil
}

func (fl *FileLock) Release() error {
	return nil
}

func (fl *FileLock) Close() error {
	return nil
}

func (fl *FileLock) IsLocked() bool {
	return false
}

func TryAcquireFileLock(lockPath string) (*FileLock, error) {
	return &FileLock{}, nil
}

func BreakStaleLock(lockPath string) error {
	return nil
}

func RemoveLockFile(lockPath string) error {
	return os.Remove(lockPath)
}

func (fl *FileLock) WithLockTimeout(timeout time.Duration, fn func() error) error {
	return fn()
}

func (fl *FileLock) TryLock() (bool, error) {
	return true, nil
}

func (fl *FileLock) Unlock() error {
	return nil
}

func (fl *FileLock) WithLock(fn func() error) error {
	return fn()
}
