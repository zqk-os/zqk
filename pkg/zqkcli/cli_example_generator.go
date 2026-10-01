package internal

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

// FieldValueGenerator is a function type for custom field value generation
type FieldValueGenerator func(fieldName string, fieldDef map[string]any, kind string) (any, error)

// CLIExampleGenerator generates dynamic CLI command examples from object specs
type CLIExampleGenerator struct {
	specLoader    *objects.SpecLoader
	fieldRegistry *objects.FieldRegistry
	projectRoot   string
	templatesDir  string // Directory containing CLI command templates

	// Extensible configuration
	customFieldGenerators map[string]FieldValueGenerator // field name -> generator
	fieldOverrides        map[string]any                 // field name -> override value
	includeOptionalFields bool                           // whether to include optional fields
	excludedFields        map[string]bool                // fields to exclude

	// Template cache
	templateCache map[string]string // template name -> content
}

// ExampleBuilder provides a fluent API for building example generators with extensible configuration
//
// Usage:
//
//	builder, _ := NewExampleBuilder()
//	generator := builder.
//	    WithFieldGenerator("component_type", customGenerator).
//	    WithFieldOverride("title", "My Title").
//	    WithOptionalFields(true).
//	    ExcludeField("updated_by").
//	    Build()
//
// This builder pattern allows for extensible maintainability:
//   - Custom field generators can be registered for any field
//   - Field values can be overridden
//   - Optional fields can be included/excluded
//   - Fields can be excluded from examples
//   - All configuration is chainable and composable
type ExampleBuilder struct {
	generator *CLIExampleGenerator
}

// findGoModRoot returns the directory containing go.mod by walking up from startPath.
// Used so template loading works when ZQK_TEST_ROOT points at a test temp dir that has no templates.
func findGoModRoot(startPath string) string {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// NewExampleBuilder creates a new builder for CLI example generation
func NewExampleBuilder() (*ExampleBuilder, error) {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("could not find project root")
	}

	specLoader := objects.NewSpecLoader("")
	fieldRegistry := objects.GetGlobalFieldRegistry()

	// Templates live in source tree; when ZQK_TEST_ROOT is set, use go.mod root for template dir
	templatesDir := filepath.Join(projectRoot, "pkg", "zqkcli", "templates", "cli_examples")
	if _, err := fileutil.Stat(templatesDir); err != nil {
		if goModRoot := findGoModRoot("."); goModRoot != emptyValue {
			templatesDir = filepath.Join(goModRoot, "pkg", "zqkcli", "templates", "cli_examples")
		}
	}

	generator := &CLIExampleGenerator{
		specLoader:            specLoader,
		fieldRegistry:         fieldRegistry,
		projectRoot:           projectRoot,
		templatesDir:          templatesDir,
		customFieldGenerators: make(map[string]FieldValueGenerator),
		fieldOverrides:        make(map[string]any),
		includeOptionalFields: false,
		excludedFields:        make(map[string]bool),
		templateCache:         make(map[string]string),
	}

	return &ExampleBuilder{generator: generator}, nil
}

// WithFieldGenerator registers a custom generator for a specific field
// The generator function receives the field name, field definition, and object kind
// and returns an example value for that field
func (b *ExampleBuilder) WithFieldGenerator(fieldName string, generator FieldValueGenerator) *ExampleBuilder {
	b.generator.customFieldGenerators[fieldName] = generator
	return b
}

// WithFieldGenerators registers multiple custom generators at once
func (b *ExampleBuilder) WithFieldGenerators(generators map[string]FieldValueGenerator) *ExampleBuilder {
	maps.Copy(b.generator.customFieldGenerators, generators)
	return b
}

// WithFieldOverride sets an override value for a specific field
// Overrides take precedence over generators
func (b *ExampleBuilder) WithFieldOverride(fieldName string, value any) *ExampleBuilder {
	b.generator.fieldOverrides[fieldName] = value
	return b
}

// WithFieldOverrides sets multiple field overrides at once
func (b *ExampleBuilder) WithFieldOverrides(overrides map[string]any) *ExampleBuilder {
	maps.Copy(b.generator.fieldOverrides, overrides)
	return b
}

// WithTemplatesDir sets a custom directory for CLI command templates
func (b *ExampleBuilder) WithTemplatesDir(dir string) *ExampleBuilder {
	b.generator.templatesDir = dir
	return b
}

