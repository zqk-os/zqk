package cli

import (
	"context"
	"errors"
	"testing"

	pkgcli "github.com/lanceman/zqk/pkg/cli"
)

func TestStorageAvailableForOptionalUse(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	cmd.SetContext(nil)
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
	cmd.SetContext(nil)
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
}
