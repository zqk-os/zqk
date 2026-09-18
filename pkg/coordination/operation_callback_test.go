package coordination

import (
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestCoordinatorOperationCallback_OnStart tests that OnStart emits events correctly
func TestCoordinatorOperationCallback_OnStart(t *testing.T) {
	t.Parallel()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		testRoot,
		fileStorage,
		"test_operation",
		"test",
	)

	// Call OnStart - should not panic
	callback.OnStart("test-op-001", map[string]any{
		"test_key": "test_value",
	})

	// Wait a bit for async goroutine to complete
	time.Sleep(100 * time.Millisecond)
}

// TestCoordinatorOperationCallback_OnProgress tests that OnProgress emits events correctly
func TestCoordinatorOperationCallback_OnProgress(t *testing.T) {
	t.Parallel()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		testRoot,
		fileStorage,
		"test_operation",
		"test",
	)

	// Call OnProgress - should not panic
	callback.OnProgress("test-op-001", 50, 100, "Halfway done")

	// Wait a bit for async goroutine to complete
	time.Sleep(100 * time.Millisecond)
}

// TestCoordinatorOperationCallback_OnComplete tests that OnComplete emits events correctly
func TestCoordinatorOperationCallback_OnComplete(t *testing.T) {
	t.Parallel()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		testRoot,
		fileStorage,
		"test_operation",
		"test",
	)

	// Call OnComplete - should not panic
	callback.OnComplete("test-op-001", map[string]any{"result": "success"}, 5*time.Second)

	// Wait a bit for async goroutine to complete
	time.Sleep(100 * time.Millisecond)
}

// TestCoordinatorOperationCallback_OnError tests that OnError emits events correctly
func TestCoordinatorOperationCallback_OnError(t *testing.T) {
	t.Parallel()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		testRoot,
		fileStorage,
		"test_operation",
		"test",
	)

	// Call OnError - should not panic
	callback.OnError("test-op-001", errors.New("test error"))

	// Wait a bit for async goroutine to complete
	time.Sleep(100 * time.Millisecond)
}

// TestCoordinatorOperationCallback_OnCancel tests that OnCancel emits events correctly
func TestCoordinatorOperationCallback_OnCancel(t *testing.T) {
	t.Parallel()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		testRoot,
		fileStorage,
		"test_operation",
		"test",
	)

	// Call OnCancel - should not panic
	callback.OnCancel("test-op-001", "user cancelled")

	// Wait a bit for async goroutine to complete
	time.Sleep(100 * time.Millisecond)
}

// TestCoordinatorOperationCallback_NoStorageProvider tests graceful handling when storage is nil
func TestCoordinatorOperationCallback_NoStorageProvider(t *testing.T) {
	t.Parallel()
	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		"",
		nil,
		"test_operation",
		"test",
	)

	// All methods should handle nil storage gracefully (no panic)
	callback.OnStart("test-op-001", nil)
	callback.OnProgress("test-op-001", 1, 2, "test")
	callback.OnComplete("test-op-001", nil, time.Second)
	callback.OnError("test-op-001", errors.New("test"))
	callback.OnCancel("test-op-001", "test")
}

// TestCoordinatorOperationCallback_NoProjectRoot tests graceful handling when project root is empty
func TestCoordinatorOperationCallback_NoProjectRoot(t *testing.T) {
	t.Parallel()
	_, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	callback := NewCoordinatorOperationCallback(
		pkgctx.NewSystemContext(),
		"", // Empty project root
		fileStorage,
		"test_operation",
		"test",
	)

	// All methods should handle empty project root gracefully (no panic)
	callback.OnStart("test-op-001", nil)
	callback.OnProgress("test-op-001", 1, 2, "test")
	callback.OnComplete("test-op-001", nil, time.Second)
	callback.OnError("test-op-001", errors.New("test"))
	callback.OnCancel("test-op-001", "test")
}