// WithOptionalFields includes optional fields in generated examples
func (b *ExampleBuilder) WithOptionalFields(include bool) *ExampleBuilder {
	b.generator.includeOptionalFields = include
	return b
}

// ExcludeField excludes a field from generated examples
func (b *ExampleBuilder) ExcludeField(fieldName string) *ExampleBuilder {
	b.generator.excludedFields[fieldName] = true
	return b
}

// ExcludeFields excludes multiple fields from generated examples
func (b *ExampleBuilder) ExcludeFields(fieldNames ...string) *ExampleBuilder {
	for _, name := range fieldNames {
		b.generator.excludedFields[name] = true
	}
	return b
}

// Build creates the CLIExampleGenerator with the configured options
func (b *ExampleBuilder) Build() *CLIExampleGenerator {
	return b.generator
}

// NewCLIExampleGenerator creates a new CLI example generator with default configuration
func NewCLIExampleGenerator() (*CLIExampleGenerator, error) {
	builder, err := NewExampleBuilder()
	if err != nil {
		return nil, err
	}
	return builder.Build(), nil
}

// GenerateExampleValue generates an example value for a field based on its spec
// This method supports extensibility through custom generators and overrides
func (g *CLIExampleGenerator) GenerateExampleValue(fieldName string, fieldDef map[string]any, kind string) (any, error) {
	// Check for override first
	if override, ok := g.fieldOverrides[fieldName]; ok {
		return override, nil
	}

	// Check for custom generator
	if generator, ok := g.customFieldGenerators[fieldName]; ok {
		return generator(fieldName, fieldDef, kind)
	}

	// Fall back to default generation
	return g.generateDefaultValue(fieldName, fieldDef, kind)
}

// generateDefaultValue generates a default example value for a field
func (g *CLIExampleGenerator) generateDefaultValue(fieldName string, fieldDef map[string]any, kind string) (any, error) {
	// Get field type
	fieldType, _ := fieldDef[objects.FieldKeyType].(string)

	// Check for enum values first
	if validation, ok := fieldDef["validation"].(map[string]any); ok {
		if enumValues, ok := validation["enum"].([]any); ok && len(enumValues) > 0 {
			// Return first enum value as example
			return enumValues[0], nil
		}
	}

	// Generate based on type
	switch fieldType {
	case "string":
		// Check for pattern to generate appropriate example
		if validation, ok := fieldDef["validation"].(map[string]any); ok {
			if pattern, ok := validation["pattern"].(string); ok {
				// Generate example based on pattern
				if strings.Contains(pattern, "COMP-") {
					return "COMP-001", nil
				} else if strings.Contains(pattern, "BLI-") {
					return "BLI-001", nil
				} else if strings.Contains(pattern, "^[A-Z]+-\\d") {
					// Generic ID pattern - use kind prefix
					prefix := strings.ToUpper(kind[:3])
					if len(kind) < 3 {
						prefix = strings.ToUpper(kind)
					}
					return fmt.Sprintf("%s-001", prefix), nil
				}
			}
		}
		// Default string examples based on field name
		if strings.Contains(fieldName, "title") || strings.Contains(fieldName, "name") {
			return "Example " + strings.Title(fieldName), nil
		}
		if strings.Contains(fieldName, "description") {
			return "Example description for " + fieldName, nil
		}
		if strings.Contains(fieldName, "status") {
			return "draft", nil // Common initial status
		}
		return "example_value", nil

	case "list":
		// Return empty list as example
		return []string{}, nil

	case "integer", "number":
		return 0, nil

	case "boolean":
		return false, nil

	case "object":
		return map[string]any{}, nil

	default:
		return nil, nil
	}
}

