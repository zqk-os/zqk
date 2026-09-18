// Package storagetesting holds the minimal interfaces and option structs shared by
// [github.com/zqk-os/zqk/pkg/testing.SetupCompleteTestEnvironment] and
// [github.com/zqk-os/zqk/pkg/storage.TestingFactory].
//
// This package exists because if pkg/testing imported pkg/storage directly, tests that use both
// would risk an import cycle (tests → pkg/testing → pkg/storage). Defining the contracts here keeps
// pkg/testing and pkg/storage mutually independent at import time while still allowing a typed
// factory hook.
//
// Storage tests: pkg/storage no longer need to import pkg/testing for setup; they use local temp
// roots, test helpers under package storage (e.g. setupTestingFactoryCompleteTestEnvironment,
// bootstrapTestRootFromProjectRoot, scenario helpers), and export_test symbols for storage_test.
// pkg/testing still uses these contracts when other packages call SetupCompleteTestEnvironment with
// [github.com/zqk-os/zqk/pkg/storage.NewTestingFactory].
//
// # Retiring this package (optional future)
//
// If pkg/testing ever imports pkg/storage directly for [IsolationFactory], storagetesting could
// fold into pkg/testing or pkg/storage; until then this thin package remains the stable seam.
package storagetesting

import "testing"

// IsolationFactory creates isolated file storage and exposes the global audit buffer for test setup.
// Implemented by [github.com/zqk-os/zqk/pkg/storage.TestingFactory].
type IsolationFactory interface {
	CreateFileStorage(testRoot string) (any, error)
	GetAuditBuffer() any
}

// GlobalHookOptions is the subset of test-setup flags storage-level hooks read.
type GlobalHookOptions struct {
	NoopStorageMetrics bool
}

// GlobalHookConfigurator is optionally implemented alongside [IsolationFactory].
type GlobalHookConfigurator interface {
	ConfigureTestGlobals(t *testing.T, opts *GlobalHookOptions)
}
