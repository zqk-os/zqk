// Package objects provides extraction of properties from YAML/JSON object files
// using a zero-allocation byte prefix scan with a lightweight yaml.Node AST fallback,
// avoiding expensive map[string]any heap unmarshaling.

package objects

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultYAMLInspectReadLimit = 4096
)

// ExtractProperty extracts a single top-level scalar property from YAML/JSON object bytes.
// It prioritizes a zero-allocation byte prefix scan, falling back to a lightweight yaml.Node
// traversal to avoid expensive map[string]any heap unmarshaling and reflection.
func ExtractProperty(data []byte, property string) string {
	if val, ok := scanForCandidate(data, property); ok {
		return val
	}
	res := extractWithYAMLNode(data, []string{property})
	return res[property]
}

// ExtractProperties extracts multiple top-level scalar properties from YAML/JSON object bytes.
// Returns a map of property name to extracted string value. Missing properties are set to "".
func ExtractProperties(data []byte, properties ...string) map[string]string {
	result := make(map[string]string, len(properties))
	if len(properties) == 0 {
		return result
	}

	var missing []string
	for _, prop := range properties {
		if val, ok := scanForCandidate(data, prop); ok {
			result[prop] = val
		} else {
			missing = append(missing, prop)
		}
	}

	if len(missing) > 0 {
		nodeResults := extractWithYAMLNode(data, missing)
		for _, prop := range missing {
			result[prop] = nodeResults[prop]
		}
	}

	return result
}

// ExtractPropertyFromFile reads a bounded prefix of the YAML/JSON file and extracts a top-level property.
// If the property is not found in the initial read window, it reads the full file.
func ExtractPropertyFromFile(path string, property string) string {
	res := ExtractPropertiesFromFile(path, property)
	return res[property]
}

// ExtractPropertiesFromFile reads a bounded prefix of the YAML/JSON file and extracts multiple top-level properties.
// If any property is not found in the initial read window, it falls back to reading the full file.
func ExtractPropertiesFromFile(path string, properties ...string) map[string]string {
	result := make(map[string]string, len(properties))
	if len(properties) == 0 {
		return result
	}

	f, err := fileutil.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()

	buf := make([]byte, defaultYAMLInspectReadLimit)
	n, readErr := f.Read(buf)
	if n <= 0 {
		return result
	}
	limitData := buf[:n]

	var missing []string
	for _, prop := range properties {
		if val, ok := scanForCandidate(limitData, prop); ok && val != "" {
			result[prop] = val
		} else {
			missing = append(missing, prop)
		}
	}

	if len(missing) == 0 {
		return result
	}

	// Read remainder of file if EOF was not reached
	var fullData []byte
	if readErr == nil && n == defaultYAMLInspectReadLimit {
		rest, err := io.ReadAll(f)
		if err == nil && len(rest) > 0 {
			fullData = make([]byte, n+len(rest))
			copy(fullData, limitData)
			copy(fullData[n:], rest)
		} else {
			fullData = limitData
		}
	} else {
		fullData = limitData
	}

	var stillMissing []string
	for _, prop := range missing {
		if val, ok := scanForCandidate(fullData, prop); ok {
			result[prop] = val
		} else {
			stillMissing = append(stillMissing, prop)
		}
	}

	if len(stillMissing) > 0 {
		nodeResults := extractWithYAMLNode(fullData, stillMissing)
		for _, prop := range stillMissing {
			result[prop] = nodeResults[prop]
		}
	}

	return result
}

func scanForCandidate(data []byte, prop string) (string, bool) {
	if len(data) == 0 || len(prop) == 0 {
		return "", false
	}
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}

	rawPatterns := []string{
		prop + ":",
		prop + " :",
		`"` + prop + `":`,
		`"` + prop + `" :`,
		`'` + prop + `':`,
		`'` + prop + `' :`,
	}

	// 1. Check if first line matches (offset 0)
	for _, raw := range rawPatterns {
		pat := []byte(raw)
		if bytes.HasPrefix(data, pat) {
			if val, ok := parseLineValue(data[len(pat):]); ok {
				return val, true
			}
		}
	}

	// 2. Check subsequent lines (prefixed by \n)
	for _, raw := range rawPatterns {
		nlPat := append([]byte("\n"), raw...)
		offset := 0
		for {
			idx := bytes.Index(data[offset:], nlPat)
			if idx == -1 {
				break
			}
			matchPos := offset + idx + len(nlPat)
			if val, ok := parseLineValue(data[matchPos:]); ok {
				return val, true
			}
			offset += idx + len(nlPat)
		}
	}

	return "", false
}

