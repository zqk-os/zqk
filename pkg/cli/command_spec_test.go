package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mockProcessorProvider is a test implementation of ProcessorProvider
type mockProcessorProvider struct {
	projectRoot string
	ctx         context.Context
}

func (m *mockProcessorProvider) ProjectRoot() string {
	return m.projectRoot
}

func (m *mockProcessorProvider) OperationContext() context.Context {
	if m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

func TestCommandSpec_LoadFromYAML(t *testing.T) {
	t.Parallel()
	testdataDir := "testdata"

	// Get absolute path to testdata
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	testdataPath := filepath.Join(wd, testdataDir)

	tests := []struct {
		name     string
		specFile string
		validate func(t *testing.T, cmd *cobra.Command)
	}{
		{
			name:     "get command spec",
			specFile: "get_command.yaml",
			validate: func(t *testing.T, cmd *cobra.Command) {
				if cmd.Use != "get <id>" {
					t.Errorf("expected Use='get <id>', got '%s'", cmd.Use)
				}
				if cmd.Short != "Get an object by ID" {
					t.Errorf("expected Short='Get an object by ID', got '%s'", cmd.Short)
				}
				if cmd.Args == nil {
					t.Error("Args should not be nil")
				}
				// Test that args validation works
				if err := cmd.Args(cmd, []string{"BLI-626"}); err != nil {
					t.Errorf("Args validation failed for valid input: %v", err)
				}
				if err := cmd.Args(cmd, []string{"BLI-626", "extra"}); err == nil {
					t.Error("Args validation should fail for too many args")
				}
			},
		},
		{
			name:     "delete command spec (CRUD)",
			specFile: "delete_command.yaml",
			validate: func(t *testing.T, cmd *cobra.Command) {
				if cmd.Use != "delete <id> [flags]" {
					t.Errorf("expected Use='delete <id> [flags]', got '%s'", cmd.Use)
				}
				if cmd.Short != "Delete an object by ID" {
					t.Errorf("expected Short='Delete an object by ID', got '%s'", cmd.Short)
				}
				// Check that CRUD flags were added via WithCascadeFlag/WithDryRunFlag
				cascadeFlag := cmd.Flags().Lookup("cascade")
				if cascadeFlag == nil {
					t.Error("cascade flag should exist (added via cascade: true)")
				}
				dryRunFlag := cmd.Flags().Lookup("dry-run")
				if dryRunFlag == nil {
					t.Error("dry-run flag should exist (added via dry_run: true)")
				}
				// Test args validation
				if err := cmd.Args(cmd, []string{"BLI-626"}); err != nil {
					t.Errorf("Args validation failed for valid input: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(testdataPath, tt.specFile)

			// Load spec from file
			data, err := fileutil.ReadFile(specPath)
			if err != nil {
				t.Fatalf("failed to read spec file: %v", err)
			}

			// Determine if it's a CRUD spec or regular spec
			var tempSpec map[string]any
			var isCRUD bool
			if parseErr := yaml.Unmarshal(data, &tempSpec); parseErr == nil {
				_, isCRUD = tempSpec["operation_type"]
			}

			var cmd *cobra.Command
			var buildErr error

			if isCRUD {
				// Parse as CRUDCommandSpec
				var crudSpec CRUDCommandSpec
				if parseErr := yaml.Unmarshal(data, &crudSpec); parseErr != nil {
					t.Fatalf("failed to parse CRUD spec: %v", parseErr)
				}
				if validateErr := crudSpec.Validate(); validateErr != nil {
					t.Fatalf("CRUD spec validation failed: %v", validateErr)
				}

				builder := NewCRUDCommandSpecBuilder(&crudSpec)
				builder.RegisterRunE("runDelete", func(cmd *cobra.Command, args []string) error {
					return nil
				})

				cmd, buildErr = builder.Build()
			} else {
				// Parse as regular CommandSpec
				var spec CommandSpec
				if parseErr := yaml.Unmarshal(data, &spec); parseErr != nil {
					t.Fatalf("failed to parse spec: %v", parseErr)
				}
				if validateErr := spec.Validate(); validateErr != nil {
					t.Fatalf("spec validation failed: %v", validateErr)
				}

				builder := NewCommandSpecBuilder(&spec)
				builder.RegisterRunE("runGet", func(cmd *cobra.Command, args []string) error {
					return nil
				})

				cmd, buildErr = builder.Build()
			}

			if buildErr != nil {
				t.Fatalf("failed to build command: %v", buildErr)
			}

			// Validate built command
			tt.validate(t, cmd)
		})
	}
}

func TestCRUDCommandSpec_LoadFromYAML(t *testing.T) {
	t.Parallel()
	testdataDir := "testdata"
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	testdataPath := filepath.Join(wd, testdataDir)

	specPath := filepath.Join(testdataPath, "delete_command.yaml")

	// Load spec from file
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read spec file: %v", err)
	}

	// Parse as CRUDCommandSpec
	var crudSpec CRUDCommandSpec
	if err := yaml.Unmarshal(data, &crudSpec); err != nil {
		t.Fatalf("failed to parse CRUD spec: %v", err)
	}

	// Validate spec
	if err := crudSpec.Validate(); err != nil {
		t.Fatalf("CRUD spec validation failed: %v", err)
	}

	// Build command from CRUD spec
	builder := NewCRUDCommandSpecBuilder(&crudSpec)
	builder.RegisterRunE("runDelete", func(cmd *cobra.Command, args []string) error {
		return nil
	})

	cmd, err := builder.Build()
	if err != nil {
		t.Fatalf("failed to build CRUD command: %v", err)
	}

	// Validate CRUD-specific features
	if cmd.Use != "delete <id> [flags]" {
		t.Errorf("expected Use='delete <id> [flags]', got '%s'", cmd.Use)
	}

	// Check that CRUD flags were added via WithCascadeFlag and WithDryRunFlag
	cascadeFlag := cmd.Flags().Lookup("cascade")
	if cascadeFlag == nil {
		t.Error("cascade flag should exist (added via WithCascadeFlag)")
	}
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("dry-run flag should exist (added via WithDryRunFlag)")
	}
}

func TestCommandSpec_ExternalSpecReference(t *testing.T) {
	t.Parallel()
	testdataDir := "testdata"
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	testdataPath := filepath.Join(wd, testdataDir)

	// Create a parent spec that references an external spec
	parentSpec := &CommandSpec{
		Name:  "parent",
		Short: "Parent command",
		Subcommands: []SubcommandSpec{
			{
				Name:    "get",
				SpecRef: "get_command.yaml",
			},
		},
	}

	// Build with processor pointing to testdata directory
	mockProc := &mockProcessorProvider{
		projectRoot: wd, // Use test directory as project root
		ctx:         context.Background(),
	}

	builder := NewCommandSpecBuilder(parentSpec).
		WithProcessor(mockProc).
		WithSpecsDir(testdataPath).
		RegisterRunE("runGet", func(cmd *cobra.Command, args []string) error {
			return nil
		})

	cmd, err := builder.Build()
	if err != nil {
		t.Fatalf("failed to build command with external spec reference: %v", err)
	}

	// Validate that subcommand was loaded from external spec
	if len(cmd.Commands()) != 1 {
		t.Fatalf("expected 1 subcommand, got %d", len(cmd.Commands()))
	}

	subcmd := cmd.Commands()[0]
	if subcmd.Use != "get <id>" {
		t.Errorf("expected subcommand Use='get <id>', got '%s'", subcmd.Use)
	}
	if subcmd.Short != "Get an object by ID" {
		t.Errorf("expected subcommand Short='Get an object by ID', got '%s'", subcmd.Short)
	}
}

func TestCommandSpec_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spec    *CommandSpec
		wantErr bool
	}{
		{
			name: "valid spec",
			spec: &CommandSpec{
				Name:  "test",
				Short: "Test command",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			spec: &CommandSpec{
				Short: "Test command",
			},
			wantErr: true,
		},
		{
			name: "missing short and description",
			spec: &CommandSpec{
				Name: "test",
			},
			wantErr: true,
		},
		{
			name: "valid with description only",
			spec: &CommandSpec{
				Name:        "test",
				Description: "Test description",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCRUDCommandSpec_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spec    *CRUDCommandSpec
		wantErr bool
	}{
		{
			name: "valid CRUD spec",
			spec: &CRUDCommandSpec{
				CommandSpec: CommandSpec{
					Name:  "create",
					Short: "Create command",
				},
				OperationType: "create",
			},
			wantErr: false,
		},
		{
			name: "missing operation type",
			spec: &CRUDCommandSpec{
				CommandSpec: CommandSpec{
					Name:  "create",
					Short: "Create command",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid operation type",
			spec: &CRUDCommandSpec{
				CommandSpec: CommandSpec{
					Name:  "create",
					Short: "Create command",
				},
				OperationType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
