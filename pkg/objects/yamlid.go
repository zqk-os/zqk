// Package objects provides extraction of id (and kind) from YAML object files
// using the project's YAML library. Reads only a prefix of the file for performance.

package objects

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	yamlIDReadLimit = 1024 // First N bytes; id/kind are at top of object YAML files
	emptyValue      = ""
)

// idKindFields is the minimal struct for yaml.Unmarshal to populate only id and kind.
type idKindFields struct {
	ID   string `yaml:"id"`
	Kind string `yaml:"kind"`
}

// ReadIDFromYAMLFile reads the file (prefix only) and extracts the "id" field using the YAML library.
func ReadIDFromYAMLFile(path string) (id string) {
	id, _ = ReadIDAndKindFromYAMLFile(path)
	return id
}

// ReadIDAndKindFromYAMLFile reads the file (prefix first, with full file fallback) and extracts "id" and "kind" using the YAML library.
// Uses a bounded read then yaml.Unmarshal so we only parse the top of the file in the common case.
func ReadIDAndKindFromYAMLFile(path string) (id, kind string) {
	f, err := fileutil.Open(path)
	if err != nil {
		return emptyValue, emptyValue
	}
	defer f.Close()

	buf := make([]byte, yamlIDReadLimit)
	n, _ := f.Read(buf)
	limitData := buf[:n]

	id = extractField(limitData, "id")
	kind = extractField(limitData, "kind")
	if id != "" && kind != "" {
		return id, kind
	}

	// Fallback to read the rest of the file
	rest, err := io.ReadAll(f)
	if err == nil {
		fullData := append(limitData, rest...)
		id = extractField(fullData, "id")
		kind = extractField(fullData, "kind")
		if id != "" && kind != "" {
			return id, kind
		}

		var out idKindFields
		if err := yaml.Unmarshal(fullData, &out); err == nil {
			return out.ID, out.Kind
		}
	}

	return id, kind
}

func extractField(data []byte, field string) string {
	idx := bytes.Index(data, []byte("\n"+field+": "))
	if idx == -1 {
		// check if it's the very first line
		if bytes.HasPrefix(data, []byte(field+": ")) {
			idx = 0
		} else {
			return ""
		}
	} else {
		idx++ // skip the newline
	}

	start := idx + len(field) + 2 // length of "field: "
	end := bytes.IndexByte(data[start:], '\n')
	if end == -1 {
		end = len(data) - start
	}
	val := bytes.TrimSpace(data[start : start+end])
	val = bytes.Trim(val, `"'`)
	return string(val)
}

// ExtractIDAndKindFromYAMLPrefix unmarshals the given YAML bytes into id and kind using string scanning.
// Used by tests; callers typically use ReadIDAndKindFromYAMLFile(path).
func ExtractIDAndKindFromYAMLPrefix(data []byte) (id, kind string) {
	if len(data) > yamlIDReadLimit {
		data = data[:yamlIDReadLimit]
	}
	id = extractField(data, "id")
	kind = extractField(data, "kind")

	// Fallback to unmarshal if we couldn't find them via string matching,
	// in case they are indented or differently formatted (though they shouldn't be).
	if id == "" || kind == "" {
		var out idKindFields
		if err := yaml.Unmarshal(data, &out); err == nil {
			if id == "" {
				id = out.ID
			}
			if kind == "" {
				kind = out.Kind
			}
		}
	}
	return id, kind
}
