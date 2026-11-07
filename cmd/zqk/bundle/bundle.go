package bundle

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/bundle"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	"gopkg.in/yaml.v3"
)

// NewBundleCmd returns the root 'bundle' command suite.
func NewBundleCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewBundleCommandBuilder(), &cobra.Command{})

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newRemoveCmd())

	return cmd
}

func resolveRoot(cmd *cobra.Command) string {
	root, _ := cliapp.ResolveCommandProjectRoot(cmd)
	if root == "" {
		root = cliapp.ResolveProjectRoot(".")
	}
	if root == "" {
		root = "."
	}
	return root
}

func newListCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewBundleListCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			root := resolveRoot(cmd)
			reg := bundle.NewRegistry(root)
			bundles, err := reg.ListInstalled()
			if err != nil {
				return err
			}

			switch format {
			case "json":
				data, err := json.MarshalIndent(bundles, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			case "yaml":
				data, err := yaml.Marshal(bundles)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(data))
			default:
				if len(bundles) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No language bundles currently installed in this workspace.")
					return nil
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "NAME\tLANGUAGE\tVERSION\tDESCRIPTION")
				for _, b := range bundles {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Name, b.Language, b.Version, b.Description)
				}
				_ = w.Flush()
			}
			return nil
		},
	})
	return cmd
}

func newInfoCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewBundleInfoCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			root := resolveRoot(cmd)
			reg := bundle.NewRegistry(root)
			b, err := reg.Get(args[0])
			if err != nil {
				return err
			}

			switch format {
			case "yaml":
				data, err := yaml.Marshal(b)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(data))
			default:
				data, err := json.MarshalIndent(b, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			}
			return nil
		},
	})
	return cmd
}

func newInstallCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewBundleAddCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			file, _ := cmd.Flags().GetString("file")
			if file == "" {
				return fmt.Errorf("--file flag is required")
			}
			m, err := bundle.LoadManifestFile(file)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}

			root := resolveRoot(cmd)
			reg := bundle.NewRegistry(root)
			if err := reg.Install(m); err != nil {
				return fmt.Errorf("install bundle: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Language bundle %q (%s v%s) installed successfully.\n", m.Name, m.Language, m.Version)
			return nil
		},
	})
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewBundleRemoveCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			root := resolveRoot(cmd)
			reg := bundle.NewRegistry(root)
			if err := reg.Remove(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Language bundle %q uninstalled.\n", args[0])
			return nil
		},
	})
	return cmd
}
