package instance_builders

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

// defaultInstanceVersion is the default schema version for instance builders
// Uses objects.DefaultSchemaVersion constant
var defaultInstanceVersion = objects.DefaultSchemaVersion

const (
	emptyValue                               = ""
	defaultDirectoryPerm   fileutil.FileMode = paths.DirPerm755
	defaultFilePerm        fileutil.FileMode = paths.FilePerm600
	codegenSourcePath                        = "pkg/specbuilder/instance_builders/codegen.go"
	codegenSpecFileExtYAML                   = ".yaml"
)

// systemFields holds both the order and lookup map for system fields
type systemFields struct {
	order  []string
	lookup map[string]bool
}

// getSystemFields returns the system fields object (single source of truth)
var getSystemFields = func() *systemFields {
	order := []string{
		objects.FieldKeyID,
		objects.FieldKeyKind,
		objects.FieldKeySchemaVersion,
		objects.FieldKeyCreatedAt,
		objects.FieldKeyCreatedBy,
		objects.FieldKeyUpdatedAt,
		objects.FieldKeyUpdatedBy,
		objects.FieldKeyNamespaceID,
		objects.FieldKeyStatus,
	}
	lookup := make(map[string]bool, len(order))
	for _, field := range order {
		lookup[field] = true
	}
	return &systemFields{
		order:  order,
		lookup: lookup,
	}
}()

// systemFieldOrder returns the canonical order of system fields (always first)
func systemFieldOrder() []string {
	return getSystemFields.order
}

// isSystemField checks if a field is a system field
func isSystemField(fieldName string) bool {
	return getSystemFields.lookup[fieldName]
}

const (
	generatedInstancePackage = "bldr_instance_v1"
	generatedEnumPackage     = "bldr_enum_v1"
	// DefaultInstanceBuilderToolDir is the kernel generator directory.
	// A pack passes its own instance_builders directory instead.
	DefaultInstanceBuilderToolDir = "pkg/specbuilder/instance_builders"
)

// InstanceBuilderOutput is where generated instance builders are written.
// outputDir is the generator tool directory (…/instance_builders). The files
// land in the sibling package, so a pack passes its own tool directory and
// the kernel default stays pkg/specbuilder/instance_builders.
func InstanceBuilderOutput(outputDir string) (dir, packageName string) {
	packageName = generatedInstancePackage
	dir = filepath.Join(filepath.Dir(outputDir), packageName)
	return dir, packageName
}

// EnumOutput is the sibling enum root for the same tool directory.
func EnumOutput(outputDir string) string {
	return filepath.Join(filepath.Dir(outputDir), generatedEnumPackage)
}

// EnumImportBase is the Go import prefix for enums written beside outputDir.
// The kernel tool directory keeps github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1.
// A relative tool directory inside this module imports its sibling enum tree.
// An absolute or out-of-module directory keeps the kernel enum import so scratch
// generation still compiles against the enums that ship with the kernel.
func EnumImportBase(outputDir string) string {
	enumDir := filepath.Clean(EnumOutput(outputDir))
	kernelEnum := filepath.Clean(filepath.Join(filepath.Dir(DefaultInstanceBuilderToolDir), generatedEnumPackage))
	if enumDir == kernelEnum || !moduleRelativeDir(enumDir) {
		return enumModuleBasePath
	}
	return kernelModulePath + "/" + filepath.ToSlash(enumDir)
}

func moduleRelativeDir(dir string) bool {
	if dir == "" || dir == "." || filepath.IsAbs(dir) || strings.HasPrefix(dir, "..") {
		return false
	}
	return true
}

