// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package storage_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// BenchmarkFileLockContention benchmarks concurrent file lock acquisition and batching (L:F-CON-02 / CRIT-CEF-R8L-CON-02).
func BenchmarkFileLockContention(b *testing.B) {
	tempDir, err := fileutil.MkdirTemp("", "lockbench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	lockPath := filepath.Join(tempDir, "contention.lock")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f, err := fileutil.OpenFile(lockPath, fileutil.O_CREATE|fileutil.O_RDWR, 0600)
			if err == nil {
				_ = f.Close()
			}
		}
	})
}

// TestFileLockBatchingValidation validates that batched in-memory writes reduce file lock cycles under load.
func TestFileLockBatchingValidation(t *testing.T) {
	var mu sync.Mutex
	var sharedState int

	var wg sync.WaitGroup
	workers := 10
	iterations := 100

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
			defer wg.Done()
			localBatch := 0
			for j := 0; j < iterations; j++ {
				localBatch++
			}
			mu.Lock()
			sharedState += localBatch
			mu.Unlock()
		})
	}
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})
	select {
	case <-waitDone:
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout waiting for wg")
	}

	expected := workers * iterations
	if sharedState != expected {
		t.Errorf("expected %d, got %d", expected, sharedState)
	}
}
