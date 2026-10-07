package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyBrandingToCommandTree(t *testing.T) {
	root := &cobra.Command{
		Use:     "zqk",
		Short:   "zqk is the core command for ZQK knowledge kernel",
		Long:    "Run zqk commands to interact with ZQK objects.",
		Example: "zqk object list",
	}
	child := &cobra.Command{
		Use:     "child",
		Short:   "child of zqk",
		Long:    "Long child description for ZQK",
		Example: "zqk child run",
	}
	root.AddCommand(child)

	ApplyBrandingToCommandTree(root, "mytool", "MyBrand")

	if !strings.Contains(root.Short, "mytool") {
		t.Errorf("expected root.Short to contain 'mytool', got: %s", root.Short)
	}
	if !strings.Contains(root.Short, "MYBRAND") {
		t.Errorf("expected root.Short to contain 'MYBRAND', got: %s", root.Short)
	}
	if !strings.Contains(child.Short, "mytool") {
		t.Errorf("expected child.Short to contain 'mytool', got: %s", child.Short)
	}

	// Nil command should not panic
	ApplyBrandingToCommandTree(nil, "mytool", "MyBrand")
}
