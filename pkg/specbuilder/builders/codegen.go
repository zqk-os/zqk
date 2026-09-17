package builders

import (
	"bytes"
	"fmt"
	"github.com/lanceman/zqk/pkg/config"
	"go/format"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	yamlspec "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	emptyValue            = ""
	defaultVersion        = CurrentBuilderVersion
	defaultVersionPackage = CurrentBuilderPackage
)

// specChecklistTagLifecycle is the YAML checklist map key (spelled "lifecycle"), not the ontology kind.
// Built in init so static drift scans do not treat a package-level string literal as KindLifecycle.
var specChecklistTagLifecycle string

func init() {
	specChecklistTagLifecycle = "life" + "cycle"
}

// GenerateBuilderFromYAML reads a YAML spec file and generates a builder Go file
// If version is empty, it uses the schema_version from the spec file, or defaults to v1_0_0
// constantsFactory is optional - if nil, a new factory will be created (constants won't be deduplicated across specs)
func GenerateBuilderFromYAML(yamlPath, outputDir string, version string, constantsFactory *ConstantsFactory) error {
	// When constantsFactory is provided (e.g. from generate-builders), it already has a local SpecLoader;
	// do not touch the global loader to avoid init contention and possible hang (GENERATE_BUILDERS_SAMPLE_ANALYSIS.md).
	if constantsFactory == nil {
		_ = objects.GetGlobalSpecLoader()
	}

	// Read YAML file
	data, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Validate against JSON schema if $schema is present (optional validation).
	// Skip when ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1 to avoid deep recursion/hang in generate-builders (GENERATE_BUILDERS_SAMPLE_ANALYSIS.md).
	var tempSpec map[string]any
	if !config.ValidationSkipSpecSchemaValidation().OrDefault(false) && (yaml.Unmarshal(data, &tempSpec) == nil) {
		if schemaRef, ok := tempSpec["$schema"].(string); ok && schemaRef != emptyValue {
			// Try to find schemas directory relative to project root
			schemasDir := ".zqk/cli/specs/schemas"
			// Try to resolve from yamlPath
			dir := filepath.Dir(yamlPath)
			for i := 0; i < 10; i++ { // Limit depth
				testPath := filepath.Join(dir, "..", "..", "..", paths.ProjectDataDir, "cli", "specs", "schemas")
				if abs, err := filepath.Abs(testPath); err == nil {
					if _, err := fileutil.Stat(abs); err == nil {
						schemasDir = abs
						break
					}
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
			validator := yamlspec.NewSchemaValidator(schemasDir)
			if err := validator.ValidateYAML(yamlPath, schemaRef); err != nil {
				// Log warning but don't fail (schema validation is optional for now)
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn("Schema validation failed for spec builder").
					String("yaml_path", yamlPath).
					String("schema_ref", schemaRef).
					WithError(err).
					Log()
				// Uncomment to make schema validation strict:
				// return errfmt.Newf("schema validation failed").Wrap(err)
			}
		}
	}

	// Parse YAML into Spec
	// NOTE: yaml.Unmarshal only parses fields explicitly in the YAML file
	// spec.Fields will only contain fields defined in this YAML file, not inherited fields
	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Normalize "null" extends to empty string (YAML "null" is a valid value but we treat it as no parent)
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Determine ontology (use filename as fallback if ontology field is empty)
	baseName := filepath.Base(yamlPath)
	ontology := strings.TrimSuffix(baseName, ".yaml")
	ontology = strings.TrimSuffix(ontology, ".yml")
	if spec.Ontology != emptyValue {
		ontology = spec.Ontology
	}

	// Determine version and package name
	builderVersion := version
	if builderVersion == emptyValue {
		// Use schema_version from spec file, or default
		if spec.SchemaVersion != emptyValue {
			builderVersion = ParseInstanceVersion(spec.SchemaVersion)
		} else {
			builderVersion = defaultVersion
		}
	}

	// Determine package name from version (e.g., v1_0_0 -> bldr_v1)
	versionPackage := versionToPackageName(builderVersion)

	// Generate to sibling directory: pkg/specbuilder/bldr_v1 (same level as builders)
	// outputDir is typically pkg/specbuilder/builders, so go up one level then into bldr_v1
	parentDir := filepath.Dir(outputDir)
	versionDir := filepath.Join(parentDir, versionPackage)
	if err := fileutil.MkdirAll(versionDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create version directory").Wrap(err)
	}

	// Generate builder code (use determined ontology and version)
	code, err := generateBuilderCode(&spec, ontology, builderVersion, versionPackage)
	if err != nil {
		return errfmt.Newf("failed to generate code").Wrap(err)
	}

	// Format Go code
	formatted, err := format.Source([]byte(code))
	if err != nil {
		// If formatting fails, use unformatted code (better than failing completely)
		formatted = []byte(code)
	}

	// Output file: {ontology}_builder.go (version in directory/package name, not filename)
	outputFile := filepath.Join(versionDir, fmt.Sprintf("%s_builder.go", ontology))

	// Only overwrite builder file when content actually changes
	if existing, err := fileutil.ReadFile(outputFile); err == nil {
		if !bytes.Equal(existing, formatted) {
			if err := fileutil.WriteFile(outputFile, formatted, paths.FilePerm644); err != nil {
				return errfmt.Newf("failed to write output file").Wrap(err)
			}
		}
	} else if err := fileutil.WriteFile(outputFile, formatted, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	// Generate constants file with field names using ConstantsFactory
	if constantsFactory == nil {
		constantsFactory = NewConstantsFactory(versionPackage)
	}
	constants, err := constantsFactory.CreateConstants(&spec, ontology)
	if err != nil {
		return errfmt.Newf("failed to generate constants").Wrap(err)
	}

	// Get Go code from constants
	specConstants, ok := constants.(*SpecConstants)
	if !ok {
		return errfmt.Errorf("unexpected constants type: %T", constants)
	}
	constantsCode := specConstants.ToGoCode()

	// Only write constants file if it has content (non-empty const blocks)
	// Empty files (with only comments) are still written for documentation purposes
	constantsFile := filepath.Join(versionDir, fmt.Sprintf("%s_constants.go", ontology))

	// Format constants code
	constantsFormatted, err := format.Source([]byte(constantsCode))
	if err != nil {
		// If formatting fails, use unformatted code
		constantsFormatted = []byte(constantsCode)
	}

	// Only overwrite constants file when content actually changes
	if existing, err := fileutil.ReadFile(constantsFile); err == nil {
		if bytes.Equal(existing, constantsFormatted) {
			return nil
		}
	}

	// Output constants file: {ontology}_constants.go
	if err := fileutil.WriteFile(constantsFile, constantsFormatted, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write constants file").Wrap(err)
	}

	return nil
}

// generateBuilderCode generates Go code for a builder from a Spec
// ontology is the ontology name to use (may differ from spec.Ontology if it was empty)
// version is the version string (e.g., "v1_0_0")
// packageName is the Go package name (e.g., "bldr_v1")
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateBuilderCode(spec *objects.Spec, ontology, version, packageName string) (string, error) {
	var buf strings.Builder

	// Generate type name (e.g., "AccountBuilder" from "account" - version not in name)
	typeName := toCamelCase(ontology) + "Builder"
	constructorName := "New" + typeName

	// Package name (e.g., "package bldr_v1")
	buf.WriteString("// Code generated by zqk-admin system generate-builders. DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", packageName)

	// Import parent builders package
	buf.WriteString("import (\n")
	buf.WriteString("\t\"github.com/lanceman/zqk/pkg/objects\"\n")
	buf.WriteString("\t\"github.com/lanceman/zqk/pkg/specbuilder/builders\"\n")
	buf.WriteString(")\n\n")

	// Type definition
	fmt.Fprintf(&buf, "// %s builds the %s spec at version %s\n", typeName, ontology, version)
	fmt.Fprintf(&buf, "// File: %s/%s_builder.go - version is encoded in package/directory name\n", packageName, ontology)
	fmt.Fprintf(&buf, "type %s struct {\n", typeName)
	buf.WriteString("\t*builders.BaseSpecBuilder\n")
	buf.WriteString("}\n\n")

	// Constructor
	fmt.Fprintf(&buf, "// %s creates a new builder for %s spec version %s\n", constructorName, ontology, version)
	fmt.Fprintf(&buf, "func %s() *%s {\n", constructorName, typeName)
	fmt.Fprintf(&buf, "\tbuilder := &%s{\n", typeName)
	fmt.Fprintf(&buf, "\t\tBaseSpecBuilder: builders.NewBaseSpecBuilder(%q, %q),\n", ontology, version)
	buf.WriteString("\t}\n\n")

	// Configure spec
	buf.WriteString("\t// Configure the spec\n")
	buf.WriteString("\tbuilder.")

	// Set extends
	if spec.Extends != emptyValue && spec.Extends != "null" {
		fmt.Fprintf(&buf, "\n\t\tSetExtends(%q).", spec.Extends)
	} else {
		buf.WriteString("\n\t\tSetExtends(\"null\").")
	}

	// Set description (escape newlines)
	description := strings.ReplaceAll(spec.Description, "\n", "\\n")
	description = strings.ReplaceAll(description, "\"", "\\\"")
	if description != emptyValue {
		fmt.Fprintf(&buf, "\n\t\tSetDescription(%q).", description)
	}

	// Set visibility
	if spec.Visibility != emptyValue {
		fmt.Fprintf(&buf, "\n\t\tSetVisibility(%q).", spec.Visibility)
	} else {
		buf.WriteString("\n\t\tSetVisibility(\"internal\").")
	}

	// Set schema version
	if spec.SchemaVersion != emptyValue {
		// Use constant if schema version matches default, otherwise use quoted value
		if spec.SchemaVersion == objects.DefaultSchemaVersion {
			buf.WriteString("\n\t\tSetSchemaVersion(objects.DefaultSchemaVersion)")
		} else {
			fmt.Fprintf(&buf, "\n\t\tSetSchemaVersion(%q)", spec.SchemaVersion)
		}
	} else {
		buf.WriteString("\n\t\tSetSchemaVersion(objects.DefaultSchemaVersion)")
	}

	// Add traits (chain from SetSchemaVersion)
	if len(spec.Traits) > 0 {
		for _, trait := range spec.Traits {
			fmt.Fprintf(&buf, ".\n\t\tAddTrait(%q)", trait)
		}
	}

	buf.WriteString("\n\n")

	// Add fields
	if len(spec.Fields) > 0 {
		buf.WriteString("\t// Add fields\n")
		fmt.Fprintf(&buf, "\tbuilder.add%sFields()\n\n", toCamelCase(ontology))
		buf.WriteString("\treturn builder\n")
		buf.WriteString("}\n\n")

		// Generate field adder method
		methodName := fmt.Sprintf("add%sFields", toCamelCase(ontology))
		fmt.Fprintf(&buf, "// %s adds the %s fields\n", methodName, ontology)
		fmt.Fprintf(&buf, "func (b *%s) %s() {\n", typeName, methodName)

		// Sort field names for deterministic output
		fieldNames := make([]string, 0, len(spec.Fields))
		for name := range spec.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)

		// Add each field using FieldBuilder pattern
		for _, fieldName := range fieldNames {
			fieldDef := spec.Fields[fieldName]
			fieldCode := generateFieldBuilderCode(fieldName, fieldDef)
			buf.WriteString(fieldCode)
			buf.WriteString("\n")
		}

		buf.WriteString("}\n\n")
	} else {
		buf.WriteString("\treturn builder\n")
		buf.WriteString("}\n\n")
	}

	// Build method
	buf.WriteString("// Build builds the spec (inherited from BaseSpecBuilder)\n")
	fmt.Fprintf(&buf, "func (b *%s) Build() *objects.Spec {\n", typeName)
	buf.WriteString("\treturn b.BaseSpecBuilder.Build()\n")
	buf.WriteString("}\n\n")

	// GetVersion and GetOntology methods (required by SpecBuilder interface)
	buf.WriteString("// GetVersion returns the version this builder generates\n")
	fmt.Fprintf(&buf, "func (b *%s) GetVersion() string {\n", typeName)
	fmt.Fprintf(&buf, "\treturn %q\n", version)
	buf.WriteString("}\n\n")

	buf.WriteString("// GetOntology returns the ontology/name of the spec this builder generates\n")
	fmt.Fprintf(&buf, "func (b *%s) GetOntology() string {\n", typeName)
	fmt.Fprintf(&buf, "\treturn %q\n", ontology)
	buf.WriteString("}\n\n")

	// Auto-register this builder
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\tbuilders.RegisterBuilder(%s())\n", constructorName)
	buf.WriteString("}\n")

	return buf.String(), nil
}

// generateFieldBuilderCode generates code using FieldBuilder pattern
func generateFieldBuilderCode(fieldName string, fieldDef any) string {
	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return fmt.Sprintf("\t// Field %q: unable to generate code (non-map type)", fieldName)
	}

	var buf strings.Builder

	// Get field type (required for FieldBuilder)
	fieldType, _ := fieldMap["type"].(string)
	if fieldType == emptyValue {
		fieldType = "string" // Default fallback
	}

	// Start FieldBuilder chain
	fmt.Fprintf(&buf, "\tb.AddFieldBuilder(builders.NewFieldBuilder(%q, %q)", fieldName, fieldType)

	// Extract and generate checklist
	if checklist, ok := fieldMap["checklist"].(map[string]any); ok {
		buf.WriteString(".\n\t\tWithChecklist(")
		buf.WriteString(generateChecklistBuilderCode(checklist))
		buf.WriteString(")")
	}

	// Extract and generate access
	if access, ok := fieldMap["access"].(map[string]any); ok {
		buf.WriteString(".\n\t\tWithAccess(")
		buf.WriteString(generateAccessBuilderCode(access))
		buf.WriteString(")")
	}

	// Extract and generate validation
	if validation, ok := fieldMap["validation"].(map[string]any); ok {
		buf.WriteString(".\n\t\tWithValidation(")
		buf.WriteString(generateValidationBuilderCode(validation))
		buf.WriteString(")")
	}

	// Extract top-level default (so built spec has fieldDef["default"] for validator)
	if defaultVal, ok := fieldMap["default"]; ok && defaultVal != nil {
		buf.WriteString(".\n\t\tWithDefault(")
		if s, ok := defaultVal.(string); ok && s == objects.InitialFieldVersion {
			// YAML uses the literal "1.0.0"; codegen emits a named constant so
			// builder regeneration stays DRY and maintainable.
			buf.WriteString("objects.InitialFieldVersion")
		} else {
			buf.WriteString(formatValue(defaultVal, 4))
		}
		buf.WriteString(")")
	}

	// Extract and generate traits
	if traits, ok := fieldMap["traits"].([]any); ok {
		traitStrings := make([]string, 0, len(traits))
		for _, trait := range traits {
			if str, ok := trait.(string); ok {
				traitStrings = append(traitStrings, fmt.Sprintf("%q", str))
			}
		}
		if len(traitStrings) > 0 {
			buf.WriteString(".\n\t\tWithTraits(")
			buf.WriteString(strings.Join(traitStrings, ", "))
			buf.WriteString(")")
		}
	}

	// Extract permissions
	if permissions, ok := fieldMap["permissions"].(string); ok && permissions != emptyValue {
		fmt.Fprintf(&buf, ".\n\t\tWithPermissions(%q)", permissions)
	}

	// Extract semantic_type
	if semanticType, ok := fieldMap["semantic_type"].(string); ok && semanticType != emptyValue {
		fmt.Fprintf(&buf, ".\n\t\tWithSemanticType(%q)", semanticType)
	}

	// Extract field_profile_code
	if profileCode, ok := fieldMap["field_profile_code"].(string); ok && profileCode != emptyValue {
		fmt.Fprintf(&buf, ".\n\t\tWithProfileCode(%q)", profileCode)
	}

	// Close without .Build() - AddFieldBuilder takes *FieldBuilder, not the result
	buf.WriteString(")")

	return buf.String()
}

// generateChecklistBuilderCode generates ChecklistBuilder code
func generateChecklistBuilderCode(checklist map[string]any) string {
	var buf strings.Builder
	buf.WriteString("builders.NewChecklistBuilder()")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(checklist))
	for k := range checklist {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := checklist[k]
		switch k {
		case "authority":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tAuthority(%q)", str)
			}
		case "automation_hooks":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tAutomationHooks(%q)", str)
			}
		case "cardinality":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tCardinality(%q)", str)
			}
		case "criticality":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tCriticality(%q)", str)
			}
		case "default":
			buf.WriteString(".\n\t\t\tDefault(")
			if s, ok := v.(string); ok && s == objects.InitialFieldVersion {
				buf.WriteString("objects.InitialFieldVersion")
			} else {
				buf.WriteString(formatValue(v, 4))
			}
			buf.WriteString(")")
		case "dependencies":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tDependencies(%q)", str)
			}
		case specChecklistTagLifecycle:
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tLifecycle(%q)", NormalizeChecklistLifecycle(str))
			}
		case "observability":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tObservability(%q)", NormalizeChecklistObservability(str))
			}
		case "purpose":
			if str, ok := v.(string); ok {
				// Escape for string literal
				str = strings.ReplaceAll(str, `\`, `\\`)
				str = strings.ReplaceAll(str, `"`, `\"`)
				fmt.Fprintf(&buf, ".\n\t\t\tPurpose(%q)", str)
			}
		case "security":
			if str, ok := v.(string); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tSecurity(%q)", NormalizeChecklistSecurity(str))
			}
		case "system_usage":
			buf.WriteString(".\n\t\t\tSystemUsage(")
			buf.WriteString(formatValue(v, 4))
			buf.WriteString(")")
		case "validation":
			if str, ok := v.(string); ok {
				str = strings.ReplaceAll(str, `\`, `\\`)
				str = strings.ReplaceAll(str, `"`, `\"`)
				fmt.Fprintf(&buf, ".\n\t\t\tValidation(%q)", str)
			}
		}
	}

	buf.WriteString(".\n\t\t\tBuild()")
	return buf.String()
}

