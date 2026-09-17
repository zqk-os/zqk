package cliexamples

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

var ontologyStemPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// FieldValueGenerator is a function type for custom field value generation.
type FieldValueGenerator func(fieldName string, fieldDef map[string]any, kind string) (any, error)

// Generator generates dynamic CLI command examples from object specs.
//
// This is intended to back help output and other UX surfaces that want to show
// valid command syntax derived from specs (and later ontologies).
type Generator struct {
	specLoader    *objects.SpecLoader
	fieldRegistry *objects.FieldRegistry
	projectRoot   string
	templatesDir  string

	// Extensible configuration
	customFieldGenerators map[string]FieldValueGenerator // field name -> generator
	fieldOverrides        map[string]any                 // field name -> override value
	includeOptionalFields bool                           // whether to include optional fields
	excludedFields        map[string]bool                // fields to exclude

	// Template cache
	templateCache map[string]string // template name -> content
}

// Builder is a fluent builder for Generator configuration.
type Builder struct {
	g *Generator
}

// NewBuilder creates a new builder for CLI example generation.
func NewBuilder() (*Builder, error) {
	projectRoot, err := findProjectRoot(".")
	if err != nil {
		return nil, err
	}

	specLoader := objects.NewSpecLoader("")
	fieldRegistry := objects.GetGlobalFieldRegistry()

	templatesDir := filepath.Join(projectRoot, "cmd", "zqk", "internal", "templates", "cli_examples")

	g := &Generator{
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

	return &Builder{g: g}, nil
}

func (b *Builder) WithTemplatesDir(dir string) *Builder {
	b.g.templatesDir = dir
	return b
}

func (b *Builder) WithOptionalFields(include bool) *Builder {
	b.g.includeOptionalFields = include
	return b
}

func (b *Builder) WithFieldGenerator(fieldName string, gen FieldValueGenerator) *Builder {
	b.g.customFieldGenerators[fieldName] = gen
	return b
}

func (b *Builder) WithFieldGenerators(generators map[string]FieldValueGenerator) *Builder {
	maps.Copy(b.g.customFieldGenerators, generators)
	return b
}

func (b *Builder) WithFieldOverride(fieldName string, value any) *Builder {
	b.g.fieldOverrides[fieldName] = value
	return b
}

func (b *Builder) WithFieldOverrides(overrides map[string]any) *Builder {
	maps.Copy(b.g.fieldOverrides, overrides)
	return b
}

func (b *Builder) ExcludeField(fieldName string) *Builder {
	b.g.excludedFields[fieldName] = true
	return b
}

func (b *Builder) ExcludeFields(fieldNames ...string) *Builder {
	for _, name := range fieldNames {
		b.g.excludedFields[name] = true
	}
	return b
}

func (b *Builder) Build() *Generator {
	return b.g
}

// New creates a generator with default configuration.
func New() (*Generator, error) {
	builder, err := NewBuilder()
	if err != nil {
		return nil, err
	}
	return builder.Build(), nil
}

func findProjectRoot(startPath string) (string, error) {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}

	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir, nil
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errfmt.Errorf("could not find project root from %s", startPath)
}

// GenerateCLICommandExamples generates CLI command examples for a kind using templates on disk.
func (g *Generator) GenerateCLICommandExamples(kind string) ([]string, error) {
	exampleObj, err := g.GenerateExampleObject(kind, "")
	if err != nil {
		return nil, err
	}
	exampleID, _ := exampleObj[objects.FieldKeyID].(string)
	exampleYAML, err := g.GenerateYAMLExample(kind, exampleID)
	if err != nil {
		return nil, err
	}

	var yamlCommented strings.Builder
	for line := range strings.SplitSeq(exampleYAML, "\n") {
		if strings.TrimSpace(line) == emptyValue {
			yamlCommented.WriteString("#\n")
			continue
		}
		yamlCommented.WriteString("# ")
		yamlCommented.WriteString(line)
		yamlCommented.WriteString("\n")
	}

	vars := map[string]string{
		"cli":                    paths.CLICommandName,
		"cli_env_prefix":         toEnvPrefix(paths.CLICommandName),
		objects.FieldKeyKind:     kind,
		objects.FieldKeyID:       exampleID,
		"yaml_example":           exampleYAML,
		"yaml_example_commented": strings.TrimSpace(yamlCommented.String()),
	}

	templateNames := []string{
		"create_from_file.tmpl",
		"create_from_stdin.tmpl",
		"get.tmpl",
		"list.tmpl",
		"update.tmpl",
		"delete.tmpl",
	}

	var examples []string
	for _, name := range templateNames {
		tpl, err := g.loadTemplate(name)
		if err != nil {
			continue // best-effort
		}
		out := g.substituteTemplate(tpl, vars)
		examples = append(examples, strings.Split(out, "\n")...)
		if len(examples) > 0 && examples[len(examples)-1] != emptyValue {
			examples = append(examples, "")
		}
	}

	// Trim trailing blank lines
	for len(examples) > 0 && examples[len(examples)-1] == emptyValue {
		examples = examples[:len(examples)-1]
	}

	return examples, nil
}

