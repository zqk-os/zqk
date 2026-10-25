// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package storagetesting

import (
	"errors"
	"sync"
	"testing"
)

var (
	_ IsolationFactory       = (*MockIsolationFactory)(nil)
	_ GlobalHookConfigurator = (*MockIsolationFactory)(nil)
)

// ConfigureTestGlobalsCall captures recorded arguments passed to ConfigureTestGlobals.
type ConfigureTestGlobalsCall struct {
	T    *testing.T
	Opts *GlobalHookOptions
}

// MockIsolationFactory is a thread-safe mock implementation of IsolationFactory
// and GlobalHookConfigurator with configurable function hooks, call counters,
// recorded arguments, and default fallbacks.
type MockIsolationFactory struct {
	mu sync.RWMutex

	// Configurable hooks
	CreateFileStorageFunc    func(testRoot string) (any, error)
	GetAuditBufferFunc       func() any
	ConfigureTestGlobalsFunc func(t *testing.T, opts *GlobalHookOptions)

	// Call counters
	createFileStorageCalls    int
	getAuditBufferCalls       int
	configureTestGlobalsCalls int

	// Recorded arguments
	createFileStorageArgs    []string
	configureTestGlobalsArgs []ConfigureTestGlobalsCall

	// Default fallback values
	defaultStorage any
	defaultBuffer  any
}

// NewMockIsolationFactory constructs an initialized, thread-safe MockIsolationFactory.
func NewMockIsolationFactory() *MockIsolationFactory {
	return &MockIsolationFactory{
		createFileStorageArgs:    make([]string, 0),
		configureTestGlobalsArgs: make([]ConfigureTestGlobalsCall, 0),
	}
}

// WithCreateFileStorage sets the hook for CreateFileStorage.
func (m *MockIsolationFactory) WithCreateFileStorage(fn func(testRoot string) (any, error)) *MockIsolationFactory {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateFileStorageFunc = fn
	return m
}

// WithAuditBuffer sets a static default audit buffer.
func (m *MockIsolationFactory) WithAuditBuffer(buf any) *MockIsolationFactory {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultBuffer = buf
	return m
}

// WithAuditBufferFunc sets the hook for GetAuditBuffer.
func (m *MockIsolationFactory) WithAuditBufferFunc(fn func() any) *MockIsolationFactory {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetAuditBufferFunc = fn
	return m
}

// WithConfigureTestGlobals sets the hook for ConfigureTestGlobals.
func (m *MockIsolationFactory) WithConfigureTestGlobals(fn func(t *testing.T, opts *GlobalHookOptions)) *MockIsolationFactory {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConfigureTestGlobalsFunc = fn
	return m
}

// WithDefaultStorage sets a fallback storage object.
func (m *MockIsolationFactory) WithDefaultStorage(storage any) *MockIsolationFactory {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultStorage = storage
	return m
}

// CreateFileStorage invokes CreateFileStorageFunc if set, or falls back to returning defaultStorage
// or testRoot as storage object (returning an error if testRoot is empty and no default is set).
func (m *MockIsolationFactory) CreateFileStorage(testRoot string) (any, error) {
	m.mu.Lock()
	m.createFileStorageCalls++
	m.createFileStorageArgs = append(m.createFileStorageArgs, testRoot)
	fn := m.CreateFileStorageFunc
	defaultStorage := m.defaultStorage
	m.mu.Unlock()

	if fn != nil {
		return fn(testRoot)
	}
	if defaultStorage != nil {
		return defaultStorage, nil
	}
	if testRoot == "" {
		return nil, errors.New("empty test root")
	}
	return testRoot, nil
}

// GetAuditBuffer invokes GetAuditBufferFunc if set, or falls back to returning defaultBuffer.
func (m *MockIsolationFactory) GetAuditBuffer() any {
	m.mu.Lock()
	m.getAuditBufferCalls++
	fn := m.GetAuditBufferFunc
	defaultBuffer := m.defaultBuffer
	m.mu.Unlock()

	if fn != nil {
		return fn()
	}
	return defaultBuffer
}

// ConfigureTestGlobals invokes ConfigureTestGlobalsFunc if set, recording the call.
func (m *MockIsolationFactory) ConfigureTestGlobals(t *testing.T, opts *GlobalHookOptions) {
	m.mu.Lock()
	m.configureTestGlobalsCalls++
	var optsCopy *GlobalHookOptions
	if opts != nil {
		c := *opts
		optsCopy = &c
	}
	m.configureTestGlobalsArgs = append(m.configureTestGlobalsArgs, ConfigureTestGlobalsCall{
		T:    t,
		Opts: optsCopy,
	})
	fn := m.ConfigureTestGlobalsFunc
	m.mu.Unlock()

	if fn != nil {
		fn(t, opts)
	}
}

// CreateFileStorageCalls returns the number of calls made to CreateFileStorage.
func (m *MockIsolationFactory) CreateFileStorageCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.createFileStorageCalls
}

// GetAuditBufferCalls returns the number of calls made to GetAuditBuffer.
func (m *MockIsolationFactory) GetAuditBufferCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getAuditBufferCalls
}

// ConfigureTestGlobalsCalls returns the number of calls made to ConfigureTestGlobals.
func (m *MockIsolationFactory) ConfigureTestGlobalsCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configureTestGlobalsCalls
}

// CreateFileStorageArgs returns a copy of arguments passed to CreateFileStorage.
func (m *MockIsolationFactory) CreateFileStorageArgs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.createFileStorageArgs))
	copy(result, m.createFileStorageArgs)
	return result
}

// ConfigureTestGlobalsArgs returns a copy of recorded calls to ConfigureTestGlobals.
func (m *MockIsolationFactory) ConfigureTestGlobalsArgs() []ConfigureTestGlobalsCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ConfigureTestGlobalsCall, len(m.configureTestGlobalsArgs))
	copy(result, m.configureTestGlobalsArgs)
	return result
}

// Reset clears all recorded calls and counters while preserving configured hooks and defaults.
func (m *MockIsolationFactory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFileStorageCalls = 0
	m.getAuditBufferCalls = 0
	m.configureTestGlobalsCalls = 0
	m.createFileStorageArgs = make([]string, 0)
	m.configureTestGlobalsArgs = make([]ConfigureTestGlobalsCall, 0)
}