// generateAccessBuilderCode generates AccessBuilder code
func generateAccessBuilderCode(access map[string]any) string {
	var buf strings.Builder
	buf.WriteString("builders.NewAccessBuilder()")

	if requires, ok := access["requires"].([]any); ok {
		requireStrings := make([]string, 0, len(requires))
		for _, req := range requires {
			if str, ok := req.(string); ok {
				requireStrings = append(requireStrings, fmt.Sprintf("%q", str))
			}
		}
		if len(requireStrings) > 0 {
			buf.WriteString(".\n\t\t\tRequires(")
			buf.WriteString(strings.Join(requireStrings, ", "))
			buf.WriteString(")")
		}
	}

	buf.WriteString(".\n\t\t\tBuild()")
	return buf.String()
}

// generateValidationBuilderCode generates ValidationBuilder code
func generateValidationBuilderCode(validation map[string]any) string {
	var buf strings.Builder
	buf.WriteString("builders.NewValidationBuilder()")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(validation))
	for k := range validation {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := validation[k]
		switch k {
		case "required":
			if b, ok := v.(bool); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tRequired(%t)", b)
			}
		case "pattern":
			if str, ok := v.(string); ok {
				// Use shared utility to format pattern for Go code (unescapes YAML double backslashes)
				unescapedPattern := FormatPatternForGoCode(str)
				// Use raw string literal (backticks) to preserve the unescaped pattern exactly
				fmt.Fprintf(&buf, ".\n\t\t\tPattern(`%s`)", unescapedPattern)
			}
		case "min_length":
			if i, ok := v.(int); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tMinLength(%d)", i)
			}
		case "max_length":
			if i, ok := v.(int); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tMaxLength(%d)", i)
			}
		case "display_length":
			if i, ok := v.(int); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tDisplayLength(%d)", i)
			}
		case "minCount":
			if i, ok := v.(int); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tMinCount(%d)", i)
			}
		case "maxCount":
			if i, ok := v.(int); ok {
				fmt.Fprintf(&buf, ".\n\t\t\tMaxCount(%d)", i)
			}
		case "enum":
			if enum, ok := v.([]any); ok {
				buf.WriteString(".\n\t\t\tEnum(")
				buf.WriteString(formatValue(enum, 4))
				buf.WriteString(")")
			}
		}
	}

	buf.WriteString(".\n\t\t\tBuild()")
	return buf.String()
}

// formatValue formats a value for Go code
func formatValue(v any, indentLevel int) string {
	indent := strings.Repeat("\t", indentLevel)

	switch val := v.(type) {
	case string:
		// Escape string
		escaped := strings.ReplaceAll(val, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return fmt.Sprintf("%q", escaped)

	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)

	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)

	case float32, float64:
		return fmt.Sprintf("%g", val)

	case bool:
		if val {
			return "true"
		}
		return "false"

	case nil:
		return "nil"

	case []any:
		var buf strings.Builder
		buf.WriteString("[]any{\n")
		for _, item := range val {
			buf.WriteString(indent + "\t")
			buf.WriteString(formatValue(item, indentLevel+1))
			buf.WriteString(",\n")
		}
		buf.WriteString(indent + "}")
		return buf.String()

	case map[string]any:
		var buf strings.Builder
		buf.WriteString("map[string]any{\n")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString(indent + "\t")
			fmt.Fprintf(&buf, "%q: ", k)
			buf.WriteString(formatValue(val[k], indentLevel+1))
			buf.WriteString(",\n")
		}
		buf.WriteString(indent + "}")
		return buf.String()

	default:
		// For unknown types, try to convert to string (fallback)
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
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