func parseLineValue(afterColon []byte) (string, bool) {
	i := 0
	for i < len(afterColon) && (afterColon[i] == ' ' || afterColon[i] == '\t') {
		i++
	}
	if i >= len(afterColon) {
		return "", true
	}

	if afterColon[i] == '\r' || afterColon[i] == '\n' {
		return "", false
	}

	first := afterColon[i]
	if first == '|' || first == '>' || first == '[' || first == '{' {
		return "", false
	}

	lineEnd := bytes.IndexAny(afterColon[i:], "\r\n")
	var line []byte
	if lineEnd == -1 {
		line = afterColon[i:]
	} else {
		line = afterColon[i : i+lineEnd]
	}

	if line[0] == '"' {
		return parseQuotedDouble(line)
	}
	if line[0] == '\'' {
		return parseQuotedSingle(line)
	}

	return parseUnquoted(line)
}

func parseQuotedDouble(line []byte) (string, bool) {
	escaped := false
	closeIdx := -1
	for j := 1; j < len(line); j++ {
		if line[j] == '\\' && !escaped {
			escaped = true
			continue
		}
		if line[j] == '"' && !escaped {
			closeIdx = j
			break
		}
		escaped = false
	}
	if closeIdx == -1 {
		return "", false
	}
	val := line[1:closeIdx]
	rest := bytes.TrimSpace(line[closeIdx+1:])
	if len(rest) > 0 && rest[0] == ',' {
		rest = bytes.TrimSpace(rest[1:])
	}
	if len(rest) > 0 && rest[0] != '#' {
		return "", false
	}
	if bytes.IndexByte(val, '\\') != -1 {
		return "", false
	}
	return string(val), true
}

func parseQuotedSingle(line []byte) (string, bool) {
	closeIdx := -1
	for j := 1; j < len(line); j++ {
		if line[j] == '\'' {
			if j+1 < len(line) && line[j+1] == '\'' {
				return "", false
			}
			closeIdx = j
			break
		}
	}
	if closeIdx == -1 {
		return "", false
	}
	val := line[1:closeIdx]
	rest := bytes.TrimSpace(line[closeIdx+1:])
	if len(rest) > 0 && rest[0] == ',' {
		rest = bytes.TrimSpace(rest[1:])
	}
	if len(rest) > 0 && rest[0] != '#' {
		return "", false
	}
	return string(val), true
}

func parseUnquoted(line []byte) (string, bool) {
	line = bytes.TrimRight(line, " \t")
	if commentIdx := bytes.Index(line, []byte(" #")); commentIdx != -1 {
		line = bytes.TrimRight(line[:commentIdx], " \t")
	} else if bytes.HasPrefix(line, []byte("#")) {
		return "", false
	}

	if len(line) > 0 && line[len(line)-1] == ',' {
		line = bytes.TrimRight(line[:len(line)-1], " \t")
	}

	val := string(line)
	if val == "null" || val == "~" {
		return "", true
	}
	return val, true
}

func extractWithYAMLNode(data []byte, properties []string) map[string]string {
	result := make(map[string]string, len(properties))
	if len(data) == 0 || len(properties) == 0 {
		return result
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return result
	}

	var root *yaml.Node
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	} else if doc.Kind == yaml.MappingNode {
		root = &doc
	}

	if root == nil || root.Kind != yaml.MappingNode {
		return result
	}

	targets := make(map[string]struct{}, len(properties))
	for _, p := range properties {
		targets[p] = struct{}{}
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]
		if _, needed := targets[keyNode.Value]; needed {
			result[keyNode.Value] = valNode.Value
		}
	}

	return result
}
