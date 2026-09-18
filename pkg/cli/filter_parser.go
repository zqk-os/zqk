package cli

import (
	"github.com/zqk-os/zqk/pkg/filter"
)

// ParseFilterString is a re-export of filter.ParseFilterString for backward compatibility.
// New callers and non-CLI subsystems should import pkg/filter directly.
func ParseFilterString(filterStr string) (field string, value any, err error) {
	return filter.ParseFilterString(filterStr)
}

// ValidateFilterFields is a re-export of filter.ValidateFilterFields for backward compatibility.
func ValidateFilterFields(kind string, filters map[string]any) error {
	return filter.ValidateFilterFields(kind, filters)
}

// SuggestSimilarFields is a re-export of filter.SuggestSimilarFields for backward compatibility.
func SuggestSimilarFields(input string, validFields []string) []string {
	return filter.SuggestSimilarFields(input, validFields)
}
