package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAddFieldsFlags_WithListKinds(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	AddFieldsFlags(cmd, true)
	for _, name := range []string{"list-kinds", "filterable", "sortable", "groupable", "specialized-only", "format"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not added", name)
		}
	}
}

func TestAddFieldsFlags_WithoutListKinds(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	AddFieldsFlags(cmd, false)
	if cmd.Flags().Lookup("list-kinds") != nil {
		t.Error("list-kinds should not be added when includeListKinds is false")
	}
	for _, name := range []string{"filterable", "sortable", "groupable", "specialized-only", "format"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not added", name)
		}
	}
}

func TestParseFieldsFlags(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	AddFieldsFlags(cmd, true)
	_ = cmd.Flags().Set("filterable", "true")
	_ = cmd.Flags().Set("format", "json")

	opts, err := ParseFieldsFlags(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Filterable {
		t.Error("expected Filterable true")
	}
	if opts.Format != "json" {
		t.Errorf("expected format json, got %q", opts.Format)
	}
	if opts.Sortable || opts.Groupable || opts.SpecializedOnly {
		t.Error("expected other bools false when not set")
	}
}

func TestParseFieldsFlags_DefaultFormat(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	AddFieldsFlags(cmd, false)
	opts, err := ParseFieldsFlags(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Format != "table" {
		t.Errorf("expected default format table, got %q", opts.Format)
	}
}
