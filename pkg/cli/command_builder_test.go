package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandBuilder_Basic(t *testing.T) {
	b := NewCommandBuilder("test-cmd").
		WithShort("short description").
		WithLong("long description").
		AddBoolFlag("verbose", "v", false, "enable verbose output").
		AddIntFlag("count", "c", 1, "iteration count").
		AddStringSliceFlag("tags", "t", "tags list").
		AddStringToStringFlag("filter", "f", "key-value filter").
		AddFloatFlag("rate", "r", 1.5, "rate multiplier")

	cmd := b.Build()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "test-cmd" {
		t.Errorf("expected use 'test-cmd', got '%s'", cmd.Use)
	}
	if cmd.Short != "short description" {
		t.Errorf("expected short 'short description', got '%s'", cmd.Short)
	}
	if cmd.Flags().Lookup("verbose") == nil {
		t.Error("expected verbose flag to exist")
	}
	if cmd.Flags().Lookup("count") == nil {
		t.Error("expected count flag to exist")
	}
	if cmd.Flags().Lookup("tags") == nil {
		t.Error("expected tags flag to exist")
	}
	if cmd.Flags().Lookup("filter") == nil {
		t.Error("expected filter flag to exist")
	}
	if cmd.Flags().Lookup("rate") == nil {
		t.Error("expected rate flag to exist")
	}
}

func TestCommandBuilder_DuplicateFlagSkip(t *testing.T) {
	// Tests the duplicate flag prevention logic
	b := NewCommandBuilder("dup-test").
		WithShort("short").
		WithCommonFlags(true, func(c *cobra.Command) {
			c.Flags().Bool("already-added", false, "first")
		}).
		AddBoolFlag("already-added", "a", true, "second should be skipped")

	cmd := b.Build()
	flag := cmd.Flags().Lookup("already-added")
	if flag == nil {
		t.Fatal("expected flag to exist")
	}
	if flag.Usage != "first" {
		t.Errorf("expected flag to retain first definition, got '%s'", flag.Usage)
	}
}

func TestDefaultCommonExcludedFlags(t *testing.T) {
	defaults := DefaultCommonExcludedFlags()
	if len(defaults) == 0 {
		t.Fatal("expected non-empty default excluded flags")
	}
	without := CommonExcludedFlagsWithout("verbose")
	if len(without) != len(defaults)-1 {
		t.Errorf("expected %d flags, got %d", len(defaults)-1, len(without))
	}
}
