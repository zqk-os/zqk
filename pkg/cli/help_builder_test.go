package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHelpBuilder(t *testing.T) {
	t.Parallel()

	t.Run("basic_help_builder", func(t *testing.T) {
		hb := NewHelpBuilder().
			WithShort("A test short description").
			WithDescription("A test full description").
			AddExample("Run a test", "zqk test --flag value").
			AddSection("Notes:", "Important notes here.").
			WithAutoDiscoverFlags(true).
			WithAutoDiscoverSubcommands(true)

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Original short",
		}
		cmd.Flags().String("sample-flag", "", "Sample flag description")

		sub := &cobra.Command{
			Use:   "sub",
			Short: "Subcommand description",
		}
		cmd.AddCommand(sub)

		hb.ApplyToCommand(cmd)

		if cmd.Short != "A test short description" {
			t.Errorf("expected short description on command, got: %s", cmd.Short)
		}
		if !strings.Contains(cmd.Long, "A test full description") {
			t.Errorf("expected description in cmd.Long: %s", cmd.Long)
		}
		if !strings.Contains(cmd.Long, "Available Subcommands:") {
			t.Errorf("expected subcommands in cmd.Long: %s", cmd.Long)
		}
		if !strings.Contains(cmd.Long, "Flags:") {
			t.Errorf("expected flags in cmd.Long: %s", cmd.Long)
		}

		// Test setupDynamicSubcommandHelp placeholder
		hb.setupDynamicSubcommandHelp(sub)
	})

	t.Run("convenience_builders", func(t *testing.T) {
		std := StandardHelpBuilder("short", "line 1", "line 2")
		if std == nil {
			t.Fatal("expected non-nil StandardHelpBuilder")
		}

		dyn := DynamicHelpBuilder("short", "line 1", "line 2")
		if dyn == nil {
			t.Fatal("expected non-nil DynamicHelpBuilder")
		}

		cmd := &cobra.Command{Use: "dummy"}
		dyn.ApplyToCommand(cmd)
		if cmd.Short != "short" {
			t.Errorf("expected cmd.Short to be 'short', got %s", cmd.Short)
		}
		if !strings.Contains(cmd.Long, "line 1") || !strings.Contains(cmd.Long, "line 2") {
			t.Errorf("expected cmd.Long to contain description lines: %s", cmd.Long)
		}

		objB := ObjectCommandHelpBuilder("obj short", "goal", "obj line")
		objCmd := &cobra.Command{Use: "obj"}
		objB.ApplyToCommand(objCmd)
		if objCmd.Short != "obj short" {
			t.Errorf("expected objCmd.Short to be 'obj short', got %s", objCmd.Short)
		}
	})

	t.Run("wrap_text_and_descriptions", func(t *testing.T) {
		hb := NewHelpBuilder().
			WithShort("short").
			AddDescriptionParagraph("Paragraph one.").
			AddDescriptionParagraph("Paragraph two with extra long content that will wrap onto multiple lines in a narrower terminal.")

		cmd := &cobra.Command{Use: "wrap"}
		text := hb.BuildForCommand(cmd)
		if !strings.Contains(text, "Paragraph one.") || !strings.Contains(text, "Paragraph two") {
			t.Errorf("expected paragraphs in text: %s", text)
		}
	})
}
