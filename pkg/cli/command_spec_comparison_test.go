package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCommandSpec_ComparisonWithProgrammatic tests that spec-built commands
// produce equivalent results to programmatic builders
func TestCommandSpec_ComparisonWithProgrammatic(t *testing.T) {
	t.Parallel()
	testdataDir := "testdata"
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	testdataPath := filepath.Join(wd, testdataDir)

	tests := []struct {
		name        string
		specFile    string
		buildProg   func() *cobra.Command
		compareFunc func(t *testing.T, specCmd, progCmd *cobra.Command)
	}{
		{
			name:     "get command",
			specFile: "get_command.yaml",
			buildProg: func() *cobra.Command {
				return NewCommandBuilder("get <id>").
					WithShort("Get an object by ID").
					WithHelpBuilder(
						DynamicHelpBuilder(
							"Get an object by ID",
							`Get an object by its ID.

The object kind is inferred from the ID format (e.g., BLI-001 -> backlog_item).`,
						).
							AddExample("Get a backlog item", "%s get BLI-626").
							AddExample("Get with JSON output", "%s get BLI-626 --format json").
							AddExample("Get with YAML output", "%s get BLI-626 --format yaml").
							ExcludeCommonFlags(),
					).
					WithArgs(cobra.ExactArgs(1)).
					WithCommonFlagsDefault(func(cmd *cobra.Command) {
						// Mock common flags for testing
						cmd.Flags().String("format", "table", "Output format")
					}).
					Build()
			},
			compareFunc: func(t *testing.T, specCmd, progCmd *cobra.Command) {
				// Compare basic properties
				if specCmd.Use != progCmd.Use {
					t.Errorf("Use mismatch: spec=%s, prog=%s", specCmd.Use, progCmd.Use)
				}
				if specCmd.Short != progCmd.Short {
					t.Errorf("Short mismatch: spec=%s, prog=%s", specCmd.Short, progCmd.Short)
				}
				// Both should have args validation
				if specCmd.Args == nil || progCmd.Args == nil {
					t.Error("Both commands should have args validation")
				}
			},
		},
		{
			name:     "delete command (CRUD)",
			specFile: "delete_command.yaml",
			buildProg: func() *cobra.Command {
				return NewCRUDCommandBuilder("delete", "delete <id> [flags]").
					WithCRUDHelp(
						"Delete an object by ID",
						`Delete an object by its ID.

By default, deletion will fail if the object has dependents (objects that reference it).
Use --cascade to delete the object and all its dependents recursively.`,
						"Delete an object (fails if it has dependents)", "%s delete BLI-626",
						"Delete with cascade (deletes object and all dependents)", "%s delete BLI-626 --cascade",
						"Dry-run to see what would be deleted", "%s delete BLI-626 --cascade --dry-run",
					).
					WithArgs(cobra.ExactArgs(1)).
					WithCommonFlagsDefault(func(cmd *cobra.Command) {
						// Mock common flags for testing
						cmd.Flags().String("format", "table", "Output format")
					}).
					WithCascadeFlag().
					WithDryRunFlag().
					Build()
			},
			compareFunc: func(t *testing.T, specCmd, progCmd *cobra.Command) {
				// Compare basic properties
				if specCmd.Use != progCmd.Use {
					t.Errorf("Use mismatch: spec=%s, prog=%s", specCmd.Use, progCmd.Use)
				}
				if specCmd.Short != progCmd.Short {
					t.Errorf("Short mismatch: spec=%s, prog=%s", specCmd.Short, progCmd.Short)
				}
				// Both should have cascade and dry-run flags
				specCascade := specCmd.Flags().Lookup("cascade")
				progCascade := progCmd.Flags().Lookup("cascade")
				if (specCascade == nil) != (progCascade == nil) {
					t.Error("cascade flag presence mismatch")
				}
				specDryRun := specCmd.Flags().Lookup("dry-run")
				progDryRun := progCmd.Flags().Lookup("dry-run")
				if (specDryRun == nil) != (progDryRun == nil) {
					t.Error("dry-run flag presence mismatch")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load spec from file
			specPath := filepath.Join(testdataPath, tt.specFile)
			data, err := fileutil.ReadFile(specPath)
			if err != nil {
				t.Fatalf("failed to read spec file: %v", err)
			}

			// Determine if it's a CRUD spec or regular spec
			var isCRUD bool
			var tempSpec map[string]any
			if err := yaml.Unmarshal(data, &tempSpec); err == nil {
				_, isCRUD = tempSpec["operation_type"]
			}

			var specCmd *cobra.Command
			if isCRUD {
				// Parse as CRUDCommandSpec
				var crudSpec CRUDCommandSpec
				if err := yaml.Unmarshal(data, &crudSpec); err != nil {
					t.Fatalf("failed to parse CRUD spec: %v", err)
				}
				if err := crudSpec.Validate(); err != nil {
					t.Fatalf("CRUD spec validation failed: %v", err)
				}

				builder := NewCRUDCommandSpecBuilder(&crudSpec)
				builder.RegisterRunE("runDelete", func(cmd *cobra.Command, args []string) error {
					return nil
				})

				var err error
				specCmd, err = builder.Build()
				if err != nil {
					t.Fatalf("failed to build CRUD command from spec: %v", err)
				}
			} else {
				// Parse as regular CommandSpec
				var spec CommandSpec
				if err := yaml.Unmarshal(data, &spec); err != nil {
					t.Fatalf("failed to parse spec: %v", err)
				}
				if err := spec.Validate(); err != nil {
					t.Fatalf("spec validation failed: %v", err)
				}

				builder := NewCommandSpecBuilder(&spec)
				builder.RegisterRunE("runGet", func(cmd *cobra.Command, args []string) error {
					return nil
				})

				var err error
				specCmd, err = builder.Build()
				if err != nil {
					t.Fatalf("failed to build command from spec: %v", err)
				}
			}

			// Build programmatic command
			progCmd := tt.buildProg()

			// Compare
			tt.compareFunc(t, specCmd, progCmd)
		})
	}
}
