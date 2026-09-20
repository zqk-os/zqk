// Package cli fields_flags provides a single place for "fields" command flags and parsing
// so object and internal (and any trait-harness-style fields command) use the same
// flag names, order, and types. Use in object-specific or kind-specific context for
// common look and feel and consistent placement.

package cli

import (
	"github.com/spf13/cobra"
)

const (
	fieldsFlagListKinds       = "list-kinds"
	fieldsFlagFilterable      = "filterable"
	fieldsFlagSortable        = "sortable"
	fieldsFlagGroupable       = "groupable"
	fieldsFlagSpecializedOnly = "specialized-only"
	fieldsFlagFormat          = "format"

	fieldsFlagHelpListKinds       = "List all object kinds (instead of listing fields for a kind)"
	fieldsFlagHelpFilterable      = "Show only filterable fields"
	fieldsFlagHelpSortable        = "Show only sortable fields"
	fieldsFlagHelpGroupable       = "Show only groupable fields"
	fieldsFlagHelpSpecializedOnly = "Show only specialized fields (exclude common fields)"
	fieldsFlagHelpFormat          = "Output format: table, json, yaml"
	fieldsDefaultFormat           = "table"
)

// FieldsOptions holds parsed flags for a fields command (list-kinds, filterable,
// sortable, groupable, specialized-only, format). Used by object fields and internal fields.
type FieldsOptions struct {
	ListKinds       bool   // List all kinds (root-only; when true, kind is not required)
	Filterable      bool   // Show only filterable fields
	Sortable        bool   // Show only sortable fields
	Groupable       bool   // Show only groupable fields
	SpecializedOnly bool   // Show only specialized fields (exclude common)
	Format          string // Output format: table, json, yaml
}

// AddFieldsFlags adds the standard fields-command flags to cmd in a fixed order:
// (1) list-kinds if includeListKinds, (2) filterable, (3) sortable, (4) groupable,
// (5) specialized-only, (6) format. Same order everywhere for consistent placement.
// FieldsHarnessFlagNames returns flag names registered by AddFieldsFlags(includeListKinds).
func FieldsHarnessFlagNames(includeListKinds bool) []string {
	names := make([]string, 0, 6)
	if includeListKinds {
		names = append(names, fieldsFlagListKinds)
	}
	names = append(names,
		fieldsFlagFilterable,
		fieldsFlagSortable,
		fieldsFlagGroupable,
		fieldsFlagSpecializedOnly,
		fieldsFlagFormat,
	)
	return names
}

func AddFieldsFlags(cmd *cobra.Command, includeListKinds bool) {
	if includeListKinds {
		cmd.Flags().Bool(fieldsFlagListKinds, false, fieldsFlagHelpListKinds)
	}
	cmd.Flags().Bool(fieldsFlagFilterable, false, fieldsFlagHelpFilterable)
	cmd.Flags().Bool(fieldsFlagSortable, false, fieldsFlagHelpSortable)
	cmd.Flags().Bool(fieldsFlagGroupable, false, fieldsFlagHelpGroupable)
	cmd.Flags().Bool(fieldsFlagSpecializedOnly, false, fieldsFlagHelpSpecializedOnly)
	cmd.Flags().String(fieldsFlagFormat, fieldsDefaultFormat, fieldsFlagHelpFormat)
}

// ParseFieldsFlags reads fields-related flags from cmd and returns typed FieldsOptions.
// Use for any command that uses AddFieldsFlags. Skips flags not present on cmd (zero value).
// Returns an error when a registered flag cannot be read (does not swallow Get* errors).
func ParseFieldsFlags(cmd *cobra.Command) (FieldsOptions, error) {
	opts := FieldsOptions{}
	var bag FlagBag
	if cmd.Flags().Lookup(fieldsFlagListKinds) != nil {
		opts.ListKinds = bag.Bool(cmd, fieldsFlagListKinds)
	}
	opts.Filterable = bag.Bool(cmd, fieldsFlagFilterable)
	opts.Sortable = bag.Bool(cmd, fieldsFlagSortable)
	opts.Groupable = bag.Bool(cmd, fieldsFlagGroupable)
	opts.SpecializedOnly = bag.Bool(cmd, fieldsFlagSpecializedOnly)
	opts.Format = bag.String(cmd, fieldsFlagFormat)
	if err := bag.Err(); err != nil {
		return FieldsOptions{}, err
	}
	if opts.Format == emptyValue {
		opts.Format = fieldsDefaultFormat
	}
	return opts, nil
}
