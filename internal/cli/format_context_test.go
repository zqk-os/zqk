package cli

import (
	"testing"

	"github.com/spf13/cobra"

	pkgcli "github.com/lanceman/zqk/pkg/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestFormatAndContextSwitching tests format routing through GetFormat with different context profiles
func TestFormatAndContextSwitching(t *testing.T) {
	tests := []struct {
		name           string
		contextProfile string
		formatFlag     string
		expectedFormat OutputFormat
		description    string
	}{
		{
			name:           "ai-agent profile sets jsonl format",
			contextProfile: "ai-agent",
			formatFlag:     "",
			expectedFormat: FormatJSONL,
			description:    "When --context ai-agent is used, format should be jsonl",
		},
		{
			name:           "human profile sets table format",
			contextProfile: "human",
			formatFlag:     "",
			expectedFormat: FormatTable,
			description:    "When --context human is used, format should be table",
		},
		{
			name:           "debug profile sets yaml format",
			contextProfile: "debug",
			formatFlag:     "",
			expectedFormat: FormatYAML,
			description:    "When --context debug is used, format should be yaml",
		},
		{
			name:           "mcp profile sets json format",
			contextProfile: "mcp",
			formatFlag:     "",
			expectedFormat: FormatJSON,
			description:    "When --context mcp is used, format should be json",
		},
		{
			name:           "format flag overrides ai-agent profile",
			contextProfile: "ai-agent",
			formatFlag:     "yaml",
			expectedFormat: FormatYAML,
			description:    "When --format yaml is set with --context ai-agent, format flag should override",
		},
		{
			name:           "format flag overrides human profile",
			contextProfile: "human",
			formatFlag:     "json",
			expectedFormat: FormatJSON,
			description:    "When --format json is set with --context human, format flag should override",
		},
		{
			name:           "no context defaults to table",
			contextProfile: "",
			formatFlag:     "",
			expectedFormat: FormatTable,
			description:    "When no context is set, format should default to table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command
			cmd := pkgcli.NewCommandBuilder("test").Build()
			AddCommonFlags(cmd)
			cmd.Flags().String("context", "", "Context profile")

			// Set context flag if provided
			if tt.contextProfile != emptyValue {
				if err := cmd.Flags().Set("context", tt.contextProfile); err != nil {
					t.Fatalf("Failed to set context flag: %v", err)
				}
			}

			// Set format flag if provided
			if tt.formatFlag != emptyValue {
				if err := cmd.Flags().Set("format", tt.formatFlag); err != nil {
					t.Fatalf("Failed to set format flag: %v", err)
				}
			}

			// Get context from command (this applies the profile)
			initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
			cliCtx, err := GetContextFromCommand(cmd, initCtx)
			if err != nil {
				t.Fatalf("Failed to get context: %v", err)
			}

			// Verify inner context format string (profile-driven defaults live on embedded Context)
			if tt.contextProfile != emptyValue && tt.formatFlag == emptyValue {
				innerFmt := cliCtx.Context.Format
				expectedCtxFormat := string(tt.expectedFormat)
				if tt.contextProfile == "ai-agent" {
					if innerFmt != "jsonl" && innerFmt != "json" {
						t.Errorf("Expected embedded context.Format to be jsonl or json for ai-agent, got %q", innerFmt)
					}
				} else {
					if innerFmt != expectedCtxFormat {
						t.Errorf("Expected embedded context.Format to be %q, got %q", expectedCtxFormat, innerFmt)
					}
				}
			}

			// Register context in registry (simulating PersistentPreRunE)
			SetContext(cmd, cliCtx)

			// Test GetFormat (this is what commands actually use)
			format := GetFormat(cmd)

			// Verify format routing
			// For ai-agent, accept either jsonl or json (profile spec may vary, but GetFormat should return one of them)
			if tt.contextProfile == "ai-agent" && tt.formatFlag == emptyValue {
				if format != FormatJSONL && format != FormatJSON {
					t.Errorf("%s: Expected format jsonl or json for ai-agent, got %q", tt.description, format)
				}
			} else {
				if format != tt.expectedFormat {
					t.Errorf("%s: Expected format %q, got %q", tt.description, tt.expectedFormat, format)
				}
			}
		})
	}
}

