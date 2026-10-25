// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package storagetesting

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestMockIsolationFactory_InterfaceSatisfaction(t *testing.T) {
	t.Parallel()

	mock := NewMockIsolationFactory()
	var factory IsolationFactory = mock
	var configurator GlobalHookConfigurator = mock

	if factory == nil || configurator == nil {
		t.Fatalf("expected non-nil factory and configurator")
	}
}

func TestMockIsolationFactory_DefaultsAndFallbacks(t *testing.T) {
	t.Parallel()

	mock := NewMockIsolationFactory()

	// Initial counters and empty arguments
	if calls := mock.CreateFileStorageCalls(); calls != 0 {
		t.Fatalf("expected 0 calls, got %d", calls)
	}
	if calls := mock.GetAuditBufferCalls(); calls != 0 {
		t.Fatalf("expected 0 calls, got %d", calls)
	}
	if calls := mock.ConfigureTestGlobalsCalls(); calls != 0 {
		t.Fatalf("expected 0 calls, got %d", calls)
	}
	if len(mock.CreateFileStorageArgs()) != 0 {
		t.Fatalf("expected empty args")
	}
	if len(mock.ConfigureTestGlobalsArgs()) != 0 {
		t.Fatalf("expected empty args")
	}

	// Default CreateFileStorage with valid root
	res, err := mock.CreateFileStorage("/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "/tmp/workspace" {
		t.Fatalf("expected '/tmp/workspace', got %v", res)
	}

	// Default CreateFileStorage with empty root returns error
	_, err = mock.CreateFileStorage("")
	if err == nil {
		t.Fatalf("expected error for empty test root")
	}

	// Default GetAuditBuffer returns nil
	buf := mock.GetAuditBuffer()
	if buf != nil {
		t.Fatalf("expected nil buffer, got %v", buf)
	}

	// Default ConfigureTestGlobals with nil and non-nil opts
	mock.ConfigureTestGlobals(t, nil)
	mock.ConfigureTestGlobals(t, &GlobalHookOptions{NoopStorageMetrics: true})

	if calls := mock.CreateFileStorageCalls(); calls != 2 {
		t.Fatalf("expected 2 CreateFileStorage calls, got %d", calls)
	}
	if calls := mock.GetAuditBufferCalls(); calls != 1 {
		t.Fatalf("expected 1 GetAuditBuffer calls, got %d", calls)
	}
	if calls := mock.ConfigureTestGlobalsCalls(); calls != 2 {
		t.Fatalf("expected 2 ConfigureTestGlobals calls, got %d", calls)
	}

	args := mock.CreateFileStorageArgs()
	if len(args) != 2 || args[0] != "/tmp/workspace" || args[1] != "" {
		t.Fatalf("unexpected CreateFileStorageArgs: %v", args)
	}

	hookArgs := mock.ConfigureTestGlobalsArgs()
	if len(hookArgs) != 2 {
		t.Fatalf("expected 2 hook args, got %d", len(hookArgs))
	}
	if hookArgs[0].Opts != nil {
		t.Fatalf("expected nil opts for first call")
	}
	if hookArgs[1].Opts == nil || !hookArgs[1].Opts.NoopStorageMetrics {
		t.Fatalf("expected NoopStorageMetrics=true for second call")
	}
}

func TestMockIsolationFactory_BuilderAndHooks(t *testing.T) {
	t.Parallel()

	customErr := errors.New("storage creation failure")
	customStorage := "custom-storage-handle"
	customBuffer := "custom-audit-buffer"

	var hookExecuted bool

	mock := NewMockIsolationFactory().
		WithCreateFileStorage(func(testRoot string) (any, error) {
			if testRoot == "fail" {
				return nil, customErr
			}
			return customStorage, nil
		}).
		WithAuditBufferFunc(func() any {
			return customBuffer
		}).
		WithConfigureTestGlobals(func(t *testing.T, opts *GlobalHookOptions) {
			if opts != nil && opts.NoopStorageMetrics {
				hookExecuted = true
			}
		})

	// Test custom CreateFileStorage hook
	res, err := mock.CreateFileStorage("/normal/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != customStorage {
		t.Fatalf("expected %v, got %v", customStorage, res)
	}

	_, err = mock.CreateFileStorage("fail")
	if !errors.Is(err, customErr) {
		t.Fatalf("expected custom error, got %v", err)
	}

	// Test custom AuditBuffer hook
	buf := mock.GetAuditBuffer()
	if buf != customBuffer {
		t.Fatalf("expected %v, got %v", customBuffer, buf)
	}

	// Test custom ConfigureTestGlobals hook
	opts := &GlobalHookOptions{NoopStorageMetrics: true}
	mock.ConfigureTestGlobals(t, opts)
	if !hookExecuted {
		t.Fatalf("expected hook to be executed")
	}
}

