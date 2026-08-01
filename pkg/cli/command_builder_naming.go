package cli

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// numericCASCommandStem matches CSPEC stems that are object-id shaped
// (timestamp_hex), e.g. "1785199714348041000_411cd659". Those must not become
// Go builder filenames/constructors.
var numericCASCommandStem = regexp.MustCompile(`^\d+_[0-9a-fA-F]+$`)

// IsNumericCASCommandStem reports whether stem is a timestamp_hex CAS id fragment
// unsuitable as a command builder name.
func IsNumericCASCommandStem(stem string) bool {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return false
	}
	stem = strings.ReplaceAll(stem, "-", "_")
	return numericCASCommandStem.MatchString(stem)
}

// IsProcessCommandSpecCAS reports whether YAML fields describe a process-layer
// command_spec CAS instance (not DNA under .zqk/cli/specs).
func IsProcessCommandSpecCAS(tempSpec map[string]any) bool {
	if tempSpec == nil {
		return false
	}
	kind, _ := tempSpec[objects.FieldKeyKind].(string)
	id, _ := tempSpec[objects.FieldKeyID].(string)
	return kind == objects.KindCommandSpec && strings.HasPrefix(id, "CSPEC-")
}

// ResolveCommandBuilderName derives the flat builder stem (underscores, no
// _command_builder suffix) from DNA-oriented fields. Prefer name/use, then
// path; use CSPEC-* only when the stem is not numeric CAS-shaped.
func ResolveCommandBuilderName(tempSpec map[string]any, yamlPath string) string {
	if tempSpec != nil {
		if name := commandNameFromSpecFields(tempSpec); name != "" {
			return name
		}
		if idStr, ok := tempSpec[objects.FieldKeyID].(string); ok && strings.HasPrefix(idStr, "CSPEC-") {
			base := strings.TrimPrefix(idStr, "CSPEC-")
			base = strings.TrimSuffix(base, "_command")
			if strings.HasPrefix(base, "root-") {
				base = strings.TrimPrefix(base, "root-")
			}
			base = strings.ReplaceAll(base, "-", "_")
			if !IsNumericCASCommandStem(base) {
				return base
			}
		}
	}
	return commandNameFromYAMLPath(yamlPath)
}

func commandNameFromSpecFields(tempSpec map[string]any) string {
	for _, key := range []string{"name", "use"} {
		raw, _ := tempSpec[key].(string)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// "get <id>" / "orchestrate-batch [priority_plan_id] ..." → first token
		if idx := strings.IndexAny(raw, " \t["); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
		if raw == "" || IsNumericCASCommandStem(strings.ReplaceAll(raw, "-", "_")) {
			continue
		}
		return strings.ReplaceAll(raw, "-", "_")
	}
	return ""
}

func commandNameFromYAMLPath(yamlPath string) string {
	base := filepath.Base(yamlPath)
	base = strings.TrimSuffix(base, "_command.yaml")
	base = strings.TrimSuffix(base, "_command.yml")
	base = strings.TrimSuffix(base, ".yaml")
	base = strings.TrimSuffix(base, ".yml")
	base = strings.ReplaceAll(base, "-", "_")
	if IsNumericCASCommandStem(base) {
		// Hash-named CAS blob with no usable name/use — refuse numeric stem.
		return ""
	}
	return base
}