func (g *Generator) loadTemplate(templateName string) (string, error) {
	if cached, ok := g.templateCache[templateName]; ok {
		return cached, nil
	}
	path := filepath.Join(g.templatesDir, templateName)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	g.templateCache[templateName] = content
	return content, nil
}

func (g *Generator) substituteTemplate(template string, vars map[string]string) string {
	out := template
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// GenerateYAMLExample renders an example object as YAML.
func (g *Generator) GenerateYAMLExample(kind, idOverride string) (string, error) {
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

// GenerateObjectSpecKindDraft returns a starter object_specs YAML document for a new kind.
// It loads extendsName.yaml with full inheritance and materializes effective top-level defaults
// (storage_profile, traits, exclude_traits, visibility) so authors see what the loader will merge.
// The leading comment block explains omission vs override; see DATA_CELL_MODEL.md (storage profiles).
func (g *Generator) GenerateObjectSpecKindDraft(ontology, extendsName string) (string, error) {
	ontology = strings.TrimSpace(ontology)
	if ontology == emptyValue {
		return "", errfmt.Errorf("ontology is required")
	}
	if !ontologyStemPattern.MatchString(ontology) {
		return "", errfmt.Errorf("ontology %q must match %s (lowercase stem, e.g. my_kind)", ontology, ontologyStemPattern.String())
	}
	extendsName = strings.TrimSpace(extendsName)
	if extendsName == emptyValue {
		extendsName = "base_object"
	}
	if !ontologyStemPattern.MatchString(extendsName) {
		return "", errfmt.Errorf("extends %q must match %s", extendsName, ontologyStemPattern.String())
	}

	parent, err := g.specLoader.LoadSpecWithInheritance(extendsName + ".yaml")
	if err != nil {
		return "", errfmt.Errorf("load extends spec %s.yaml: %w", extendsName, err)
	}
	if parent == nil {
		return "", errfmt.Errorf("nil spec for extends %q", extendsName)
	}

	schemaVersion := parent.SchemaVersion
	if schemaVersion == emptyValue {
		schemaVersion = "2.0.0"
	}
	visibility := strings.TrimSpace(parent.Visibility)
	if visibility == emptyValue {
		visibility = "internal"
	}

	comment := buildObjectSpecKindDraftComment(ontology, extendsName, parent)

	body := struct {
		SchemaVersion  string         `yaml:"schema_version"`
		Ontology       string         `yaml:"ontology"`
		Extends        string         `yaml:"extends"`
		Visibility     string         `yaml:"visibility"`
		StorageProfile string         `yaml:"storage_profile,omitempty"`
		Traits         []string       `yaml:"traits,omitempty"`
		ExcludeTraits  []string       `yaml:"exclude_traits,omitempty"`
		Description    string         `yaml:"description"`
		Fields         map[string]any `yaml:"fields"`
	}{
		SchemaVersion:  schemaVersion,
		Ontology:       ontology,
		Extends:        extendsName,
		Visibility:     visibility,
		StorageProfile: parent.StorageProfile,
		// Strip names already conferred by Includes so drafts do not teach
		// `base_object_traits` + `base_auditable_traits` as co-authored.
		// TRACK: TDE-CEF-TRAIT-INCLUDE-REDUNDANT-001
		Traits:        objects.NewTraitRegistry().StripRedundantIncludedTraits(parent.ResolvedTraits),
		ExcludeTraits: append([]string(nil), parent.ExcludeTraits...),
		Description:   "Describe this kind: purpose, lifecycle reference, and operational notes.\n",
		Fields:        map[string]any{},
	}

	yamlBytes, err := yaml.Marshal(&body)
	if err != nil {
		return "", errfmt.Newf("marshal object spec draft").Wrap(err)
	}

	return comment + string(yamlBytes), nil
}

func buildObjectSpecKindDraftComment(ontology, extendsName string, parent *objects.Spec) string {
	var b strings.Builder
	b.WriteString("# Object kind spec (draft) — save as .zqk/specs/objects/")
	b.WriteString(ontology)
	b.WriteString(".yaml\n")
	b.WriteString("#\n")
	b.WriteString("# Inheritance model (object_specs loader):\n")
	b.WriteString("# - `extends` points at the parent kind (YAML stem under object_specs/). Parent fields and traits\n")
	b.WriteString("#   merge into this kind; omit a field block here to inherit parent field definitions.\n")
	b.WriteString("# - Top-level keys you omit may still be resolved at load time from the parent chain (same as\n")
	b.WriteString("#   this draft's effective defaults were copied from the resolved `")
	b.WriteString(extendsName)
	b.WriteString("` spec).\n")
	b.WriteString("# - Override `storage_profile`, `traits`, or `exclude_traits` when this kind needs a different\n")
	b.WriteString("#   data-cell profile than the effective default below (e.g. ")
	b.WriteString(string(datacell.ProfileStream))
	b.WriteString(" for high-volume kinds listed in configs/high_volume_kinds.yaml).\n")
	b.WriteString("# - Remove a copied default key to rely on inheritance; set explicitly to lock a value.\n")
	b.WriteString("#\n")
	if parent != nil && parent.StorageProfile != emptyValue {
		b.WriteString("# Effective defaults below include storage_profile=")
		b.WriteString(parent.StorageProfile)
		b.WriteString(" from the resolved extends chain (see auditable → base_object for CAS-backed kinds).\n")
	} else {
		b.WriteString("# Effective storage_profile is unset on the parent chain; declare storage_profile when you\n")
		b.WriteString("# choose cas_entity, ")
		b.WriteString(string(datacell.ProfileLightFile))
		b.WriteString(", or ")
		b.WriteString(string(datacell.ProfileStream))
		b.WriteString(".\n")
	}
	b.WriteString("#\n\n")
	return b.String()
}

// GenerateExampleObject generates an example object for a kind.
func (g *Generator) GenerateExampleObject(kind string, idOverride string) (map[string]any, error) {
	spec, err := g.specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		return nil, errfmt.Errorf("failed to load spec for kind %s: %w", kind, err)
	}

	kindFields, err := g.fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}

	example := map[string]any{
		objects.FieldKeyKind: kind,
	}

	// ID
	if idOverride != emptyValue {
		example[objects.FieldKeyID] = idOverride
	} else {
		prefix := strings.ToUpper(kind)
		if len(kind) >= 3 {
			prefix = strings.ToUpper(kind[:3])
		}
		example[objects.FieldKeyID] = fmt.Sprintf("%s-001", prefix)
	}

	// Include required fields (and a small set of commonly useful fields).
	for _, f := range kindFields.AllFields {
		if g.excludedFields[f.Name] {
			continue
		}
		shouldInclude := f.Required || g.includeOptionalFields || isCommonlyUsedField(f.Name)
		if !shouldInclude {
			continue
		}
		fieldDefAny, ok := spec.ResolvedFields[f.Name]
		if !ok {
			continue
		}
		fieldDef, ok := fieldDefAny.(map[string]any)
		if !ok {
			continue
		}
		val, err := g.GenerateExampleValue(f.Name, fieldDef, kind)
		if err != nil {
			continue
		}
		if val != nil {
			example[f.Name] = val
		}
	}

	if spec.SchemaVersion != emptyValue {
		example[objects.FieldKeySchemaVersion] = spec.SchemaVersion
	}

	// Basic extensible_object defaults if present in spec
	if _, ok := spec.ResolvedFields[objects.FieldKeyDomain]; ok {
		if _, exists := example[objects.FieldKeyDomain]; !exists {
			example[objects.FieldKeyDomain] = "custom"
		}
	}
	if _, ok := spec.ResolvedFields[objects.FieldKeySpecInterpreter]; ok {
		if _, exists := example[objects.FieldKeySpecInterpreter]; !exists {
			example[objects.FieldKeySpecInterpreter] = "default"
		}
	}
	if _, ok := spec.ResolvedFields[objects.FieldKeySpecContextBroker]; ok {
		if _, exists := example[objects.FieldKeySpecContextBroker]; !exists {
			example[objects.FieldKeySpecContextBroker] = "default"
		}
	}

	return example, nil
}

