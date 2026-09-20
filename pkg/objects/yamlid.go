// Package objects provides extraction of id (and kind) from YAML object files
// using the project's fast inspection utility.

package objects

const (
	yamlIDReadLimit = defaultYAMLInspectReadLimit // First N bytes; id/kind are at top of object YAML files
	emptyValue      = ""
)

// ReadIDFromYAMLFile reads the file (prefix only) and extracts the "id" field using the fast YAML inspection utility.
func ReadIDFromYAMLFile(path string) (id string) {
	return ExtractPropertyFromFile(path, FieldKeyID)
}

// ReadIDAndKindFromYAMLFile reads the file (prefix first, with full file fallback) and extracts "id" and "kind"
// using the fast YAML inspection utility.
func ReadIDAndKindFromYAMLFile(path string) (id, kind string) {
	props := ExtractPropertiesFromFile(path, FieldKeyID, FieldKeyKind)
	return props[FieldKeyID], props[FieldKeyKind]
}

// ExtractIDAndKindFromYAMLPrefix extracts id and kind from the prefix of YAML data using the fast YAML inspection utility.
// Used by tests; callers typically use ReadIDAndKindFromYAMLFile(path).
func ExtractIDAndKindFromYAMLPrefix(data []byte) (id, kind string) {
	if len(data) > yamlIDReadLimit {
		data = data[:yamlIDReadLimit]
	}
	props := ExtractProperties(data, FieldKeyID, FieldKeyKind)
	return props[FieldKeyID], props[FieldKeyKind]
}
