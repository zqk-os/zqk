// Package cli fields_output provides shared formatting for "fields" command output
// with clear segregation: common fields first, then specialized fields. Same look
// and structure for object and internal (and any kind-specific fields command).

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// Standard section layout for fields output (common look and feel).
const (
	FieldsSectionWidth       = 60
	CommonFieldsHeader       = "Common Fields (inherited by all objects):"
	SpecializedFieldsHeader  = "Specialized Fields (specific to this kind):"
	FieldsTableTitleTemplate = "Fields for '%s':"
)

// fieldInfoToMaps converts FieldInfo slice to []map[string]any for JSON/YAML.
func fieldInfoToMaps(fields []objects.FieldInfo) []map[string]any {
	out := make([]map[string]any, len(fields))
	for i := range fields {
		f := &fields[i]
		m := map[string]any{objects.FieldKeyName: f.Name}
		if f.Type != emptyValue {
			m[objects.FieldKeyType] = f.Type
		}
		if len(f.Traits) > 0 {
			m["traits"] = f.Traits
		}
		if f.Required {
			m["required"] = true
		}
		if f.Description != emptyValue {
			m[objects.FieldKeyDescription] = f.Description
		}
		if len(f.EnumValues) > 0 {
			m["enum_values"] = f.EnumValues
		}
		out[i] = m
	}
	return out
}

// FormatKindFieldsSegregatedTable returns table-formatted bytes: common section then
// specialized section, with standard headers and separator length.
func FormatKindFieldsSegregatedTable(kindFields *objects.KindFields) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, FieldsTableTitleTemplate+"\n\n", kindFields.Kind)
	sep := strings.Repeat("-", FieldsSectionWidth)

	if len(kindFields.CommonFields) > 0 {
		buf.WriteString(CommonFieldsHeader + "\n")
		buf.WriteString(sep + "\n")
		writeFieldLines(&buf, kindFields.CommonFields)
		buf.WriteString("\n")
	}
	if len(kindFields.SpecializedFields) > 0 {
		buf.WriteString(SpecializedFieldsHeader + "\n")
		buf.WriteString(sep + "\n")
		writeFieldLines(&buf, kindFields.SpecializedFields)
	}
	return buf.Bytes()
}

func writeFieldLines(buf *bytes.Buffer, fields []objects.FieldInfo) {
	for i := range fields {
		f := &fields[i]
		fmt.Fprintf(buf, "  %s", f.Name)
		if f.Type != emptyValue {
			fmt.Fprintf(buf, " (%s)", f.Type)
		}
		if len(f.Traits) > 0 {
			fmt.Fprintf(buf, " [%s]", strings.Join(f.Traits, ", "))
		}
		if f.Required {
			buf.WriteString(" (required)")
		}
		if f.Description != emptyValue {
			fmt.Fprintf(buf, "\n    %s", f.Description)
		}
		if len(f.EnumValues) > 0 {
			fmt.Fprintf(buf, "\n    Valid values: %s", strings.Join(f.EnumValues, ", "))
		}
		buf.WriteString("\n")
	}
}

// KindFieldsSegregatedData returns a map suitable for JSON or YAML output with
// kind, common_fields, and specialized_fields. Caller marshals and writes.
func KindFieldsSegregatedData(kindFields *objects.KindFields) map[string]any {
	data := map[string]any{objects.FieldKeyKind: kindFields.Kind}
	if len(kindFields.CommonFields) > 0 {
		data["common_fields"] = fieldInfoToMaps(kindFields.CommonFields)
	}
	if len(kindFields.SpecializedFields) > 0 {
		data["specialized_fields"] = fieldInfoToMaps(kindFields.SpecializedFields)
	}
	return data
}

// FormatKindFieldsSegregatedJSON returns JSON bytes for segregated common/specialized output.
func FormatKindFieldsSegregatedJSON(kindFields *objects.KindFields) ([]byte, error) {
	data := KindFieldsSegregatedData(kindFields)
	return json.MarshalIndent(data, "", "  ")
}

// FormatKindFieldsSegregatedYAML returns YAML bytes for segregated common/specialized output.
func FormatKindFieldsSegregatedYAML(kindFields *objects.KindFields) ([]byte, error) {
	data := KindFieldsSegregatedData(kindFields)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
