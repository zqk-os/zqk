package objects

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FormatMultiLineYAML ensures multi-line strings are properly formatted in YAML
// It uses yaml.Node to set the literal block scalar style (|) for strings containing newlines
// This prevents malformed YAML when strings contain newlines or special characters.
// TRACK: [REDACTED-ID] — wire this into yamlMarshalForPersistence / CAS writes
// (today many object paths still call yaml.Marshal and emit double-quoted \n bodies).
func FormatMultiLineYAML(data map[string]any) ([]byte, error) {
	// Convert map to yaml.Node tree
	node := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{},
	}

	// Recursively process the map
	for key, value := range data {
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: key,
		}
		node.Content = append(node.Content, keyNode)

		valueNode := formatValueNode(value)
		node.Content = append(node.Content, valueNode)
	}

	// Marshal the node tree to YAML
	return yaml.Marshal(node)
}

// formatValueNode recursively formats values, setting literal block scalar style for multi-line strings
func formatValueNode(value any) *yaml.Node {
	switch v := value.(type) {
	case string:
		node := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
		}
		// Use literal block scalar (|) for strings containing newlines
		// This ensures proper YAML formatting for multi-line strings
		if containsNewline(v) {
			node.Style = yaml.LiteralStyle
		}
		return node
	case map[string]any:
		node := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{},
		}
		for key, val := range v {
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: key,
			}
			node.Content = append(node.Content, keyNode)
			valueNode := formatValueNode(val)
			node.Content = append(node.Content, valueNode)
		}
		return node
	case []any:
		node := &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: []*yaml.Node{},
		}
		for _, item := range v {
			itemNode := formatValueNode(item)
			node.Content = append(node.Content, itemNode)
		}
		return node
	case []string:
		node := &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: []*yaml.Node{},
		}
		for _, item := range v {
			itemNode := formatValueNode(item)
			node.Content = append(node.Content, itemNode)
		}
		return node
	default:
		// For other types, use standard marshaling
		// This will handle numbers, booleans, etc.
		var node yaml.Node
		if err := node.Encode(v); err != nil {
			// Fallback to string representation
			return &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: fmt.Sprintf("%v", v),
			}
		}
		return &node
	}
}

// containsNewline checks if a string contains newline characters
func containsNewline(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}
