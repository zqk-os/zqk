package object

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestKernelSkipsPackOwnedKindsUntilTheRegistrar(t *testing.T) {
	previous := objects.PackOwnedKinds()
	t.Cleanup(func() {
		objects.SetPackOwnedKinds(previous)
		SetPackKindRegistrar(nil)
	})
	objects.SetPackOwnedKinds([]string{"goal"})
	SetPackKindRegistrar(nil)

	withoutPack := &cobra.Command{Use: "object"}
	attachDiscoveredKindCommands(withoutPack, []string{"account", "goal"})
	if names := commandNames(withoutPack); len(names) != 1 || names[0] != "account" {
		t.Fatalf("kernel list: %v", names)
	}

	withPack := &cobra.Command{Use: "object"}
	SetPackKindRegistrar(func(objectCmd *cobra.Command) {
		RegisterKindCommandsForKinds(objectCmd, []string{"goal"})
	})
	attachDiscoveredKindCommands(withPack, []string{"account", "goal"})
	names := commandNames(withPack)
	if len(names) != 2 || names[0] != "account" || names[1] != "goal" {
		t.Fatalf("pack registrar: %v", names)
	}
}

func commandNames(cmd *cobra.Command) []string {
	var names []string
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	return names
}