// GenerateExampleObject generates a complete example object for a kind
func (g *CLIExampleGenerator) GenerateExampleObject(kind string, idOverride string) (map[string]any, error) {
	// Load spec
	specFile := kind + ".yaml"
	spec, err := g.specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		return nil, errfmt.Newf("failed to load spec for kind %s", kind).Wrap(err)
	}

	// Get fields
	kindFields, err := g.fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Newf("failed to get fields for kind %s", kind).Wrap(err)
	}

	// Build example object
	exampleObj := make(map[string]any)

	// Set kind
	exampleObj[objects.FieldKeyKind] = kind

	// Set ID (use override if provided, otherwise generate)
	if idOverride != emptyValue {
		exampleObj[objects.FieldKeyID] = idOverride
	} else {
		// Generate ID based on kind
		prefix := strings.ToUpper(kind[:3])
		if len(kind) < 3 {
			prefix = strings.ToUpper(kind)
		}
		exampleObj[objects.FieldKeyID] = fmt.Sprintf("%s-001", prefix)
	}

	// Generate values for all fields
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		// Skip if excluded
		if g.excludedFields[field.Name] {
			continue
		}

		// Skip if already set
		if _, exists := exampleObj[field.Name]; exists {
			continue
		}

		// Get field definition from spec
		fieldDef, ok := spec.ResolvedFields[field.Name]
		if !ok {
			continue
		}

		fieldDefMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}

		// Generate example value
		exampleValue, err := g.GenerateExampleValue(field.Name, fieldDefMap, kind)
		if err != nil {
			continue
		}

		// Set if not nil and (required, commonly used, or optional fields are included)
		shouldInclude := field.Required || g.isCommonlyUsedField(field.Name) || g.includeOptionalFields
		if exampleValue != nil && shouldInclude {
			exampleObj[field.Name] = exampleValue
		}
	}

	// Set schema version from spec
	exampleObj[objects.FieldKeySchemaVersion] = spec.SchemaVersion

	// For extensible objects, extract domain and set interpreter/broker
	if domain, err := g.extractDomainFromSpec(kind); err == nil && domain != emptyValue {
		exampleObj[objects.FieldKeyDomain] = domain
		exampleObj[objects.FieldKeySpecInterpreter] = fmt.Sprintf("%s_interpreter", domain)
		exampleObj[objects.FieldKeySpecContextBroker] = fmt.Sprintf("%s_broker", domain)
	}

	return exampleObj, nil
}

// extractDomainFromSpec extracts the domain value from a spec file
func (g *CLIExampleGenerator) extractDomainFromSpec(kind string) (string, error) {
	specsDir := filepath.Join(g.projectRoot, paths.ProcessInternalObjectSpecsDir)
	specFile := kind + ".yaml"
	rawSpecPath := filepath.Join(specsDir, specFile)

	rawData, err := fileutil.ReadFile(rawSpecPath)
	if err != nil {
		return "", err
	}

	var rawSpec map[string]any
	if err := yaml.Unmarshal(rawData, &rawSpec); err != nil {
		return "", err
	}

	domain, _ := rawSpec[objects.FieldKeyDomain].(string)
	return domain, nil
}

// isCommonlyUsedField checks if a field is commonly used in examples
func (g *CLIExampleGenerator) isCommonlyUsedField(fieldName string) bool {
	commonFields := []string{
		"title", "description", "status", "priority", "category",
		"component_type", "domain", "spec_interpreter", "spec_context_broker",
	}
	for _, common := range commonFields {
		if strings.EqualFold(fieldName, common) {
			return true
		}
	}
	return false
}

