package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/specbuilder/registry"
)

// NewGenerateFieldRegistryCmd creates the system generate-field-registry command
func NewGenerateFieldRegistryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate-field-registry",
		Short: "Generate field ID registry from specs",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := registry.GenerateRegistry()
			if err != nil {
				return err
			}
			path := "field_registry.json"
			if err := registry.SaveRegistry(path, reg); err != nil {
				return err
			}
			msg := fmt.Sprintf("Field ID registry generated at %s\n", path)
			return cli.WriteOutput(cmd, []byte(msg))
		},
	}
}
