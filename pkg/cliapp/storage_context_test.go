package cli

import (
	"context"
	"errors"
	"testing"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestStorageAvailableForOptionalUse(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	var noContext context.Context
	cmd.SetContext(noContext)
	if StorageAvailableForOptionalUse(cmd) {
		t.Error("StorageAvailableForOptionalUse with nil context should be false")
	}
	cmd.SetContext(context.WithValue(context.Background(), storageProviderKey{}, "mock"))
	if !StorageAvailableForOptionalUse(cmd) {
		t.Error("StorageAvailableForOptionalUse with provider in context should be true")
	}
}

func TestGetObjectStorageForCommand_ErrStorageNotInContextWhenProjectRootEmpty(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	var noContext context.Context
	cmd.SetContext(noContext)
	_, err := GetObjectStorageForCommand(cmd, "")
	if err == nil {
		t.Fatal("expected ErrStorageNotInContext when projectRoot is empty")
	}
	if !errors.Is(err, ErrStorageNotInContext) {
		t.Errorf("errors.Is(err, ErrStorageNotInContext) = false; err = %v", err)
	}
}

// TestRegisterStorageForProjectRoot_NoOpWhenEmptyProjectRootOrNilProvider verifies that
// RegisterStorageForProjectRoot does not panic and does not mutate the cache when
// project root is empty or provider is nil (bootstrap guards).
func TestRegisterStorageForProjectRoot_NoOpWhenEmptyProjectRootOrNilProvider(t *testing.T) {
	RegisterStorageForProjectRoot("", nil)
	RegisterStorageForProjectRoot("x", nil)

	// GetObjectStorageForProjectRoot on empty root
	if _, ok := GetObjectStorageForProjectRoot(""); ok {
		t.Errorf("expected ok=false for empty root")
	}
}

func TestStorageContext_ProviderLifecycleAndCache(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.storage_context",
	})
	testRoot := proj.Root
	mockP := proj.FileStorage

	// WithStorageProvider guards
	if WithStorageProvider(nil, "p") != nil {
		t.Errorf("expected nil for nil ctx")
	}
	bg := context.Background()
	if WithStorageProvider(bg, nil) != bg {
		t.Errorf("expected original ctx for nil provider")
	}

	// WithStorageProvider and GetStorageProvider
	ctxWithP := WithStorageProvider(bg, mockP)
	if GetStorageProvider(ctxWithP) != mockP {
		t.Errorf("expected mockP from GetStorageProvider")
	}
	if GetStorageProvider(nil) != nil {
		t.Errorf("expected nil from nil ctx")
	}

	// Register and GetObjectStorageForProjectRoot
	RegisterStorageForProjectRoot(testRoot, mockP)
	gotP, ok := GetObjectStorageForProjectRoot(testRoot)
	if !ok || gotP != mockP {
		t.Errorf("expected registered mockP for root, got ok=%v", ok)
	}

	// GetObjectStorageForCommand with provider in cmd context
	cmd := pkgcli.NewCommandBuilder("test").Build()
	cmd.SetContext(ctxWithP)
	pFromCmd, err := GetObjectStorageForCommand(cmd, testRoot)
	if err != nil || pFromCmd != mockP {
		t.Errorf("expected mockP from command context: %v", err)
	}

	// GetObjectStorageForCommand with provider in cache (empty cmd context)
	cmdNoCtx := pkgcli.NewCommandBuilder("test").Build()
	pFromCache, err := GetObjectStorageForCommand(cmdNoCtx, testRoot)
	if err != nil || pFromCache != mockP {
		t.Errorf("expected mockP from cache: %v", err)
	}

	// GetObjectStorageForCommand creates new storage and invokes OnStorageCreated
	hookCalled := false
	OnStorageCreated = func(p storage.ObjectStorageProvider) {
		hookCalled = true
	}
	defer func() { OnStorageCreated = nil }()

	freshProj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.fresh_storage",
	})
	cmdFresh := pkgcli.NewCommandBuilder("fresh").Build()
	freshP, err := GetObjectStorageForCommand(cmdFresh, freshProj.Root)
	if err != nil || freshP == nil {
		t.Fatalf("unexpected error creating storage: %v", err)
	}
	if !hookCalled {
		t.Errorf("expected OnStorageCreated hook to be called")
	}
}

func TestNewStorageProviderFromFactory(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.factory_test",
	})
	provider, err := NewStorageProviderFromFactory(context.Background(), proj.Root)
	if err != nil {
		t.Fatalf("NewStorageProviderFromFactory failed: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider from factory")
	}
}
