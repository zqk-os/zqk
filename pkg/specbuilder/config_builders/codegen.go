package config_builders

import (
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const defaultVersion = "v1_0_0"
const defaultVersionPackage = "bldr_config_v1"
const configBuildersImportPath = "github.com/zqk-os/zqk/pkg/specbuilder/config_builders"

// codegenSourcePath is this file; embedded in generated builder headers so editors and agents do not hand-edit outputs.
const codegenSourcePath = "pkg/specbuilder/config_builders/codegen.go"

const (
	defaultDirectoryPerm fileutil.FileMode = paths.DirPerm755
	defaultFilePerm      fileutil.FileMode = paths.FilePerm600
)

// defaultConfigYAMLVersion is the top-level "version" string in internal config YAML for v1 builders.
// When generating into defaultVersionPackage, codegen emits ConfigYAMLVersionV1 in map literals
// (see pkg/specbuilder/bldr_config_v1/config_version.go) instead of a string literal.
const defaultConfigYAMLVersion = "1.0.0"

// schedulerMaintenanceConfigFileName is the basename (no extension) of scheduler_maintenance_config.yaml.
// Map literals for required_jobs use objects.FieldKey* for id and job_type so field-key literal checks pass.
const schedulerMaintenanceConfigFileName = "scheduler_maintenance_config"

// GenerateBuilderFromYAML reads a YAML config file and generates a builder Go file
func GenerateBuilderFromYAML(yamlPath, outputDir string) error {
	// Read YAML file
	data, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Parse YAML into map (raw structure)
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Determine file name (without extension)
	baseName := filepath.Base(yamlPath)
	fileName := strings.TrimSuffix(baseName, ".yaml")
	fileName = strings.TrimSuffix(fileName, ".yml")

	// Generate to sibling directory: pkg/specbuilder/bldr_config_v1 (same level as config_builders)
	parentDir := filepath.Dir(outputDir)
	versionDir := filepath.Join(parentDir, defaultVersionPackage)
	if err := fileutil.MkdirAll(versionDir, defaultDirectoryPerm); err != nil {
		return errfmt.Newf("failed to create version directory").Wrap(err)
	}

	sourceYAML := sourceYAMLForCodegenHeader(yamlPath)

	// Generate builder code
	code, err := generateBuilderCode(config, fileName, defaultVersion, defaultVersionPackage, sourceYAML)
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
	if err := fileutil.WriteFile(outputFile, formatted, defaultFilePerm); err != nil {
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

// sourceYAMLForCodegenHeader returns a stable, repo-relative path when possible (for generated file headers).
func sourceYAMLForCodegenHeader(yamlPath string) string {
	clean := filepath.Clean(yamlPath)
	cwd, err := fileutil.Getwd()
	if err != nil {
		return filepath.ToSlash(clean)
	}
	absYAML, err := filepath.Abs(clean)
	if err != nil {
		return filepath.ToSlash(clean)
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return filepath.ToSlash(clean)
	}
	rel, err := filepath.Rel(absCwd, absYAML)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return filepath.ToSlash(absYAML)
	}
	return filepath.ToSlash(rel)
}

// generateBuilderCode generates Go code for a config builder from a config map
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateBuilderCode(config map[string]any, fileName, version, packageName, sourceYAML string) (string, error) {
	var buf strings.Builder

	// Generate type name (e.g., "PathsConfigBuilder" from "paths_config")
	typeName := toCamelCase(fileName) + "Builder"
	constructorName := "New" + typeName

	writeGeneratedConfigBuilderHeader(&buf, sourceYAML)
	writePackageDeclaration(&buf, packageName)
	writeImportBlock(&buf, true)

	// Type definition
	fmt.Fprintf(&buf, "// %s builds the %s config at version %s\n", typeName, fileName, version)
	fmt.Fprintf(&buf, "// File: %s/%s_builder.go - version is encoded in package/directory name\n", packageName, fileName)
	fmt.Fprintf(&buf, "type %s struct {\n", typeName)
	buf.WriteString("\t*config_builders.BaseConfigBuilder\n")
	buf.WriteString("}\n\n")

	// Constructor
	fmt.Fprintf(&buf, "// %s creates a new builder for %s config version %s\n", constructorName, fileName, version)
	fmt.Fprintf(&buf, "func %s() *%s {\n", constructorName, typeName)
	fmt.Fprintf(&buf, "\tbuilder := &%s{\n", typeName)
	fmt.Fprintf(&buf, "\t\tBaseConfigBuilder: config_builders.NewBaseConfigBuilder(%q, %q),\n", fileName, version)
	buf.WriteString("\t}\n\n")

	// Set config
	buf.WriteString("\t// Configure the config\n")
	buf.WriteString("\tbuilder.\n\t\tSetConfig(")
	buf.WriteString(formatValue(config, 2, packageName, fileName))
	buf.WriteString(")\n\n")

	buf.WriteString("\treturn builder\n")
	buf.WriteString("}\n\n")

	// Init function for registration
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\tconfig_builders.RegisterBuilder(%s())\n", constructorName)
	buf.WriteString("}\n")

	return buf.String(), nil
}

func writeGeneratedConfigBuilderHeader(buf *strings.Builder, sourceYAML string) {
	fmt.Fprintf(buf, "// Code generated by %s from %s.\n", codegenSourcePath, sourceYAML)
	fmt.Fprintf(buf, "// DO NOT EDIT. Regenerate with: %s\n\n", paths.CLIUsage("system", "generate-config-builders", "--overwrite"))
}

func writePackageDeclaration(buf *strings.Builder, packageName string) {
	buf.WriteString("// Code generated by zqk-admin system generate-builders. DO NOT EDIT.\n\n")
	fmt.Fprintf(buf, "package %s\n\n", packageName)
}

func writeImportBlock(buf *strings.Builder, includeObjects bool) {
	buf.WriteString("import (\n")
	fmt.Fprintf(buf, "\t%q\n", configBuildersImportPath)
	if includeObjects {
		buf.WriteString("\t\"github.com/zqk-os/zqk/pkg/objects\"\n")
	}
	buf.WriteString(")\n\n")
}

// formatValue formats a value as Go code. packageName is the generated package (e.g. bldr_config_v1)
// so embedded YAML "version" fields can reference ConfigYAMLVersionV1.
// fileName is the config basename (no extension); scheduler_maintenance_config maps use objects.FieldKey* for id/job_type.
func formatValue(val any, indentLevel int, packageName, fileName string) string {
	indent := strings.Repeat("\t", indentLevel)

	switch v := val.(type) {
	case string:
		if v == objects.DefaultSystemAccountID {
			return "objects.DefaultSystemAccountID"
		}
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
			buf.WriteString(formatValue(item, indentLevel+1, packageName, fileName))
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
			switch {
			case fileName == schedulerMaintenanceConfigFileName && k == objects.FieldKeyID:
				buf.WriteString("objects.FieldKeyID: ")
			case fileName == schedulerMaintenanceConfigFileName && k == objects.FieldKeyJobType:
				buf.WriteString("objects.FieldKeyJobType: ")
			default:
				fmt.Fprintf(&buf, "%q: ", k)
			}
			if packageName == defaultVersionPackage && k == "version" {
				if s, ok := v[k].(string); ok && s == defaultConfigYAMLVersion {
					buf.WriteString("ConfigYAMLVersionV1")
				} else {
					buf.WriteString(formatValue(v[k], indentLevel+1, packageName, fileName))
				}
			} else {
				buf.WriteString(formatValue(v[k], indentLevel+1, packageName, fileName))
			}
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