// GenerateInstanceBuilderFromSpec reads a spec file and generates an instance builder
// This creates ONE builder per object type (not per instance)
func GenerateInstanceBuilderFromSpec(specPath, outputDir string, schemaVersion string) error {
	// Generate enum package first so generated builders can import typed enums.
	if err := generateEnumsForSpec(specPath, outputDir); err != nil {
		return errfmt.Newf("failed to generate enums").Wrap(err)
	}

	// Read spec file
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		return errfmt.Newf("failed to read spec file").Wrap(err)
	}

	// Parse spec
	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return errfmt.Newf("failed to parse spec").Wrap(err)
	}

	// Determine ontology
	ontology := spec.Ontology
	if ontology == emptyValue {
		baseName := filepath.Base(specPath)
		ontology = strings.TrimSuffix(baseName, ".yaml")
		ontology = strings.TrimSuffix(ontology, ".yml")
	}

	// Use schema version from spec if not provided
	if schemaVersion == emptyValue {
		if spec.SchemaVersion != emptyValue {
			schemaVersion = spec.SchemaVersion
		} else {
			schemaVersion = defaultInstanceVersion
		}
	}

	// Load resolved spec so generated fluent methods include inherited fields.
	specForGeneration := &spec
	if ontology != emptyValue {
		if resolvedSpec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance(codegenSpecFileNameForOntology(ontology)); err == nil && resolvedSpec != nil {
			specForGeneration = resolvedSpec
			if specForGeneration.Ontology == emptyValue {
				specForGeneration.Ontology = ontology
			}
			if specForGeneration.SchemaVersion == emptyValue {
				specForGeneration.SchemaVersion = schemaVersion
			}
		}
	}

	// Generate field order: system fields first, then spec fields
	fieldOrder := buildFieldOrder(specForGeneration)

	versionDir, packageName := InstanceBuilderOutput(outputDir)
	code, err := generateInstanceBuilderCode(specForGeneration, ontology, fieldOrder, packageName, EnumImportBase(outputDir))
	if err != nil {
		return errfmt.Newf("failed to generate code").Wrap(err)
	}

	// Format code
	formatted, err := format.Source([]byte(code))
	if err != nil {
		// If formatting fails, use unformatted code (but log warning)
		formatted = []byte(code)
	}

	if err := fileutil.MkdirAll(versionDir, defaultDirectoryPerm); err != nil {
		return errfmt.Newf("failed to create version directory").Wrap(err)
	}

	// Write file
	filename := fmt.Sprintf("%s_instance_builder.go", ontology)
	outputPath := filepath.Join(versionDir, filename)
	if err := fileutil.WriteFile(outputPath, formatted, defaultFilePerm); err != nil {
		return errfmt.Newf("failed to write builder file").Wrap(err)
	}

	return nil
}

// buildFieldOrder creates the canonical field order for sequence format
// System fields first, then spec-defined fields
func buildFieldOrder(spec *objects.Spec) []string {
	fields := specFieldMap(spec)

	// Start with system fields (constant order)
	order := make([]string, 0, len(systemFieldOrder())+len(fields))
	order = append(order, systemFieldOrder()...)

	// Add spec-defined fields in sorted order
	fieldNames := make([]string, 0, len(fields))
	for fieldName := range fields {
		// Skip system fields that are already in order
		if !isSystemField(fieldName) {
			fieldNames = append(fieldNames, fieldName)
		}
	}
	sort.Strings(fieldNames)
	order = append(order, fieldNames...)

	return order
}

type goImport struct {
	alias string
	path  string
}

type goImportBuilder struct {
	imports []goImport
}

func (b *goImportBuilder) Include(path string) *goImportBuilder {
	b.imports = append(b.imports, goImport{path: path})
	return b
}

func (b *goImportBuilder) IncludeWithAlias(alias, path string) *goImportBuilder {
	b.imports = append(b.imports, goImport{alias: alias, path: path})
	return b
}

type goFileBuilder struct {
	buf strings.Builder
}

func NewGoFile(_ string) *goFileBuilder {
	return &goFileBuilder{}
}

func generatedHeaderLines(source string) []string {
	return []string{
		fmt.Sprintf("// Code generated by %s from %s. DO NOT EDIT.", codegenSourcePath, source),
		fmt.Sprintf("// DO NOT EDIT. Regenerate with: %s", paths.CLIUsage("system", "generate-instance-builders", "--overwrite")),
	}
}

func codegenSpecFileNameForOntology(ontology string) string {
	return ontology + codegenSpecFileExtYAML
}

func (f *goFileBuilder) WithHeader(lines ...string) *goFileBuilder {
	for _, line := range lines {
		f.buf.WriteString(line + "\n")
	}
	f.buf.WriteString("\n")
	return f
}

func (f *goFileBuilder) WithPackage(pkg string) *goFileBuilder {
	f.buf.WriteString("package " + pkg + "\n\n")
	return f
}

