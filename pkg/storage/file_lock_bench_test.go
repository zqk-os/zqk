package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkFileLock_Acquisition_NoContention(b *testing.B) {
	tmpDir := b.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fileLock, err := NewFileLock(lockFile)
		if err != nil {
			b.Fatalf("Failed to create file lock: %v", err)
		}

		if err := fileLock.Lock(); err != nil {
			b.Fatalf("Failed to acquire lock: %v", err)
		}

		_ = fileLock.Unlock() //nolint:errcheck // Test cleanup - errors are acceptable
		fileLock.Close()
	}
}

func BenchmarkFileLock_TryLock_NoContention(b *testing.B) {
	tmpDir := b.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fileLock, err := NewFileLock(lockFile)
		if err != nil {
			b.Fatalf("Failed to create file lock: %v", err)
		}

		acquired, err := fileLock.TryLock()
		if err != nil {
			b.Fatalf("TryLock returned error: %v", err)
		}
		if !acquired {
			b.Fatal("TryLock should have acquired the lock")
		}

		_ = fileLock.Unlock() //nolint:errcheck // Test cleanup - errors are acceptable
		fileLock.Close()
	}
}

func BenchmarkFileLock_WithLock(b *testing.B) {
	tmpDir := b.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		b.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := fileLock.WithLock(func() error {
			// Simulate work
			time.Sleep(1 * time.Microsecond)
			return nil
		})
		if err != nil {
			b.Fatalf("WithLock returned error: %v", err)
		}
	}
}

func BenchmarkFileLock_Metrics_Overhead(b *testing.B) {
	tmpDir := b.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// Test with metrics enabled
	b.Run("WithMetrics", func(b *testing.B) {
		ResetFileLockMetrics()
		for i := 0; i < b.N; i++ {
			fileLock, err := NewFileLock(lockFile)
			if err != nil {
				b.Fatalf("Failed to create file lock: %v", err)
			}

			_ = fileLock.Lock()   //nolint:errcheck // Test setup - errors are acceptable
			_ = fileLock.Unlock() //nolint:errcheck // Test cleanup - errors are acceptable
			fileLock.Close()
		}
	})

	// Test with metrics disabled
	b.Run("WithoutMetrics", func(b *testing.B) {
		ResetFileLockMetrics()
		for i := 0; i < b.N; i++ {
			fileLock, err := NewFileLockWithConfig(lockFile, FileLockConfig{
				EnableMetrics: false,
			})
			if err != nil {
				b.Fatalf("Failed to create file lock: %v", err)
			}

			_ = fileLock.Lock()   //nolint:errcheck // Test setup - errors are acceptable
			_ = fileLock.Unlock() //nolint:errcheck // Test cleanup - errors are acceptable
			fileLock.Close()
		}
	})
}