// GenerateExampleValue generates an example value for a field based on its spec.
func (g *Generator) GenerateExampleValue(fieldName string, fieldDef map[string]any, kind string) (any, error) {
	if override, ok := g.fieldOverrides[fieldName]; ok {
		return override, nil
	}
	if gen, ok := g.customFieldGenerators[fieldName]; ok {
		return gen(fieldName, fieldDef, kind)
	}
	return generateDefaultValue(fieldName, fieldDef, kind), nil
}

func generateDefaultValue(fieldName string, fieldDef map[string]any, kind string) any {
	fieldType, _ := fieldDef[objects.FieldKeyType].(string)

	// enum
	if validation, ok := fieldDef["validation"].(map[string]any); ok {
		if enumValues, ok := validation["enum"].([]any); ok && len(enumValues) > 0 {
			return enumValues[0]
		}
	}

	switch fieldType {
	case "string":
		switch {
		case strings.Contains(fieldName, "title") || strings.Contains(fieldName, "name"):
			return "Example " + fieldName
		case strings.Contains(fieldName, "description"):
			return "Example description"
		case fieldName == "status":
			return "draft"
		default:
			return "example_value"
		}
	case "list", "array":
		return []any{}
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "object":
		return map[string]any{}
	default:
		_ = kind
		return nil
	}
}

func isCommonlyUsedField(fieldName string) bool {
	switch fieldName {
	case "title", "status", "description":
		return true
	default:
		return false
	}
}

func toEnvPrefix(cliName string) string {
	if cliName == emptyValue {
		return "ZQK"
	}
	cliName = strings.TrimSpace(cliName)
	var b strings.Builder
	b.Grow(len(cliName))
	lastUnderscore := false

	for _, r := range cliName {
		if r >= 'a' && r <= 'z' {
			r = r - 'a' + 'A'
		}
		isAZ := r >= 'A' && r <= 'Z'
		is09 := r >= '0' && r <= '9'
		if isAZ || is09 {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	out := strings.Trim(b.String(), "_")
	if out == emptyValue {
		return "ZQK"
	}
	return out
}