func (f *goFileBuilder) WithImports(configure func(*goImportBuilder)) *goFileBuilder {
	ib := &goImportBuilder{}
	configure(ib)
	f.buf.WriteString("import (\n")
	for _, imp := range ib.imports {
		if imp.alias != emptyValue {
			fmt.Fprintf(&f.buf, "\t%s %q\n", imp.alias, imp.path)
			continue
		}
		fmt.Fprintf(&f.buf, "\t%q\n", imp.path)
	}
	f.buf.WriteString(")\n\n")
	return f
}

type goDefinitionsBuilder struct {
	file *goFileBuilder
}

func (f *goFileBuilder) WithDefinitions() *goDefinitionsBuilder {
	return &goDefinitionsBuilder{file: f}
}

func (d *goDefinitionsBuilder) WithComment(lines ...string) *goDefinitionsBuilder {
	for _, line := range lines {
		d.file.buf.WriteString("// " + line + "\n")
	}
	return d
}

func (d *goDefinitionsBuilder) WithStruct(name string, fields ...string) *goDefinitionsBuilder {
	fmt.Fprintf(&d.file.buf, "type %s struct {\n", name)
	for _, field := range fields {
		d.file.buf.WriteString("\t" + field + "\n")
	}
	d.file.buf.WriteString("}\n\n")
	return d
}

type goFuncBodyBuilder struct {
	buf    *strings.Builder
	indent int
}

func (b *goFuncBodyBuilder) writeLineWithIndent(line string, indent int) {
	for i := 0; i < indent; i++ {
		b.buf.WriteString("\t")
	}
	b.buf.WriteString(line + "\n")
}

func (b *goFuncBodyBuilder) Line(line string) *goFuncBodyBuilder {
	b.writeLineWithIndent(line, b.indent)
	return b
}

func (b *goFuncBodyBuilder) Blank() *goFuncBodyBuilder {
	b.buf.WriteString("\n")
	return b
}

func (b *goFuncBodyBuilder) Lines(lines ...string) *goFuncBodyBuilder {
	for _, line := range lines {
		b.Line(line)
	}
	return b
}

func (b *goFuncBodyBuilder) Block(header string, configure func(*goFuncBodyBuilder)) *goFuncBodyBuilder {
	b.Line(header + " {")
	child := &goFuncBodyBuilder{
		buf:    b.buf,
		indent: b.indent + 1,
	}
	configure(child)
	b.Line("}")
	return b
}

func (b *goFuncBodyBuilder) If(condition string, then func(*goFuncBodyBuilder)) *goFuncBodyBuilder {
	return b.Block("if "+condition, then)
}

func (b *goFuncBodyBuilder) IfElse(condition string, then, otherwise func(*goFuncBodyBuilder)) *goFuncBodyBuilder {
	b.Line("if " + condition + " {")
	then(&goFuncBodyBuilder{
		buf:    b.buf,
		indent: b.indent + 1,
	})
	b.Line("} else {")
	otherwise(&goFuncBodyBuilder{
		buf:    b.buf,
		indent: b.indent + 1,
	})
	b.Line("}")
	return b
}

func (b *goFuncBodyBuilder) Choose(condition bool, whenTrue, whenFalse func(*goFuncBodyBuilder)) *goFuncBodyBuilder {
	if condition {
		whenTrue(b)
		return b
	}
	whenFalse(b)
	return b
}

func (d *goDefinitionsBuilder) WithFunction(signature string, configure func(*goFuncBodyBuilder)) *goDefinitionsBuilder {
	d.file.buf.WriteString(signature + " {\n")
	body := &goFuncBodyBuilder{
		buf:    &d.file.buf,
		indent: 1,
	}
	configure(body)
	d.file.buf.WriteString("}\n\n")
	return d
}

func (d *goDefinitionsBuilder) WithRaw(code string) *goDefinitionsBuilder {
	d.file.buf.WriteString(code)
	return d
}

func (f *goFileBuilder) Buffer() *strings.Builder {
	return &f.buf
}

func (f *goFileBuilder) String() string {
	return f.buf.String()
}