func TestMockIsolationFactory_WithDefaultStorageAndBuffer(t *testing.T) {
	t.Parallel()

	fallbackStorage := struct{ Name string }{Name: "fallback-storage"}
	fallbackBuffer := struct{ ID int }{ID: 42}

	mock := NewMockIsolationFactory().
		WithDefaultStorage(fallbackStorage).
		WithAuditBuffer(fallbackBuffer)

	// Even with empty root, fallback storage is returned
	res, err := mock.CreateFileStorage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != fallbackStorage {
		t.Fatalf("expected %v, got %v", fallbackStorage, res)
	}

	buf := mock.GetAuditBuffer()
	if buf != fallbackBuffer {
		t.Fatalf("expected %v, got %v", fallbackBuffer, buf)
	}
}

func TestMockIsolationFactory_Reset(t *testing.T) {
	t.Parallel()

	mock := NewMockIsolationFactory().
		WithAuditBuffer("preserved-buffer")

	_, err := mock.CreateFileStorage("/tmp/alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = mock.GetAuditBuffer()
	mock.ConfigureTestGlobals(t, &GlobalHookOptions{NoopStorageMetrics: false})

	if mock.CreateFileStorageCalls() != 1 || mock.GetAuditBufferCalls() != 1 || mock.ConfigureTestGlobalsCalls() != 1 {
		t.Fatalf("expected calls to be 1")
	}

	mock.Reset()

	if calls := mock.CreateFileStorageCalls(); calls != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", calls)
	}
	if calls := mock.GetAuditBufferCalls(); calls != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", calls)
	}
	if calls := mock.ConfigureTestGlobalsCalls(); calls != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", calls)
	}
	if len(mock.CreateFileStorageArgs()) != 0 {
		t.Fatalf("expected empty args after reset")
	}
	if len(mock.ConfigureTestGlobalsArgs()) != 0 {
		t.Fatalf("expected empty args after reset")
	}

	// Preserved default buffer still works
	buf := mock.GetAuditBuffer()
	if buf != "preserved-buffer" {
		t.Fatalf("expected 'preserved-buffer', got %v", buf)
	}
}

func TestMockIsolationFactory_Concurrency(t *testing.T) {
	t.Parallel()

	mock := NewMockIsolationFactory()
	const workers = 10
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workers * 3)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("worker-storage-%d", workerID), "concurrent mock create storage").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				path := fmt.Sprintf("/tmp/worker-%d/iter-%d", workerID, j)
				storage, err := mock.CreateFileStorage(path)
				if err != nil || storage != path {
					t.Errorf("worker %d unexpected: %v, %v", workerID, storage, err)
				}
			}
		})

		goroutinelabels.NewGoroutine(fmt.Sprintf("worker-buffer-%d", workerID), "concurrent mock get audit buffer").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = mock.GetAuditBuffer()
			}
		})

		goroutinelabels.NewGoroutine(fmt.Sprintf("worker-hook-%d", workerID), "concurrent mock configure test globals").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				mock.ConfigureTestGlobals(t, &GlobalHookOptions{NoopStorageMetrics: (j % 2) == 0})
			}
		})
	}

	wg.Wait()

	expectedCalls := workers * iterations
	if calls := mock.CreateFileStorageCalls(); calls != expectedCalls {
		t.Fatalf("expected %d CreateFileStorageCalls, got %d", expectedCalls, calls)
	}
	if calls := mock.GetAuditBufferCalls(); calls != expectedCalls {
		t.Fatalf("expected %d GetAuditBufferCalls, got %d", expectedCalls, calls)
	}
	if calls := mock.ConfigureTestGlobalsCalls(); calls != expectedCalls {
		t.Fatalf("expected %d ConfigureTestGlobalsCalls, got %d", expectedCalls, calls)
	}

	args := mock.CreateFileStorageArgs()
	if len(args) != expectedCalls {
		t.Fatalf("expected %d args, got %d", expectedCalls, len(args))
	}

	hookArgs := mock.ConfigureTestGlobalsArgs()
	if len(hookArgs) != expectedCalls {
		t.Fatalf("expected %d hook args, got %d", expectedCalls, len(hookArgs))
	}
}
