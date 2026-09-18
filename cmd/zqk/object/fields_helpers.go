package object

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

// EventLogger is an alias for the event logger type to simplify function signatures
type EventLogger = *logging.EventLogger

// FieldsFlags holds parsed flags for the fields command (object package view).
// Populated from clipkg.ParseFieldsFlags for consistent flag handling.
type FieldsFlags struct {
	ListKinds       bool
	Filterable      bool
	Sortable        bool
	Groupable       bool
	SpecializedOnly bool
	Format          string
}

// parseFieldsFlags parses flags via shared parser and returns object's FieldsFlags.
func parseFieldsFlags(cmd *cobra.Command) (FieldsFlags, error) {
	opts, err := clipkg.ParseFieldsFlags(cmd)
	if err != nil {
		return FieldsFlags{}, err
	}
	return FieldsFlags{
		ListKinds:       opts.ListKinds,
		Filterable:      opts.Filterable,
		Sortable:        opts.Sortable,
		Groupable:       opts.Groupable,
		SpecializedOnly: opts.SpecializedOnly,
		Format:          opts.Format,
	}, nil
}

// buildFieldMap builds a map of field names to FieldInfo for quick lookup
func buildFieldMap(kindFields *objects.KindFields) map[string]objects.FieldInfo {
	fieldMap := make(map[string]objects.FieldInfo)
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		fieldMap[field.Name] = *field
	}
	return fieldMap
}

// getFieldsByTrait gets fields filtered by a specific trait
func getFieldsByTrait(kind string, logger EventLogger, kindFields *objects.KindFields, trait string, generator func(string) ([]string, error)) ([]objects.FieldInfo, error) {
	fieldNames, err := generator(kind)
	if err != nil {
		logging.FluentEvent(logger).Error("Failed to get fields by trait", err).
			String("kind", kind).
			String("trait", trait).
			Log()
		return nil, err
	}

	fieldMap := buildFieldMap(kindFields)
	var fieldsToShow []objects.FieldInfo
	for _, fieldName := range fieldNames {
		if field, ok := fieldMap[fieldName]; ok {
			fieldsToShow = append(fieldsToShow, field)
		}
	}
	return fieldsToShow, nil
}

// getFilterableFields gets filterable fields for a kind
func getFilterableFields(kind string, logger EventLogger, kindFields *objects.KindFields) ([]objects.FieldInfo, error) {
	return getFieldsByTrait(kind, logger, kindFields, "filterable", objects.GenerateFilterableFields)
}

// getSortableFields gets sortable fields for a kind
func getSortableFields(kind string, logger EventLogger, kindFields *objects.KindFields) ([]objects.FieldInfo, error) {
	return getFieldsByTrait(kind, logger, kindFields, "sortable", objects.GenerateSortableFields)
}

// getGroupableFields gets groupable fields for a kind
func getGroupableFields(kind string, logger EventLogger, kindFields *objects.KindFields) ([]objects.FieldInfo, error) {
	return getFieldsByTrait(kind, logger, kindFields, "groupable", objects.GenerateGroupableFields)
}

// getFieldsToShow determines which fields to show based on flags
func getFieldsToShow(flags FieldsFlags, kind string, logger EventLogger, kindFields *objects.KindFields) ([]objects.FieldInfo, error) {
	if flags.Filterable {
		return getFilterableFields(kind, logger, kindFields)
	}
	if flags.Sortable {
		return getSortableFields(kind, logger, kindFields)
	}
	if flags.Groupable {
		return getGroupableFields(kind, logger, kindFields)
	}
	if flags.SpecializedOnly {
		return kindFields.SpecializedFields, nil
	}
	return nil, nil // Signal to show all fields grouped
}

// outputFieldsByFormat outputs fields in the requested format
func outputFieldsByFormat(cmd *cobra.Command, format string, kindFields *objects.KindFields, fieldsToShow []objects.FieldInfo) error {
	if kindFields != nil {
		// Show all fields grouped
		switch format {
		case objectFormatJSON:
			return outputFieldsJSON(cmd, kindFields)
		case objectFormatYAML:
			return outputFieldsYAML(cmd, kindFields)
		default:
			return outputFieldsTable(cmd, kindFields)
		}
	}

	// Show filtered fields
	switch format {
	case objectFormatJSON, objectFormatYAML:
		data := map[string]any{
			"fields": convertFieldsToMaps(fieldsToShow),
		}
		return cli.FormatOutput(cmd, data)
	default:
		return outputFieldsListTable(cmd, fieldsToShow)
	}
}

// outputFieldsJSON outputs fields in JSON format (common then specialized, shared layout).
func outputFieldsJSON(cmd *cobra.Command, kindFields *objects.KindFields) error {
	return cli.FormatOutputAs(cmd, cli.FormatJSON, clipkg.KindFieldsSegregatedData(kindFields))
}

// outputFieldsYAML outputs fields in YAML format (common then specialized, shared layout).
func outputFieldsYAML(cmd *cobra.Command, kindFields *objects.KindFields) error {
	return cli.FormatOutputAs(cmd, cli.FormatYAML, clipkg.KindFieldsSegregatedData(kindFields))
}

// outputFieldsTable outputs fields in table format (common then specialized, shared layout).
func outputFieldsTable(cmd *cobra.Command, kindFields *objects.KindFields) error {
	return cli.WriteOutput(cmd, clipkg.FormatKindFieldsSegregatedTable(kindFields))
}

// outputFieldsListTable outputs a list of fields in table format
func outputFieldsListTable(cmd *cobra.Command, fields []objects.FieldInfo) error {
	var buf bytes.Buffer
	buf.WriteString("Fields:\n")
	buf.WriteString(strings.Repeat("-", 60) + "\n")

	for i := range fields {
		field := &fields[i]
		fmt.Fprintf(&buf, "  %s", field.Name)
		if field.Type != emptyValue {
			fmt.Fprintf(&buf, " (%s)", field.Type)
		}
		if len(field.Traits) > 0 {
			fmt.Fprintf(&buf, " [%s]", strings.Join(field.Traits, ", "))
		}
		if field.Required {
			buf.WriteString(" (required)")
		}
		if field.Description != emptyValue {
			fmt.Fprintf(&buf, "\n    %s", field.Description)
		}
		if len(field.EnumValues) > 0 {
			fmt.Fprintf(&buf, "\n    Valid values: %s", strings.Join(field.EnumValues, ", "))
		}
		buf.WriteString("\n")
	}

	return cli.WriteOutput(cmd, buf.Bytes())
}

// convertFieldsToMaps converts FieldInfo slices to maps for JSON/YAML output
func convertFieldsToMaps(fields []objects.FieldInfo) []map[string]any {
	result := make([]map[string]any, len(fields))
	for i := range fields {
		field := &fields[i]
		m := map[string]any{
			objects.FieldKeyName: field.Name,
		}
		if field.Type != emptyValue {
			m[objects.FieldKeyType] = field.Type
		}
		if len(field.Traits) > 0 {
			m["traits"] = field.Traits
		}
		if field.Required {
			m["required"] = true
		}
		if field.Description != emptyValue {
			m[objects.FieldKeyDescription] = field.Description
		}
		if len(field.EnumValues) > 0 {
			m["enum_values"] = field.EnumValues
		}
		result[i] = m
	}
	return result
}
