package trait_builders

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const (
	emptyValue            = ""
	defaultVersion        = "v1_0_0"
	defaultVersionPackage = "bldr_trait_v1"
)

// GenerateBuilderFromYAML reads a YAML trait file and generates a builder Go file
func GenerateBuilderFromYAML(yamlPath, outputDir string) error {
	// Read YAML file
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Parse YAML into trait file structure (matching TraitRegistry.loadTraitFromFile)
	var traitFile struct {
		Name         string         `yaml:"name"`
		Description  string         `yaml:"description"`
		Category     string         `yaml:"category"`
		ObjectLevel  bool           `yaml:"object_level"`
		FieldLevel   bool           `yaml:"field_level"`
		Requires     []string       `yaml:"requires"`
		Conflicts    []string       `yaml:"conflicts"`
		Includes     []string       `yaml:"includes"`
		Version      string         `yaml:"version"`
		Status       string         `yaml:"status"`
		ObjectConfig map[string]any `yaml:"object_config"`
		FieldConfig  map[string]any `yaml:"field_config"`
	}

	if err := yaml.Unmarshal(data, &traitFile); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Skip non-active traits (matching TraitRegistry behavior)
	if traitFile.Status != emptyValue && traitFile.Status != objects.ObjectStatusActive {
		return errfmt.Errorf("trait %s has status '%s', skipping", traitFile.Name, traitFile.Status)
	}

	// Determine trait name (use filename as fallback if name field is empty)
	baseName := filepath.Base(yamlPath)
	traitName := strings.TrimSuffix(baseName, ".yaml")
	traitName = strings.TrimSuffix(traitName, ".yml")
	if traitFile.Name != emptyValue {
		traitName = traitFile.Name
	}

	// Generate to sibling directory: pkg/specbuilder/bldr_trait_v1 (same level as trait_builders)
	parentDir := filepath.Dir(outputDir)
	versionDir := filepath.Join(parentDir, defaultVersionPackage)
	if err := os.MkdirAll(versionDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create version directory").Wrap(err)
	}

	// Generate builder code (extract fields from traitFile)
	code, err := generateBuilderCode(
		traitFile.Name,
		traitFile.Description,
		traitFile.Category,
		traitFile.ObjectLevel,
		traitFile.FieldLevel,
		traitFile.Requires,
		traitFile.Conflicts,
		traitFile.Includes,
		traitFile.ObjectConfig,
		traitFile.FieldConfig,
		traitName,
		defaultVersion,
		defaultVersionPackage,
	)
	if err != nil {
		return errfmt.Newf("failed to generate code").Wrap(err)
	}

	// Format Go code
	formatted, err := format.Source([]byte(code))
	if err != nil {
		// If formatting fails, use unformatted code (better than failing completely)
		formatted = []byte(code)
	}

	// Output file: {trait_name}_builder.go (version in directory/package name, not filename)
	outputFile := filepath.Join(versionDir, fmt.Sprintf("%s_builder.go", traitName))

	// Write output file
	if err := os.WriteFile(outputFile, formatted, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

// generateBuilderCode generates Go code for a trait builder
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateBuilderCode(
	_ string, description, category string,
	objectLevel, fieldLevel bool,
	requires, conflicts, includes []string,
	objectConfig, fieldConfig map[string]any,
	traitName, version, packageName string,
) (string, error) {
	var buf strings.Builder

	// Generate type name (e.g., "ListableBuilder" from "listable")
	typeName := toCamelCase(traitName) + "Builder"
	constructorName := "New" + typeName

	// Package name (e.g., "package bldr_trait_v1")
	fmt.Fprintf(&buf, "package %s\n\n", packageName)

	// Import parent trait_builders package
	buf.WriteString("import (\n")
	buf.WriteString("\t\"github.com/lanceman/zqk/pkg/specbuilder/trait_builders\"\n")
	buf.WriteString(")\n\n")

	// Type definition
	fmt.Fprintf(&buf, "// %s builds the %s trait at version %s\n", typeName, traitName, version)
	fmt.Fprintf(&buf, "// File: %s/%s_builder.go - version is encoded in package/directory name\n", packageName, traitName)
	fmt.Fprintf(&buf, "type %s struct {\n", typeName)
	buf.WriteString("\t*trait_builders.BaseTraitBuilder\n")
	buf.WriteString("}\n\n")

	// Constructor
	fmt.Fprintf(&buf, "// %s creates a new builder for %s trait version %s\n", constructorName, traitName, version)
	fmt.Fprintf(&buf, "func %s() *%s {\n", constructorName, typeName)
	fmt.Fprintf(&buf, "\tbuilder := &%s{\n", typeName)
	fmt.Fprintf(&buf, "\t\tBaseTraitBuilder: trait_builders.NewBaseTraitBuilder(%q, %q),\n", traitName, version)
	buf.WriteString("\t}\n\n")

	// Configure trait
	hasConfig := len(objectConfig) > 0 || len(fieldConfig) > 0
	hasSetters := description != emptyValue || category != emptyValue || objectLevel || fieldLevel || len(requires) > 0 || len(conflicts) > 0 || len(includes) > 0 || hasConfig

	if hasSetters {
		buf.WriteString("\t// Configure the trait\n")
		buf.WriteString("\tbuilder.")

		// Set description
		if description != emptyValue {
			desc := strings.ReplaceAll(description, "\n", "\\n")
			desc = strings.ReplaceAll(desc, "\"", "\\\"")
			fmt.Fprintf(&buf, "\n\t\tSetDescription(%q).", desc)
		}

		// Set category
		if category != emptyValue {
			fmt.Fprintf(&buf, "\n\t\tSetCategory(%q).", category)
		}

		// Set object level and field level (always present)
		hasMoreAfterFieldLevel := len(requires) > 0 || len(conflicts) > 0 || len(includes) > 0 || hasConfig
		fmt.Fprintf(&buf, "\n\t\tSetObjectLevel(%v).", objectLevel)
		fmt.Fprintf(&buf, "\n\t\tSetFieldLevel(%v)", fieldLevel)
		if hasMoreAfterFieldLevel {
			buf.WriteString(".")
		}

		// Add requires
		hasMoreAfterRequires := len(conflicts) > 0 || len(includes) > 0 || hasConfig
		for i, req := range requires {
			fmt.Fprintf(&buf, "\n\t\tAddRequires(%q", req)
			if i < len(requires)-1 || hasMoreAfterRequires {
				buf.WriteString(").")
			} else {
				buf.WriteString(")")
			}
		}

		// Add conflicts
		hasMoreAfterConflicts := len(includes) > 0 || hasConfig
		for i, conflict := range conflicts {
			fmt.Fprintf(&buf, "\n\t\tAddConflicts(%q", conflict)
			if i < len(conflicts)-1 || hasMoreAfterConflicts {
				buf.WriteString(").")
			} else {
				buf.WriteString(")")
			}
		}

		// Add includes
		hasMoreAfterIncludes := hasConfig
		for i, include := range includes {
			fmt.Fprintf(&buf, "\n\t\tAddIncludes(%q", include)
			if i < len(includes)-1 || hasMoreAfterIncludes {
				buf.WriteString(").")
			} else {
				buf.WriteString(")")
			}
		}

		// Set config (from object_config and field_config)
		if hasConfig {
			config := make(map[string]any)
			if len(objectConfig) > 0 {
				config["object_config"] = objectConfig
			}
			if len(fieldConfig) > 0 {
				config["field_config"] = fieldConfig
			}
			buf.WriteString("\n\t\tSetConfig(")
			buf.WriteString(formatValue(config, 2))
			buf.WriteString(")")
		}

		buf.WriteString("\n\n")
	}

	buf.WriteString("\treturn builder\n")
	buf.WriteString("}\n\n")

	// Init function for registration
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\ttrait_builders.RegisterBuilder(%s())\n", constructorName)
	buf.WriteString("}\n")

	return buf.String(), nil
}

// formatValue formats a value as Go code
func formatValue(val any, indentLevel int) string {
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
			buf.WriteString(formatValue(item, indentLevel+1))
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
			fmt.Fprintf(&buf, "%q: ", k)
			buf.WriteString(formatValue(v[k], indentLevel+1))
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
