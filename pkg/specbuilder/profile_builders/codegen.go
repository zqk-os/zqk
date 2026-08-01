package profile_builders

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/drifthotspots"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const (
	emptyValue            = ""
	defaultVersion        = "v1_0_0"
	defaultVersionPackage = "bldr_profile_v1"
)

// GenerateBuilderFromYAML reads a YAML profile file and generates a builder Go file
func GenerateBuilderFromYAML(yamlPath, outputDir string) error {
	// Read YAML file
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Parse YAML into UnifiedProfile
	var profile config.UnifiedProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Determine profile name (use filename as fallback if metadata.name is empty)
	baseName := filepath.Base(yamlPath)
	profileName := strings.TrimSuffix(baseName, ".yaml")
	profileName = strings.TrimSuffix(profileName, ".yml")
	if profile.Metadata.Name != emptyValue {
		profileName = profile.Metadata.Name
	}

	// Generate to sibling directory: pkg/specbuilder/bldr_profile_v1 (same level as profile_builders)
	parentDir := filepath.Dir(outputDir)
	versionDir := filepath.Join(parentDir, defaultVersionPackage)
	if err := os.MkdirAll(versionDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create version directory").Wrap(err)
	}

	modRoot, err := paths.ModuleRootFromPath(outputDir)
	if err != nil {
		return errfmt.Newf("module root for codegen").Wrap(err)
	}
	modImport, err := paths.ModuleImportPath(modRoot)
	if err != nil {
		return errfmt.Newf("module import path").Wrap(err)
	}
	wireToConst, err := drifthotspots.LoadFieldKeyWireToConstFromModuleRoot(modRoot)
	if err != nil {
		return errfmt.Newf("field keys for codegen").Wrap(err)
	}

	// Generate builder code (use determined profile name)
	code, err := generateBuilderCode(&profile, profileName, defaultVersion, defaultVersionPackage, wireToConst, modImport, filepath.Base(outputDir))
	if err != nil {
		return errfmt.Newf("failed to generate code").Wrap(err)
	}

	// Format Go code
	formatted, err := format.Source([]byte(code))
	if err != nil {
		// If formatting fails, use unformatted code (better than failing completely)
		formatted = []byte(code)
	}

	// Output file: {profile_name}_builder.go (version in directory/package name, not filename)
	outputFile := filepath.Join(versionDir, fmt.Sprintf("%s_builder.go", profileName))

	// Write output file
	if err := os.WriteFile(outputFile, formatted, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

func specUsesFieldKeyLiterals(val any, wireToConst map[string]string) bool {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			if _, ok := wireToConst[k]; ok {
				return true
			}
			if specUsesFieldKeyLiterals(child, wireToConst) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if specUsesFieldKeyLiterals(item, wireToConst) {
				return true
			}
		}
	}
	return false
}

