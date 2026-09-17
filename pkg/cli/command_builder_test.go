package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandBuilder_Basic(t *testing.T) {
	t.Parallel()
	cmd := NewCommandBuilder("test <arg>").
		WithShort("Test command").
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(func(cmd *cobra.Command, args []string) error {
			return nil
		}).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Use != "test <arg>" {
		t.Errorf("expected Use='test <arg>', got '%s'", cmd.Use)
	}
	if cmd.Short != "Test command" {
		t.Errorf("expected Short='Test command', got '%s'", cmd.Short)
	}
	if cmd.Args == nil {
		t.Error("Args should not be nil")
	}
	if cmd.RunE == nil {
		t.Error("RunE should not be nil")
	}
}

func TestCommandBuilder_WithHelpBuilder(t *testing.T) {
	t.Parallel()
	helpBuilder := DynamicHelpBuilder(
		"Test command",
		"This is a test command description.",
	).
		AddExample("Test example", "%s test arg1")

	cmd := NewCommandBuilder("test <arg>").
		WithHelpBuilder(helpBuilder).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(func(cmd *cobra.Command, args []string) error {
			return nil
		}).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Long == emptyValue {
		t.Fatal("cmd.Long should not be empty")
	}
	// Help builder sets Short from first parameter, Long from description
	if cmd.Short != "Test command" {
		t.Errorf("expected Short='Test command', got '%s'", cmd.Short)
	}
	if !contains(cmd.Long, "test command description") {
		t.Errorf("cmd.Long should contain 'test command description', got: %s", cmd.Long)
	}
	if !contains(cmd.Long, "zqk test arg1") {
		t.Errorf("cmd.Long should contain example 'zqk test arg1', got: %s", cmd.Long)
	}
}

func TestCommandBuilder_WithFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCommandBuilder("test").
		AddStringFlag("file", "f", "default.txt", "File path").
		AddBoolFlag("verbose", "v", false, "Verbose output").
		AddIntFlag("count", "c", 10, "Count value").
		AddStringArrayFlag("tags", "t", "Tags list").
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}

	// Check string flag
	fileFlag := cmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Fatal("file flag should exist")
	}
	if fileFlag.Shorthand != "f" {
		t.Errorf("expected shorthand 'f', got '%s'", fileFlag.Shorthand)
	}
	if fileFlag.DefValue != "default.txt" {
		t.Errorf("expected default 'default.txt', got '%s'", fileFlag.DefValue)
	}

	// Check bool flag
	verboseFlag := cmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("verbose flag should exist")
	}
	if verboseFlag.Shorthand != "v" {
		t.Errorf("expected shorthand 'v', got '%s'", verboseFlag.Shorthand)
	}

	// Check int flag
	countFlag := cmd.Flags().Lookup("count")
	if countFlag == nil {
		t.Fatal("count flag should exist")
	}
	if countFlag.Shorthand != "c" {
		t.Errorf("expected shorthand 'c', got '%s'", countFlag.Shorthand)
	}

	// Check string array flag
	tagsFlag := cmd.Flags().Lookup("tags")
	if tagsFlag == nil {
		t.Fatal("tags flag should exist")
	}
	if tagsFlag.Shorthand != "t" {
		t.Errorf("expected shorthand 't', got '%s'", tagsFlag.Shorthand)
	}
}

func TestCommandBuilder_WithRequiredFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCommandBuilder("test").
		AddFlag(FlagConfig{
			Name:        "required",
			Type:        FlagTypeString,
			Description: "Required field",
			Required:    true,
		}).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	requiredFlag := cmd.Flags().Lookup("required")
	if requiredFlag == nil {
		t.Fatal("required flag should exist")
	}
	// Note: MarkFlagRequired is called during Build, but validation happens at runtime
}

func TestCommandBuilder_WithSubcommands(t *testing.T) {
	t.Parallel()
	subcmd1 := &cobra.Command{Use: "sub1"}
	subcmd2 := &cobra.Command{Use: "sub2"}

	cmd := NewCommandBuilder("test").
		AddSubcommand(subcmd1).
		AddSubcommand(subcmd2).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if len(cmd.Commands()) != 2 {
		t.Fatalf("expected 2 subcommands, got %d", len(cmd.Commands()))
	}
	if cmd.Commands()[0].Use != "sub1" {
		t.Errorf("expected first subcommand Use='sub1', got '%s'", cmd.Commands()[0].Use)
	}
	if cmd.Commands()[1].Use != "sub2" {
		t.Errorf("expected second subcommand Use='sub2', got '%s'", cmd.Commands()[1].Use)
	}
}

func TestCommandBuilder_WithCommonFlags(t *testing.T) {
	t.Parallel()
	commonFlagsAdded := false
	addCommonFlagsFunc := func(cmd *cobra.Command) {
		commonFlagsAdded = true
		cmd.Flags().String("format", "json", "Output format")
	}

	cmd := NewCommandBuilder("test").
		WithCommonFlagsDefault(addCommonFlagsFunc).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if !commonFlagsAdded {
		t.Error("common flags function should have been called")
	}
	if cmd.Flags().Lookup("format") == nil {
		t.Error("format flag should exist")
	}
}

