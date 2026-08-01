package routing_builders

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/drifthotspots"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const defaultVersion = "v1_0_0"
const defaultVersionPackage = "bldr_routing_v1"

// GenerateBuilderFromYAML reads a YAML routing rules file and generates a builder Go file
func GenerateBuilderFromYAML(yamlPath, outputDir string) error {
	// Read YAML file
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Parse YAML into array of maps (raw structure)
	var rules []map[string]any
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Determine file name (without extension)
	baseName := filepath.Base(yamlPath)
	fileName := strings.TrimSuffix(baseName, ".yaml")
	fileName = strings.TrimSuffix(fileName, ".yml")

	// Generate to sibling directory: pkg/specbuilder/bldr_routing_v1 (same level as routing_builders)
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

	// Generate builder code
	code, err := generateBuilderCode(rules, fileName, defaultVersion, defaultVersionPackage, wireToConst, modImport, filepath.Base(outputDir))
	if err != nil {
		return errfmt.Newf("failed to generate code").Wrap(err)
	}

	// Format Go code
	formatted, err := format.Source([]byte(code))
	if err != nil {
		// If formatting fails, use unformatted code (better than failing completely)
		formatted = []byte(code)
	}

	// Output file: {file_name}_builder.go (version in directory/package name, not filename)
	outputFile := filepath.Join(versionDir, fmt.Sprintf("%s_builder.go", fileName))

	// Write output file
	if err := os.WriteFile(outputFile, formatted, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

func valueUsesFieldKeyLiterals(val any, wireToConst map[string]string) bool {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			if _, ok := wireToConst[k]; ok {
				return true
			}
			if valueUsesFieldKeyLiterals(child, wireToConst) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if valueUsesFieldKeyLiterals(item, wireToConst) {
				return true
			}
		}
	}
	return false
}

func rulesUseFieldKeyLiterals(rules []map[string]any, wireToConst map[string]string) bool {
	for _, rule := range rules {
		if valueUsesFieldKeyLiterals(rule, wireToConst) {
			return true
		}
	}
	return false
}

// generateBuilderCode generates Go code for a routing rule builder from an array of rules
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateBuilderCode(rules []map[string]any, fileName, version, packageName string, wireToConst map[string]string, moduleImport, specbuilderChildDir string) (string, error) {
	var buf strings.Builder

	// Generate type name (e.g., "DefaultRulesBuilder" from "default_rules")
	typeName := toCamelCase(fileName) + "Builder"
	constructorName := "New" + typeName

	needObjectsImport := len(rules) > 0 && rulesUseFieldKeyLiterals(rules, wireToConst)
	parentImport := paths.SpecbuilderPackageImportPath(moduleImport, specbuilderChildDir)

	// Package name (e.g., "package bldr_routing_v1")
	fmt.Fprintf(&buf, "package %s\n\n", packageName)

	// Import parent routing_builders package
	buf.WriteString("import (\n")
	if needObjectsImport {
		fmt.Fprintf(&buf, "\t%q\n", paths.ObjectsImportPath(moduleImport))
	}
	fmt.Fprintf(&buf, "\t%q\n", parentImport)
	buf.WriteString(")\n\n")

	// Type definition
	fmt.Fprintf(&buf, "// %s builds the %s routing rules at version %s\n", typeName, fileName, version)
	fmt.Fprintf(&buf, "// File: %s/%s_builder.go - version is encoded in package/directory name\n", packageName, fileName)
	fmt.Fprintf(&buf, "type %s struct {\n", typeName)
	buf.WriteString("\t*routing_builders.BaseRoutingRuleBuilder\n")
	buf.WriteString("}\n\n")

	// Constructor
	fmt.Fprintf(&buf, "// %s creates a new builder for %s routing rules version %s\n", constructorName, fileName, version)
	fmt.Fprintf(&buf, "func %s() *%s {\n", constructorName, typeName)
	fmt.Fprintf(&buf, "\tbuilder := &%s{\n", typeName)
	fmt.Fprintf(&buf, "\t\tBaseRoutingRuleBuilder: routing_builders.NewBaseRoutingRuleBuilder(%q, %q),\n", fileName, version)
	buf.WriteString("\t}\n\n")

	// Add rules
	if len(rules) > 0 {
		buf.WriteString("\t// Add routing rules\n")
		fmt.Fprintf(&buf, "\tbuilder.add%sRules()\n\n", toCamelCase(fileName))
		buf.WriteString("\treturn builder\n")
		buf.WriteString("}\n\n")

		// Generate rules adder method
		methodName := fmt.Sprintf("add%sRules", toCamelCase(fileName))
		fmt.Fprintf(&buf, "// %s adds the %s routing rules\n", methodName, fileName)
		fmt.Fprintf(&buf, "func (b *%s) %s() {\n", typeName, methodName)

		for _, rule := range rules {
			buf.WriteString("\n\tb.AddRule(")
			buf.WriteString(formatValueWithFieldKeys(rule, 1, wireToConst))
			buf.WriteString(")")
		}

		buf.WriteString("\n}\n\n")
	} else {
		buf.WriteString("\treturn builder\n")
		buf.WriteString("}\n\n")
	}

	// Init function for registration
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\trouting_builders.RegisterBuilder(%s())\n", constructorName)
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