// generateInstanceBuilderCode generates the Go code for an instance builder
//
//nolint:unparam // Codegen helpers currently return an always-nil error for forward compatibility.
func generateInstanceBuilderCode(spec *objects.Spec, ontology string, fieldOrder []string, packageName, enumImportBase string) (string, error) {
	// Type name (e.g., "PolicyInstanceBuilder" from "policy")
	typeName := toCamelCase(ontology) + "InstanceBuilder"
	constructorName := "New" + typeName

	enumImportAlias := "enumv"
	enumImportPath := fmt.Sprintf("%s/%s", enumImportBase, sanitizePackageName(ontology))
	enumFields := collectEnumFieldNames(spec)

	file := NewGoFile(fmt.Sprintf("%s_instance_builder.go", ontology)).
		WithHeader(generatedHeaderLines("object spec YAML")...).
		WithPackage(packageName).
		WithImports(func(i *goImportBuilder) {
			i.Include("github.com/zqk-os/zqk/pkg/objects").
				Include("github.com/zqk-os/zqk/pkg/specbuilder/instance_builders")
			if len(enumFields) > 0 {
				i.IncludeWithAlias(enumImportAlias, enumImportPath)
			}
		})

	defs := file.WithDefinitions()
	defs.WithComment(fmt.Sprintf("%s builds %s instances", typeName, ontology)).
		WithComment(fmt.Sprintf("ONE builder handles ALL %s instances", ontology)).
		WithStruct(typeName, "*instance_builders.BaseInstanceBuilder")
	defs.WithComment(fmt.Sprintf("%s creates a new %s instance builder.", constructorName, ontology)).
		WithComment("Pass the instance schema_version string for this spec; use objects.DefaultSchemaVersion for the project default (pkg/objects constants).").
		WithComment("Field order is derived from the spec (not hardcoded) to avoid drift").
		WithFunction(
			fmt.Sprintf("func %s(schemaVersion string) *%s", constructorName, typeName),
			func(fn *goFuncBodyBuilder) {
				fn.Line("// Derive canonical field order from spec (follows architecture pattern)").
					Line("// System fields first, then spec-defined fields in sorted order").
					Line(fmt.Sprintf("fieldOrder := instance_builders.FieldOrderFromSpec(%q)", ontology)).
					Blank().
					Line("// Create base builder").
					Line(fmt.Sprintf("builder := &%s{", typeName)).
					Line("	BaseInstanceBuilder: instance_builders.NewBaseInstanceBuilder(").
					Line(fmt.Sprintf("		%q,", ontology)).
					Line("		schemaVersion,").
					Line("		fieldOrder,").
					Line("		nil, // spec builder (optional)").
					Line("	),").
					Line("}").
					Blank().
					Line("return builder")
			},
		)
	buf := file.Buffer()

	// Override embedded base convenience methods using generated enum-aware signatures.
	// Keep behavior in generated builders and enforce fluent short style.
	if _, ok := enumFields[objects.FieldKeyStatus]; ok {
		defs.WithRaw(renderFluentSetter(typeName, "Status", enumImportAlias+".Status", "status", fieldKeyExpr("status"), true))
	}
	defs.WithRaw(renderFluentSetter(typeName, "ID", "string", "id", fieldKeyExpr("id"), false))

	// Generate fluent API methods for spec-defined fields
	generatedMethods := make(map[string]bool)
	fields := specFieldMap(spec)

	// Generate methods for all spec fields (excluding system fields)
	fieldNames := make([]string, 0, len(fields))
	for fieldName := range fields {
		if !isSystemField(fieldName) {
			fieldNames = append(fieldNames, fieldName)
		}
	}
	sort.Strings(fieldNames)

	for _, fieldName := range fieldNames {
		methodName := toCamelCase(fieldName)
		// Skip methods that would conflict with base builder methods
		if methodName == "Field" || methodName == "ID" || methodName == "Status" {
			continue
		}
		if !generatedMethods[methodName] {
			fieldDef := fields[fieldName]
			fieldType := inferFieldType(fieldDef)
			if _, ok := enumFields[fieldName]; ok {
				enumType := enumTypeForField(fieldName)
				fieldType = fmt.Sprintf("%s.%s", enumImportAlias, enumType)
			}
			generateSetterMethod(buf, fieldName, methodName, fieldType, typeName)
			generatedMethods[methodName] = true
		}
	}

	// Callers construct with NewForKind. Generated files do not register on the kernel singleton.
	return file.String(), nil
}

