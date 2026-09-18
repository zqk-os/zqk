package cli

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// FieldCompletionCandidate represents a single completion suggestion for a field,
// including a concise, human-readable constraint summary (for display alongside
// the flag/field name in shell completion).
type FieldCompletionCandidate struct {
	Name        string // field name
	Description string // constraint/trait summary (enum values, ranges, formats, etc.)
}

// BuildFieldCompletionCandidates builds completion candidates for a given kind and
// filter function over SpecFieldSummary (e.g. groupable/filterable/sortable). The
// index is expected to be preloaded (e.g. via TryLoadSpecIndexForProjectRoot).
func BuildFieldCompletionCandidates(
	index *objects.SpecIndex,
	kind string,
	filter func(objects.SpecFieldSummary) bool,
) []FieldCompletionCandidate {
	if index == nil || kind == emptyValue || filter == nil {
		return nil
	}

	ks, ok := index.GetKindSummary(kind)
	if !ok {
		return nil
	}

	candidates := make([]FieldCompletionCandidate, 0, len(ks.Fields))
	for _, f := range ks.Fields {
		if !filter(f) {
			continue
		}
		candidates = append(candidates, FieldCompletionCandidate{
			Name:        f.Name,
			Description: formatFieldSummary(f),
		})
	}
	return candidates
}

// BuildEnumValueCompletions builds completion values for a specific enum-like field
// on a kind, filtering by a value prefix. It returns the raw enum values; callers
// are responsible for adding any field/operator prefix (e.g. "status=").
func BuildEnumValueCompletions(
	index *objects.SpecIndex,
	kind string,
	fieldName string,
	valuePrefix string,
) []string {
	if index == nil || kind == emptyValue || fieldName == emptyValue {
		return nil
	}

	ks, ok := index.GetKindSummary(kind)
	if !ok {
		return nil
	}

	for _, f := range ks.Fields {
		if f.Name != fieldName || len(f.EnumValues) == 0 {
			continue
		}
		out := make([]string, 0, len(f.EnumValues))
		for _, v := range f.EnumValues {
			if valuePrefix == emptyValue || strings.HasPrefix(v, valuePrefix) {
				out = append(out, v)
			}
		}
		return out
	}

	return nil
}

// formatFieldSummary renders a concise, single-line description for a field, using
// trait and validation hints from SpecFieldSummary. This is intentionally compact;
// callers can include it as the "description" alongside the field name in completion.
func formatFieldSummary(f objects.SpecFieldSummary) string {
	parts := make([]string, 0, 4)

	// Type / semantic kind.
	if f.SemanticType != emptyValue {
		parts = append(parts, f.SemanticType)
	} else if f.Type != emptyValue {
		parts = append(parts, f.Type)
	}

	// Enum values: show a short sample.
	if len(f.EnumValues) > 0 {
		max := 3
		if len(f.EnumValues) < max {
			max = len(f.EnumValues)
		}
		preview := f.EnumValues[:max]
		if len(f.EnumValues) > max {
			parts = append(parts, fmt.Sprintf("enum: %v, ...", preview))
		} else {
			parts = append(parts, fmt.Sprintf("enum: %v", preview))
		}
	}

	// Numeric ranges.
	if f.Numeric && (f.MinValue != 0 || f.MaxValue != 0) {
		switch {
		case f.MinValue != 0 && f.MaxValue != 0:
			parts = append(parts, fmt.Sprintf("%g–%g", f.MinValue, f.MaxValue))
		case f.MinValue != 0:
			parts = append(parts, fmt.Sprintf(">= %g", f.MinValue))
		case f.MaxValue != 0:
			parts = append(parts, fmt.Sprintf("<= %g", f.MaxValue))
		}
	}

	// Length hints.
	if f.MaxLength > 0 {
		if f.MinLength > 0 {
			parts = append(parts, fmt.Sprintf("len %d–%d", f.MinLength, f.MaxLength))
		} else {
			parts = append(parts, fmt.Sprintf("max %d chars", f.MaxLength))
		}
	} else if f.MinLength > 0 {
		parts = append(parts, fmt.Sprintf("min %d chars", f.MinLength))
	}
	if f.DisplayLength > 0 {
		parts = append(parts, fmt.Sprintf("display width %d", f.DisplayLength))
	}

	// Time/date formats.
	if f.Format != emptyValue && (f.TimeLike || f.DateLike || f.SemanticType == "timestamp" || f.SemanticType == "date") {
		parts = append(parts, f.Format)
	}

	// Fallback: join traits if nothing else was added.
	if len(parts) == 0 && len(f.Traits) > 0 {
		parts = append(parts, fmt.Sprintf("traits: %v", f.Traits))
	}

	return strings.Join(parts, ", ")
}
