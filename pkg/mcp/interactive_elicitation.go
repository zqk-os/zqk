// Conversion from pkg/interactive types to MCP elicitation params only (not a tool bridge; see ARCHITECTURE_PATTERNS.md).
package mcp

import (
	"github.com/lanceman/zqk/pkg/interactive"
)

// ConvertFieldTokenInfoToElicitationParam converts a FieldTokenInfo to an MCP ElicitationParam
func ConvertFieldTokenInfoToElicitationParam(tokenInfo *interactive.FieldTokenInfo) ElicitationParam {
	param := ElicitParam(
		tokenInfo.Name,
		tokenInfo.Description,
		mapFieldTypeToElicitationType(tokenInfo.Type),
		tokenInfo.Required,
	)

	// Add enum choices if available
	if len(tokenInfo.EnumValues) > 0 {
		choices := make([]any, len(tokenInfo.EnumValues))
		for i, val := range tokenInfo.EnumValues {
			choices[i] = val
		}
		param.Choices = choices
	}

	return param
}

// ConvertFieldTokenInfosToElicitationParams converts a slice of FieldTokenInfo to ElicitationParams
func ConvertFieldTokenInfosToElicitationParams(tokenInfos []*interactive.FieldTokenInfo) []ElicitationParam {
	params := make([]ElicitationParam, 0, len(tokenInfos))
	for _, tokenInfo := range tokenInfos {
		params = append(params, ConvertFieldTokenInfoToElicitationParam(tokenInfo))
	}
	return params
}

// mapFieldTypeToElicitationType maps field types to MCP elicitation parameter types
// mapFieldTypeToElicitationType is a convenience wrapper around MapFieldTypeToElicitationType.
// Kept for backward compatibility with existing code.
func mapFieldTypeToElicitationType(fieldType string) string {
	return MapFieldTypeToElicitationType(fieldType)
}