// generateBuilderCode generates Go code for a profile builder from a UnifiedProfile
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateBuilderCode(profile *config.UnifiedProfile, profileName, version, packageName string, wireToConst map[string]string, moduleImport, specbuilderChildDir string) (string, error) {
	var buf strings.Builder

	// Generate type name (e.g., "BaseProfileBuilder" from "base_profile")
	typeName := toCamelCase(profileName) + "Builder"
	constructorName := "New" + typeName

	needObjectsImport := len(profile.Spec) > 0 && specUsesFieldKeyLiterals(profile.Spec, wireToConst)
	parentImport := paths.SpecbuilderPackageImportPath(moduleImport, specbuilderChildDir)

	// Package name (e.g., "package bldr_profile_v1")
	fmt.Fprintf(&buf, "package %s\n\n", packageName)

	// Import parent profile_builders package
	buf.WriteString("import (\n")
	fmt.Fprintf(&buf, "\t%q\n", paths.ConfigImportPath(moduleImport))
	if needObjectsImport {
		fmt.Fprintf(&buf, "\t%q\n", paths.ObjectsImportPath(moduleImport))
	}
	fmt.Fprintf(&buf, "\t%q\n", parentImport)
	buf.WriteString(")\n\n")

	// Type definition
	fmt.Fprintf(&buf, "// %s builds the %s profile at version %s\n", typeName, profileName, version)
	fmt.Fprintf(&buf, "// File: %s/%s_builder.go - version is encoded in package/directory name\n", packageName, profileName)
	fmt.Fprintf(&buf, "type %s struct {\n", typeName)
	buf.WriteString("\t*profile_builders.BaseProfileBuilder\n")
	buf.WriteString("}\n\n")

	// Constructor
	fmt.Fprintf(&buf, "// %s creates a new builder for %s profile version %s\n", constructorName, profileName, version)
	fmt.Fprintf(&buf, "func %s() *%s {\n", constructorName, typeName)
	fmt.Fprintf(&buf, "\tbuilder := &%s{\n", typeName)
	fmt.Fprintf(&buf, "\t\tBaseProfileBuilder: profile_builders.NewBaseProfileBuilder(%q, %q),\n", profileName, version)
	buf.WriteString("\t}\n\n")

	// Configure profile
	buf.WriteString("\t// Configure the profile\n")
	buf.WriteString("\tbuilder.")

	// Set schema version (DRY: shared const in bldr_profile_v1 for the default YAML schema)
	if profile.SchemaVersion != emptyValue {
		if packageName == defaultVersionPackage && profile.SchemaVersion == DefaultProfileSchemaVersion {
			buf.WriteString("\n\t\tSetSchemaVersion(SchemaVersionV1).")
		} else {
			fmt.Fprintf(&buf, "\n\t\tSetSchemaVersion(%q).", profile.SchemaVersion)
		}
	}

	// Set kind
	if profile.Kind != emptyValue {
		fmt.Fprintf(&buf, "\n\t\tSetKind(%q).", profile.Kind)
	}

	// Set type
	if profile.Type != emptyValue {
		var typeConst string
		switch profile.Type {
		case config.ProfileTypeCLIContext:
			typeConst = "ProfileTypeCLIContext"
		case config.ProfileTypeMetricsSampler:
			typeConst = "ProfileTypeMetricsSampler"
		case config.ProfileTypeTransceiverRouter:
			typeConst = "ProfileTypeTransceiverRouter"
		default:
			// Fallback: construct from string value
			typeConst = "ProfileType" + strings.ToUpper(string(profile.Type[0])) + strings.ReplaceAll(string(profile.Type[1:]), "_", "")
		}
		fmt.Fprintf(&buf, "\n\t\tSetType(config.%s).", typeConst)
	}

	// Set metadata
	buf.WriteString("\n\t\tSetMetadata(config.ProfileMetadata{")
	fmt.Fprintf(&buf, "\n\t\t\tName: %q,", profile.Metadata.Name)
	if profile.Metadata.Extends != emptyValue && profile.Metadata.Extends != "null" {
		fmt.Fprintf(&buf, "\n\t\t\tExtends: %q,", profile.Metadata.Extends)
	}
	if profile.Metadata.Description != emptyValue {
		desc := strings.ReplaceAll(profile.Metadata.Description, "\n", "\\n")
		desc = strings.ReplaceAll(desc, "\"", "\\\"")
		fmt.Fprintf(&buf, "\n\t\t\tDescription: %q,", desc)
	}
	buf.WriteString("\n\t\t}).")

	// Set spec
	if len(profile.Spec) > 0 {
		buf.WriteString("\n\t\tSetSpec(")
		buf.WriteString(formatValueWithFieldKeys(profile.Spec, 2, wireToConst))
		buf.WriteString(")")
	}

	buf.WriteString("\n\n")

	buf.WriteString("\treturn builder\n")
	buf.WriteString("}\n\n")

	// Init function for registration
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\tprofile_builders.RegisterBuilder(%s())\n", constructorName)
	buf.WriteString("}\n")

	return buf.String(), nil
}

// formatValueWithFieldKeys formats a value as Go code, using objects.FieldKey* for map keys
// that match field_keys.go wire values (field-key literal gate).
func formatValueWithFieldKeys(val any, indentLevel int, wireToConst map[string]string) string {
	indent := strings.Repeat("\t", indentLevel)

	switch v := val.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "nil"
	case []any:
		var buf strings.Builder
		buf.WriteString("[]any{\n")
		for _, item := range v {
			buf.WriteString(indent + "\t")
			buf.WriteString(formatValueWithFieldKeys(item, indentLevel+1, wireToConst))
			buf.WriteString(",\n")
		}
		buf.WriteString(indent + "}")
		return buf.String()
	case map[string]any:
		var buf strings.Builder
		buf.WriteString("map[string]any{\n")
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString(indent + "\t")
			if cname, ok := wireToConst[k]; ok {
				fmt.Fprintf(&buf, "objects.%s: ", cname)
			} else {
				fmt.Fprintf(&buf, "%q: ", k)
			}
			buf.WriteString(formatValueWithFieldKeys(v[k], indentLevel+1, wireToConst))
			buf.WriteString(",\n")
		}
		buf.WriteString(indent + "}")
		return buf.String()
	default:
		// For unknown types, try to convert to string (fallback)
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
}

// toCamelCase converts snake_case to CamelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[0:1]) + part[1:])
		}
	}
	return result.String()
}