// generateSetterMethod generates a fluent setter method
func generateSetterMethod(buf *strings.Builder, fieldName, methodName, fieldType, typeName string) {
	buf.WriteString(renderFluentSetter(
		typeName,
		methodName,
		fieldType,
		fieldName,
		fieldKeyExpr(fieldName),
		strings.HasPrefix(fieldType, "enumv."),
	))
	legacy := "Set" + methodName
	buf.WriteString(renderCompatAlias(typeName, legacy, methodName, fieldType))
}

func renderFluentSetter(typeName, methodName, valueType, fieldName, fieldKeyExpr string, enumStringCast bool) string {
	file := NewGoFile("")
	defs := file.WithDefinitions()
	defs.WithComment(fmt.Sprintf("%s sets the %s field", methodName, fieldName)).
		WithFunction(
			fmt.Sprintf("func (b *%s) %s(value %s) *%s", typeName, methodName, valueType, typeName),
			func(fn *goFuncBodyBuilder) {
				fn.Choose(
					enumStringCast,
					func(b *goFuncBodyBuilder) {
						b.Line(fmt.Sprintf("b.SetField(%s, string(value))", fieldKeyExpr))
					},
					func(b *goFuncBodyBuilder) {
						b.Line(fmt.Sprintf("b.SetField(%s, value)", fieldKeyExpr))
					},
				)
				fn.Line("return b")
			},
		)
	return file.String()
}

func fieldKeyExpr(fieldName string) string {
	if suffix := objects.FieldKeyConstName(fieldName); suffix != emptyValue {
		return "objects.FieldKey" + suffix
	}
	return fmt.Sprintf("%q", fieldName)
}

func renderCompatAlias(typeName, aliasName, canonicalName, valueType string) string {
	file := NewGoFile("")
	defs := file.WithDefinitions()
	defs.WithComment(fmt.Sprintf("%s is a compatibility alias for %s.", aliasName, canonicalName)).
		WithFunction(
			fmt.Sprintf("func (b *%s) %s(value %s) *%s", typeName, aliasName, valueType, typeName),
			func(fn *goFuncBodyBuilder) {
				fn.Line(fmt.Sprintf("return b.%s(value)", canonicalName))
			},
		)
	return file.String()
}

// inferFieldType infers the Go type from a field definition
func inferFieldType(fieldDef any) string {
	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return "any" // Default to any if can't determine
	}

	fieldType, ok := fieldMap[objects.FieldKeyType].(string)
	if !ok {
		return "any"
	}

	// Map YAML types to Go types
	switch fieldType {
	case "string":
		return "string"
	case "integer", "int":
		return "int"
	case "boolean", "bool":
		return "bool"
	case "array", "list":
		// For now, use []string as default (can be enhanced)
		return "[]string"
	case "object", "map":
		return "map[string]any"
	default:
		return "any"
	}
}

func collectEnumFieldNames(spec *objects.Spec) map[string]struct{} {
	out := map[string]struct{}{}
	lifecycle, err := objects.GetGlobalLifecycleLoader().LoadLifecycle(spec.Ontology)
	if err == nil && lifecycle != nil && len(lifecycle.Statuses) > 0 {
		out[objects.FieldKeyStatus] = struct{}{}
	}
	fields := specFieldMap(spec)
	fieldNames := make([]string, 0, len(fields))
	for fieldName := range fields {
		fieldNames = append(fieldNames, fieldName)
	}
	for _, fieldName := range fieldNames {
		raw := fields[fieldName]
		fieldMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fieldType, _ := fieldMap[objects.FieldKeyType].(string)
		if fieldType != "enum" {
			continue
		}
		validation, _ := fieldMap["validation"].(map[string]any)
		if _, ok := validation["enum"].([]any); ok {
			out[fieldName] = struct{}{}
		}
	}
	return out
}

func specFieldMap(spec *objects.Spec) map[string]any {
	if spec == nil {
		return map[string]any{}
	}
	if len(spec.ResolvedFields) > 0 {
		return spec.ResolvedFields
	}
	if spec.Fields != nil {
		return spec.Fields
	}
	return map[string]any{}
}

// toCamelCase converts snake_case to CamelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, part := range parts {
		if part == emptyValue {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}