// GenerateYAMLExample generates a YAML string representation of an example object
func (g *CLIExampleGenerator) GenerateYAMLExample(kind, idOverride string) (string, error) {
	obj, err := g.GenerateExampleObject(kind, idOverride)
	if err != nil {
		return "", err
	}

	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// loadTemplate loads a template file from disk (with caching)
func (g *CLIExampleGenerator) loadTemplate(templateName string) (string, error) {
	// Check cache first
	if cached, ok := g.templateCache[templateName]; ok {
		return cached, nil
	}

	// Load from disk
	templatePath := filepath.Join(g.templatesDir, templateName+".tmpl")
	data, err := fileutil.ReadFile(templatePath)
	if err != nil {
		return "", errfmt.Newf("failed to load template %s", templateName).Wrap(err)
	}

	content := string(data)
	// Cache the template
	g.templateCache[templateName] = content
	return content, nil
}

// substituteTemplate replaces template variables with actual values
func (g *CLIExampleGenerator) substituteTemplate(template string, vars map[string]string) string {
	merged := map[string]string{
		"cli_env_prefix": brand.EnvPrefix(),
	}
	maps.Copy(merged, vars)
	result := template
	for key, value := range merged {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// GenerateCLICommandExamples generates CLI command examples for a kind using templates from disk
func (g *CLIExampleGenerator) GenerateCLICommandExamples(kind string) ([]string, error) {
	var examples []string

	// Generate example object
	exampleObj, err := g.GenerateExampleObject(kind, "")
	if err != nil {
		return nil, err
	}

	exampleID, _ := exampleObj[objects.FieldKeyID].(string)
	exampleYAML, err := g.GenerateYAMLExample(kind, exampleID)
	if err != nil {
		return nil, err
	}

	// Prepare YAML with comment prefix for file example
	var yamlCommented strings.Builder
	for line := range strings.SplitSeq(exampleYAML, "\n") {
		if line != emptyValue {
			yamlCommented.WriteString("# " + line + "\n")
		}
	}

	// Template variables
	vars := map[string]string{
		"cli":                    paths.CLICommandName,
		objects.FieldKeyKind:     kind,
		objects.FieldKeyID:       exampleID,
		"yaml_example":           exampleYAML,
		"yaml_example_commented": strings.TrimSpace(yamlCommented.String()),
	}

	// Load and render templates
	templates := []string{
		"create_from_file",
		"create_from_stdin",
		"list",
		"get",
		"update",
		"delete",
	}

	for _, templateName := range templates {
		template, err := g.loadTemplate(templateName)
		if err != nil {
			// If template doesn't exist, skip it (graceful degradation)
			continue
		}

		rendered := g.substituteTemplate(template, vars)
		for line := range strings.SplitSeq(rendered, "\n") {
			if line != emptyValue || len(examples) == 0 || examples[len(examples)-1] != emptyValue {
				examples = append(examples, line)
			}
		}
		// Add separator between sections
		if len(examples) > 0 && examples[len(examples)-1] != emptyValue {
			examples = append(examples, "")
		}
	}

	return examples, nil
}

// GenerateGraphCLICommandExamples generates CLI command examples for graph backend
func (g *CLIExampleGenerator) GenerateGraphCLICommandExamples(kind string) ([]string, error) {
	var examples []string

	// Generate example object
	exampleObj, err := g.GenerateExampleObject(kind, "")
	if err != nil {
		return nil, err
	}

	exampleID, _ := exampleObj[objects.FieldKeyID].(string)
	exampleYAML, err := g.GenerateYAMLExample(kind, exampleID)
	if err != nil {
		return nil, err
	}

	// Prepare YAML with comment prefix for file example
	var yamlCommented strings.Builder
	for line := range strings.SplitSeq(exampleYAML, "\n") {
		if line != emptyValue {
			yamlCommented.WriteString("# " + line)
		}
		yamlCommented.WriteString("\n")
	}

	// Template variables
	vars := map[string]string{
		"cli":                    paths.CLICommandName,
		objects.FieldKeyKind:     kind,
		objects.FieldKeyID:       exampleID,
		"yaml_example":           exampleYAML,
		"yaml_example_commented": strings.TrimSpace(yamlCommented.String()),
	}

	// Load and render graph-specific templates
	templates := []string{
		"graph_setup",
		"graph_create_from_file",
		"graph_query",
	}

	for _, templateName := range templates {
		template, err := g.loadTemplate(templateName)
		if err != nil {
			// If template doesn't exist, skip it (graceful degradation)
			continue
		}

		rendered := g.substituteTemplate(template, vars)
		for line := range strings.SplitSeq(rendered, "\n") {
			if line != emptyValue || len(examples) == 0 || examples[len(examples)-1] != emptyValue {
				examples = append(examples, line)
			}
		}
		// Add separator between sections
		if len(examples) > 0 && examples[len(examples)-1] != emptyValue {
			examples = append(examples, "")
		}
	}

	return examples, nil
}

// GenerateBootstrapScriptExample generates a bootstrap script example using templates from disk
func (g *CLIExampleGenerator) GenerateBootstrapScriptExample(kind string, count int) ([]string, error) {
	// Load bootstrap script template
	template, err := g.loadTemplate("bootstrap_script")
	if err != nil {
		return nil, errfmt.Newf("failed to load bootstrap script template").Wrap(err)
	}

	// Generate example objects
	exampleObj, err := g.GenerateExampleObject(kind, "")
	if err != nil {
		return nil, err
	}

	exampleID, _ := exampleObj[objects.FieldKeyID].(string)

	// Generate root YAML
	rootID := strings.Replace(exampleID, "001", "ROOT-001", 1)
	rootYAML, err := g.GenerateYAMLExample(kind, rootID)
	if err != nil {
		return nil, errfmt.Newf("failed to generate root YAML").Wrap(err)
	}

	// Generate child YAML template (with placeholder for ID)
	childID := strings.Replace(exampleID, "001", "CHILD-001", 1)
	childYAML, err := g.GenerateYAMLExample(kind, childID)
	if err != nil {
		return nil, errfmt.Newf("failed to generate child YAML").Wrap(err)
	}

	// Replace child ID with shell variable pattern and indent for heredoc
	childYAMLTemplate := strings.ReplaceAll(childYAML, childID, "${kind}-CHILD-00$i")
	// Indent each line for proper heredoc formatting
	childYAMLLines := strings.Split(childYAMLTemplate, "\n")
	var indentedChildYAML strings.Builder
	for _, line := range childYAMLLines {
		if line != emptyValue {
			indentedChildYAML.WriteString("  " + line)
		}
		indentedChildYAML.WriteString("\n")
	}
	childYAMLTemplate = strings.TrimRight(indentedChildYAML.String(), "\n")

	// Prepare template variables (include cli so {cli} in template is substituted)
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	vars := map[string]string{
		"cli":                 cliCmd,
		objects.FieldKeyKind:  kind,
		"count":               fmt.Sprintf("%d", count),
		"root_yaml":           rootYAML,
		"child_yaml_template": childYAMLTemplate,
	}

	// Substitute template variables
	rendered := g.substituteTemplate(template, vars)

	// Split into lines and return
	lines := strings.Split(rendered, "\n")
	// Remove trailing empty lines
	for len(lines) > 0 && lines[len(lines)-1] == emptyValue {
		lines = lines[:len(lines)-1]
	}

	return lines, nil
}

// GenerateGraphBootstrapScriptExample generates a bootstrap script example for graph backend using templates from disk
func (g *CLIExampleGenerator) GenerateGraphBootstrapScriptExample(kind string, count int) ([]string, error) {
	// Load graph bootstrap script template
	template, err := g.loadTemplate("graph_bootstrap_script")
	if err != nil {
		return nil, errfmt.Newf("failed to load graph bootstrap script template").Wrap(err)
	}

	// Generate example objects
	exampleObj, err := g.GenerateExampleObject(kind, "")
	if err != nil {
		return nil, err
	}

	exampleID, _ := exampleObj[objects.FieldKeyID].(string)

	// Generate root YAML
	rootID := strings.Replace(exampleID, "001", "ROOT-001", 1)
	rootYAML, err := g.GenerateYAMLExample(kind, rootID)
	if err != nil {
		return nil, errfmt.Newf("failed to generate root YAML").Wrap(err)
	}

	// Generate child YAML template (with placeholder for ID)
	childID := strings.Replace(exampleID, "001", "CHILD-001", 1)
	childYAML, err := g.GenerateYAMLExample(kind, childID)
	if err != nil {
		return nil, errfmt.Newf("failed to generate child YAML").Wrap(err)
	}

	// Replace child ID with shell variable pattern and indent for heredoc
	childYAMLTemplate := strings.ReplaceAll(childYAML, childID, "${kind}-CHILD-00$i")
	// Indent each line for proper heredoc formatting
	childYAMLLines := strings.Split(childYAMLTemplate, "\n")
	var indentedChildYAML strings.Builder
	for _, line := range childYAMLLines {
		if line != emptyValue {
			indentedChildYAML.WriteString("  " + line)
		}
		indentedChildYAML.WriteString("\n")
	}
	childYAMLTemplate = strings.TrimRight(indentedChildYAML.String(), "\n")

	// Prepare template variables (include cli so {cli} in template is substituted)
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	vars := map[string]string{
		"cli":                 cliCmd,
		objects.FieldKeyKind:  kind,
		"count":               fmt.Sprintf("%d", count),
		"root_yaml":           rootYAML,
		"child_yaml_template": childYAMLTemplate,
	}

	// Substitute template variables
	rendered := g.substituteTemplate(template, vars)

	// Split into lines and return
	lines := strings.Split(rendered, "\n")
	// Remove trailing empty lines
	for len(lines) > 0 && lines[len(lines)-1] == emptyValue {
		lines = lines[:len(lines)-1]
	}

	return lines, nil
}