func TestCRUDCommandBuilder_Basic(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("create", "create <kind>").
		WithCRUDHelp(
			"Create an object",
			"Create a new object of the specified kind.",
			"Create example", "%s create backlog_item",
		).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(func(cmd *cobra.Command, args []string) error {
			return nil
		}).
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Use != "create <kind>" {
		t.Errorf("expected Use='create <kind>', got '%s'", cmd.Use)
	}
	// Help builder sets Short from first parameter, Long from description
	if cmd.Short != "Create an object" {
		t.Errorf("expected Short='Create an object', got '%s'", cmd.Short)
	}
	if !contains(cmd.Long, "Create a new object of the specified kind") {
		t.Errorf("cmd.Long should contain 'Create a new object of the specified kind', got: %s", cmd.Long)
	}
}

func TestCRUDCommandBuilder_WithDataInputFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("create", "create").
		WithDataInputFlags().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Flags().Lookup("file") == nil {
		t.Error("file flag should exist")
	}
	if cmd.Flags().Lookup("data") == nil {
		t.Error("data flag should exist")
	}
}

func TestCRUDCommandBuilder_WithUpdateFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("update", "update <id>").
		WithUpdateFlags().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Flags().Lookup("file") == nil {
		t.Error("file flag should exist")
	}
	if cmd.Flags().Lookup("data") == nil {
		t.Error("data flag should exist")
	}
	if cmd.Flags().Lookup("field") == nil {
		t.Error("field flag should exist")
	}
}

func TestCRUDCommandBuilder_WithDryRunFlag(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("delete", "delete <id>").
		WithDryRunFlag().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag should exist")
	}
	if dryRunFlag.Value.Type() != "bool" {
		t.Errorf("expected type 'bool', got '%s'", dryRunFlag.Value.Type())
	}
}

func TestCRUDCommandBuilder_WithCascadeFlag(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("delete", "delete <id>").
		WithCascadeFlag().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	cascadeFlag := cmd.Flags().Lookup("cascade")
	if cascadeFlag == nil {
		t.Fatal("cascade flag should exist")
	}
	if cascadeFlag.Value.Type() != "bool" {
		t.Errorf("expected type 'bool', got '%s'", cascadeFlag.Value.Type())
	}
}

func TestCRUDCommandBuilder_WithQueryFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCRUDCommandBuilder("list", "list <kind>").
		WithQueryFlags().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Flags().Lookup("filter") == nil {
		t.Error("filter flag should exist")
	}
	if cmd.Flags().Lookup("sort-by") == nil {
		t.Error("sort-by flag should exist")
	}
	if cmd.Flags().Lookup("sort-asc") == nil {
		t.Error("sort-asc flag should exist")
	}
	if cmd.Flags().Lookup("offset") == nil {
		t.Error("offset flag should exist")
	}
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("limit flag should exist")
	}
}

func TestCRUDCommandBuilder_MethodChaining(t *testing.T) {
	t.Parallel()
	// Test that all methods properly chain and return *CRUDCommandBuilder
	cmd := NewCRUDCommandBuilder("update", "update <id>").
		WithCRUDHelp("Update", "Update description").
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(func(cmd *cobra.Command, args []string) error { return nil }).
		WithCommonFlagsDefault(func(cmd *cobra.Command) {}).
		WithUpdateFlags().
		WithDryRunFlag().
		Build()

	if cmd == nil {
		t.Fatal("cmd should not be nil")
	}
	if cmd.Use != "update <id>" {
		t.Errorf("expected Use='update <id>', got '%s'", cmd.Use)
	}
}

func TestCommandBuilder_FlagTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		addFlag  func(*CommandBuilder) *CommandBuilder
		flagName string
		expected string
	}{
		{
			name: "String flag",
			addFlag: func(b *CommandBuilder) *CommandBuilder {
				return b.AddStringFlag("str", "", "default", "String flag")
			},
			flagName: "str",
			expected: "string",
		},
		{
			name: "Bool flag",
			addFlag: func(b *CommandBuilder) *CommandBuilder {
				return b.AddBoolFlag("bool", "", false, "Bool flag")
			},
			flagName: "bool",
			expected: "bool",
		},
		{
			name: "Int flag",
			addFlag: func(b *CommandBuilder) *CommandBuilder {
				return b.AddIntFlag("int", "", 0, "Int flag")
			},
			flagName: "int",
			expected: "int",
		},
		{
			name: "StringArray flag",
			addFlag: func(b *CommandBuilder) *CommandBuilder {
				return b.AddStringArrayFlag("arr", "", "Array flag")
			},
			flagName: "arr",
			expected: "stringArray",
		},
		{
			name: "StringSlice flag",
			addFlag: func(b *CommandBuilder) *CommandBuilder {
				return b.AddStringSliceFlag("slice", "", "Slice flag")
			},
			flagName: "slice",
			expected: "stringSlice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.addFlag(NewCommandBuilder("test")).Build()
			flag := cmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Flag %s should exist", tt.flagName)
			}
			if flag.Value.Type() != tt.expected {
				t.Errorf("expected type '%s', got '%s'", tt.expected, flag.Value.Type())
			}
		})
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
