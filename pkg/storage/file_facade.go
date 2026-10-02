package storage

// Facade boundary for upcoming decomposition.

import (
	"github.com/zqk-os/zqk/pkg/storage/file"
)

type FileLockConfig = file.FileLockConfig
type FileLock = file.FileLock
type FileLockMetrics = file.FileLockMetrics
type FileLockMetricsSnapshot = file.FileLockMetricsSnapshot
type FileLockDerivedMetrics = file.FileLockDerivedMetrics
type FileLockStrategy = file.FileLockStrategy
type FileLockStrategyMetrics = file.FileLockStrategyMetrics
type LockHandle = file.LockHandle
type AutoCleanupStrategy = file.AutoCleanupStrategy

func NewFileLock(filePath string) (*FileLock, error) {
	return file.NewFileLock(filePath)
}

func NewFileLockWithConfig(filePath string, config FileLockConfig) (*FileLock, error) {
	return file.NewFileLockWithConfig(filePath, config)
}

func NewAutoCleanupStrategy() *AutoCleanupStrategy {
	return file.NewAutoCleanupStrategy()
}

func GetFileLockStrategyMetrics() *FileLockStrategyMetrics {
	return file.GetFileLockStrategyMetrics()
}

func GetFileLockMetrics() *FileLockMetrics {
	return file.GetFileLockMetrics()
}

func GetFileLockSnapshotAndDerived() (file.FileLockMetricsSnapshot, file.FileLockDerivedMetrics) {
	return file.GetFileLockMetrics().SnapshotAndDerived()
}

func ResetFileLockMetrics() {
	file.ResetFileLockMetrics()
}