// TestFormatRoutingPrecedence tests that format precedence is correct
func TestFormatRoutingPrecedence(t *testing.T) {
	tests := []struct {
		name           string
		contextProfile string
		formatFlag     string
		expectedFormat OutputFormat
		description    string
	}{
		{
			name:           "format flag has highest precedence",
			contextProfile: "ai-agent",
			formatFlag:     "table",
			expectedFormat: FormatTable,
			description:    "Format flag should override profile format",
		},
		{
			name:           "profile format used when no format flag",
			contextProfile: "ai-agent",
			formatFlag:     "",
			expectedFormat: FormatJSONL,
			description:    "Profile format should be used when format flag is not set",
		},
		{
			name:           "default table when no profile and no flag",
			contextProfile: "",
			formatFlag:     "",
			expectedFormat: FormatTable,
			description:    "Should default to table when no profile or flag is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command
			cmd := pkgcli.NewCommandBuilder("test").Build()
			AddCommonFlags(cmd)
			cmd.Flags().String("context", "", "Context profile")

			// Set context flag if provided
			if tt.contextProfile != emptyValue {
				if err := cmd.Flags().Set("context", tt.contextProfile); err != nil {
					t.Fatalf("Failed to set context flag: %v", err)
				}
			}

			// Set format flag if provided
			if tt.formatFlag != emptyValue {
				if err := cmd.Flags().Set("format", tt.formatFlag); err != nil {
					t.Fatalf("Failed to set format flag: %v", err)
				}
			}

			// Get context from command
			initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
			cliCtx, err := GetContextFromCommand(cmd, initCtx)
			if err != nil {
				t.Fatalf("Failed to get context: %v", err)
			}

			// Register context in registry
			SetContext(cmd, cliCtx)

			// Test GetFormat
			format := GetFormat(cmd)

			if format != tt.expectedFormat {
				t.Errorf("%s: Expected format %q, got %q", tt.description, tt.expectedFormat, format)
			}
		})
	}
}

// TestAllProfileFormats tests that all profiles set their expected formats
func TestAllProfileFormats(t *testing.T) {
	profileFormats := map[string]OutputFormat{
		"ai-agent": FormatJSONL,
		"human":    FormatTable,
		"debug":    FormatYAML,
		"mcp":      FormatJSON,
	}

	for profile, expectedFormat := range profileFormats {
		t.Run(profile, func(t *testing.T) {
			// Create a test command
			cmd := pkgcli.NewCommandBuilder("test").Build()
			AddCommonFlags(cmd)
			// Add context flag (GetContextFromCommand checks cmd.Flags() which works for both local and persistent)
			cmd.Flags().String("context", "", "Context profile")

			// Set context flag
			if err := cmd.Flags().Set("context", profile); err != nil {
				t.Fatalf("Failed to set context flag: %v", err)
			}

			// Get context from command
			initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
			cliCtx, err := GetContextFromCommand(cmd, initCtx)
			if err != nil {
				t.Fatalf("Failed to get context: %v", err)
			}

			// Verify profile is set
			if cliCtx.Profile != profile {
				t.Errorf("Expected context.Profile to be %q, got %q", profile, cliCtx.Profile)
			}

			// Verify format is set correctly (embedded string + wrapper OutputFormat)
			expectedFormatStr := string(expectedFormat)
			if cliCtx.Context.Format != expectedFormatStr {
				t.Errorf("Expected embedded context.Format to be %q, got %q", expectedFormatStr, cliCtx.Context.Format)
			}
			if cliCtx.Format != expectedFormat {
				t.Errorf("Expected wrapper Format to be %q, got %q", expectedFormat, cliCtx.Format)
			}

			// Register context in registry
			SetContext(cmd, cliCtx)

			// Test GetFormat routing
			format := GetFormat(cmd)
			if format != expectedFormat {
				t.Errorf("Expected GetFormat to return %q, got %q", expectedFormat, format)
			}
		})
	}
}

// TestGetFormatFromNestedCommand verifies that --format on root (persistent) is found when
// GetFormat is called from a nested subcommand (e.g. object get), matching real CLI usage.
func TestGetFormatFromNestedCommand(t *testing.T) {
	root := pkgcli.NewCommandBuilder("zqk").Build()
	root.PersistentFlags().StringP("format", "f", "", "Output format")

	obj := pkgcli.NewCommandBuilder("object").Build()
	root.AddCommand(obj)

	get := pkgcli.NewCommandBuilder("get").
		WithRunE(func(cmd *cobra.Command, args []string) error {
			format := GetFormat(cmd)
			if format != FormatYAML {
				t.Errorf("GetFormat from nested command: expected yaml, got %q", format)
			}
			return nil
		}).
		Build()
	AddCommonFlags(get) // child has local format too (like object get)
	obj.AddCommand(get)

	root.SetArgs([]string{"object", "get", "BLI-765", "--format", "yaml"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}
