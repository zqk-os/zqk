package instance_builders

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

const enumModuleBasePath = "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1"

const (
	enumCodegenFieldTypeEnum      = "enum"
	enumCodegenFieldNameStatus    = "status"
	enumCodegenTypeNameStatus     = "Status"
	enumCodegenFieldKeyValidation = "validation"
	enumCodegenFieldKeyEnum       = "enum"
	enumCodegenFileExtYAML        = ".yaml"
	enumCodegenFileExtYML         = ".yml"
)

type enumSpec struct {
	TypeName       string
	FieldName      string
	Values         []string
	IsLifecycle    bool
	AliasPackage   string
	AliasTypeName  string
	AliasImportAs  string
	AliasImportPkg string
	AliasValues    map[string]struct{}
}

type sharedStatusPackage struct {
	Alias     string
	ImportPkg string
	Values    map[string]struct{}
}

func generateEnumsForSpec(specPath, outputDir string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return errfmt.Newf("failed to read spec file").Wrap(err)
	}

	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return errfmt.Newf("failed to parse spec").Wrap(err)
	}

	ontology := spec.Ontology
	if ontology == emptyValue {
		ontology = ontologyFromSpecPath(specPath)
	}
	if ontology == emptyValue {
		return errfmt.Errorf("ontology is empty for spec %s", specPath)
	}

	specForGeneration := &spec
	if resolvedSpec, err := loadSpecWithInheritanceByOntology(ontology); err == nil && resolvedSpec != nil {
		specForGeneration = resolvedSpec
		if specForGeneration.Ontology == emptyValue {
			specForGeneration.Ontology = ontology
		}
	}

	enumOwners, err := buildEnumOwnerMap(specPath, ontology)
	if err != nil {
		return errfmt.Newf("failed to build enum owner map").Wrap(err)
	}

	sharedStatus, err := ensureDomainSharedStatusPackage(specPath, outputDir, ontology)
	if err != nil {
		return errfmt.Newf("failed to generate domain shared status package").Wrap(err)
	}

	defs := collectEnumDefinitions(specForGeneration, ontology, enumOwners, sharedStatus)
	if len(defs) == 0 {
		return nil
	}

	code := generateEnumPackageCode(ontology, defs)

	formatted, err := format.Source([]byte(code))
	if err != nil {
		formatted = []byte(code)
	}

	parentDir := filepath.Dir(outputDir)
	enumRoot := filepath.Join(parentDir, "bldr_enum_v1")
	enumDir := filepath.Join(enumRoot, sanitizePackageName(ontology))
	if err := os.MkdirAll(enumDir, defaultDirectoryPerm); err != nil {
		return errfmt.Newf("failed to create enum package dir").Wrap(err)
	}

	outPath := filepath.Join(enumDir, "enums_generated.go")
	if err := os.WriteFile(outPath, formatted, defaultFilePerm); err != nil {
		return errfmt.Newf("failed to write enum package").Wrap(err)
	}
	return nil
}

func collectEnumDefinitions(spec *objects.Spec, ontology string, enumOwners map[string]string, sharedStatus *sharedStatusPackage) []enumSpec {
	defMap := map[string]enumSpec{}

	lifecycle, err := objects.GetGlobalLifecycleLoader().LoadLifecycle(ontology)
	if err == nil && lifecycle != nil && len(lifecycle.Statuses) > 0 {
		values := make([]string, 0, len(lifecycle.Statuses))
		for _, st := range lifecycle.Statuses {
			if st.Value != emptyValue {
				values = append(values, st.Value)
			}
		}
		if len(values) > 0 {
			statusDef := enumSpec{
				TypeName:    enumCodegenTypeNameStatus,
				FieldName:   enumCodegenFieldNameStatus,
				Values:      uniqueSorted(values),
				IsLifecycle: true,
			}
			if sharedStatus != nil {
				statusDef.AliasTypeName = enumCodegenTypeNameStatus
				statusDef.AliasImportAs = sharedStatus.Alias
				statusDef.AliasImportPkg = sharedStatus.ImportPkg
				statusDef.AliasValues = sharedStatus.Values
			} else if owner, ownerValues := findStatusAliasOwner(spec, ontology); owner != emptyValue && owner != ontology {
				statusDef.AliasPackage = owner
				statusDef.AliasTypeName = enumCodegenTypeNameStatus
				statusDef.AliasImportAs = sanitizePackageName(owner) + "enum"
				statusDef.AliasImportPkg = fmt.Sprintf("%s/%s", enumModuleBasePath, sanitizePackageName(owner))
				statusDef.AliasValues = ownerValues
			}
			defMap[enumCodegenTypeNameStatus] = statusDef
		}
	}

	for fieldName, raw := range specFieldMap(spec) {
		fieldMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fieldType, _ := fieldMap[objects.FieldKeyType].(string)
		if fieldType != enumCodegenFieldTypeEnum {
			continue
		}
		validation, _ := fieldMap[enumCodegenFieldKeyValidation].(map[string]any)
		enumRaw, ok := validation[enumCodegenFieldKeyEnum].([]any)
		if !ok || len(enumRaw) == 0 {
			continue
		}
		values := make([]string, 0, len(enumRaw))
		for _, v := range enumRaw {
			if s, ok := v.(string); ok && s != emptyValue {
				values = append(values, s)
			}
		}
		if len(values) == 0 {
			continue
		}
		typeName := toCamelCase(fieldName)
		owner := enumOwners[fieldName]
		def := enumSpec{
			TypeName:  typeName,
			FieldName: fieldName,
			Values:    uniqueSorted(values),
		}
		if owner != emptyValue && owner != ontology {
			alias := sanitizePackageName(owner) + "enum"
			def.AliasPackage = owner
			def.AliasTypeName = typeName
			def.AliasImportAs = alias
			def.AliasImportPkg = fmt.Sprintf("%s/%s", enumModuleBasePath, sanitizePackageName(owner))
			def.AliasValues = enumValuesForField(owner, fieldName)
		}
		defMap[typeName] = def
	}

	if len(defMap) == 0 {
		return nil
	}
	keys := make([]string, 0, len(defMap))
	for k := range defMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]enumSpec, 0, len(keys))
	for _, k := range keys {
		out = append(out, defMap[k])
	}
	return out
}

func generateEnumPackageCode(ontology string, defs []enumSpec) string {
	var b strings.Builder
	pkgName := sanitizePackageName(ontology)
	for _, line := range generatedHeaderLines("object spec/lifecycle YAML") {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "package %s\n\n", pkgName)

	imports := collectAliasImports(defs)
	if len(imports) > 0 {
		b.WriteString("import (\n")
		keys := make([]string, 0, len(imports))
		for alias := range imports {
			keys = append(keys, alias)
		}
		sort.Strings(keys)
		for _, alias := range keys {
			fmt.Fprintf(&b, "\t%s %q\n", alias, imports[alias])
		}
		b.WriteString(")\n\n")
	}

	for _, def := range defs {
		if def.AliasImportAs != emptyValue {
			fmt.Fprintf(&b, "type %s = %s.%s\n\n", def.TypeName, def.AliasImportAs, def.AliasTypeName)
		} else {
			fmt.Fprintf(&b, "type %s string\n\n", def.TypeName)
		}
		b.WriteString("const (\n")
		for _, v := range def.Values {
			if def.AliasImportAs != emptyValue && aliasHasValue(def.AliasValues, v) {
				fmt.Fprintf(&b, "\t%s%s %s = %s.%s%s\n", def.TypeName, toConstToken(v), def.TypeName, def.AliasImportAs, def.TypeName, toConstToken(v))
			} else {
				fmt.Fprintf(&b, "\t%s%s %s = %q\n", def.TypeName, toConstToken(v), def.TypeName, v)
			}
		}
		b.WriteString(")\n\n")
	}
	return b.String()
}

func collectAliasImports(defs []enumSpec) map[string]string {
	out := map[string]string{}
	for _, def := range defs {
		if def.AliasImportAs != emptyValue && def.AliasImportPkg != emptyValue {
			out[def.AliasImportAs] = def.AliasImportPkg
		}
	}
	return out
}

func ensureDomainSharedStatusPackage(specPath, outputDir, ontology string) (*sharedStatusPackage, error) {
	domain := objects.GetDirectoryFromKind(ontology)
	if domain == emptyValue {
		return nil, nil
	}
	statuses, err := collectDomainStatuses(specPath, domain)
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, nil
	}

	pkgSegment := "shared_" + sanitizePackageName(domain)
	file := NewGoFile("")
	defs := file.WithDefinitions()
	file.WithHeader(generatedHeaderLines("lifecycle YAML")...).WithPackage(pkgSegment)

	values := make([]string, 0, len(statuses))
	for v := range statuses {
		values = append(values, v)
	}
	sort.Strings(values)
	defs.WithRaw(fmt.Sprintf("type %s string\n\nconst (\n", enumCodegenTypeNameStatus))
	for _, v := range values {
		defs.WithRaw(fmt.Sprintf("\t%s%s %s = %q\n", enumCodegenTypeNameStatus, toConstToken(v), enumCodegenTypeNameStatus, v))
	}
	defs.WithRaw(")\n")

	formatted, err := format.Source([]byte(file.String()))
	if err != nil {
		formatted = []byte(file.String())
	}
	parentDir := filepath.Dir(outputDir)
	enumRoot := filepath.Join(parentDir, "bldr_enum_v1")
	outDir := filepath.Join(enumRoot, pkgSegment)
	if err := os.MkdirAll(outDir, defaultDirectoryPerm); err != nil {
		return nil, errfmt.Newf("create shared status dir").Wrap(err)
	}
	outPath := filepath.Join(outDir, "status_enums_generated.go")
	if err := os.WriteFile(outPath, formatted, defaultFilePerm); err != nil {
		return nil, errfmt.Newf("write shared status file").Wrap(err)
	}

	return &sharedStatusPackage{
		Alias:     pkgSegment + "enum",
		ImportPkg: fmt.Sprintf("%s/%s", enumModuleBasePath, pkgSegment),
		Values:    statuses,
	}, nil
}

func collectDomainStatuses(specPath, domain string) (map[string]struct{}, error) {
	specsDir := filepath.Dir(specPath)
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, errfmt.Newf("read specs dir").Wrap(err)
	}
	out := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), enumCodegenFileExtYAML) || appledouble.SkipNameInReadDir(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specsDir, entry.Name()))
		if err != nil {
			continue
		}
		var spec objects.Spec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		if spec.Ontology == emptyValue {
			spec.Ontology = strings.TrimSuffix(entry.Name(), enumCodegenFileExtYAML)
		}
		if objects.GetDirectoryFromKind(spec.Ontology) != domain {
			continue
		}
		lifecycle, err := objects.GetGlobalLifecycleLoader().LoadLifecycle(spec.Ontology)
		if err != nil || lifecycle == nil {
			continue
		}
		for _, st := range lifecycle.Statuses {
			if st.Value != emptyValue {
				out[st.Value] = struct{}{}
			}
		}
	}
	return out, nil
}

func buildEnumOwnerMap(specPath, ontology string) (map[string]string, error) {
	owners := map[string]string{}
	chain, err := loadSpecInheritanceChain(specPath)
	if err != nil {
		return nil, err
	}
	for _, s := range chain {
		for fieldName, raw := range s.Fields {
			fieldMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			fieldType, _ := fieldMap[objects.FieldKeyType].(string)
			if fieldType != enumCodegenFieldTypeEnum {
				continue
			}
			if _, exists := owners[fieldName]; !exists {
				owners[fieldName] = s.Ontology
			}
		}
	}
	if _, ok := owners[enumCodegenFieldNameStatus]; !ok {
		owners[enumCodegenFieldNameStatus] = ontology
	}
	return owners, nil
}

func loadSpecInheritanceChain(startPath string) ([]*objects.Spec, error) {
	visited := map[string]bool{}
	chain := make([]*objects.Spec, 0, 4)
	currentPath := startPath
	for {
		data, err := os.ReadFile(currentPath)
		if err != nil {
			return nil, errfmt.Errorf("read spec %s: %w", currentPath, err)
		}
		var spec objects.Spec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, errfmt.Errorf("parse spec %s: %w", currentPath, err)
		}
		if spec.Ontology == emptyValue {
			spec.Ontology = ontologyFromSpecPath(currentPath)
		}
		if visited[spec.Ontology] {
			return nil, errfmt.Errorf("circular inheritance while loading %s", spec.Ontology)
		}
		visited[spec.Ontology] = true
		chain = append(chain, &spec)

		if spec.Extends == emptyValue || spec.Extends == "null" {
			break
		}
		currentPath = filepath.Join(filepath.Dir(currentPath), specFilenameFromOntology(spec.Extends))
	}
	return chain, nil
}

func findStatusAliasOwner(spec *objects.Spec, ontology string) (string, map[string]struct{}) {
	if spec == nil {
		return "", nil
	}
	parent := spec.Extends
	visited := map[string]bool{}
	for parent != emptyValue && parent != "null" {
		if visited[parent] {
			break
		}
		visited[parent] = true
		lifecycle, err := objects.GetGlobalLifecycleLoader().LoadLifecycle(parent)
		if err == nil && lifecycle != nil && len(lifecycle.Statuses) > 0 {
			return parent, lifecycleStatusValueSet(lifecycle.Statuses)
		}
		parentSpec, err := loadSpecWithInheritanceByOntology(parent)
		if err != nil || parentSpec == nil {
			break
		}
		parent = parentSpec.Extends
	}
	_ = ontology
	return "", nil
}

func lifecycleStatusValueSet(statuses []objects.Status) map[string]struct{} {
	out := map[string]struct{}{}
	for _, st := range statuses {
		if st.Value != emptyValue {
			out[st.Value] = struct{}{}
		}
	}
	return out
}

func enumValuesForField(ontology, fieldName string) map[string]struct{} {
	spec, err := loadSpecWithInheritanceByOntology(ontology)
	if err != nil || spec == nil {
		return nil
	}
	raw, ok := specFieldMap(spec)[fieldName]
	if !ok {
		return nil
	}
	fieldMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	validation, _ := fieldMap[enumCodegenFieldKeyValidation].(map[string]any)
	enumRaw, ok := validation[enumCodegenFieldKeyEnum].([]any)
	if !ok {
		return nil
	}
	out := map[string]struct{}{}
	for _, v := range enumRaw {
		if s, ok := v.(string); ok && s != emptyValue {
			out[s] = struct{}{}
		}
	}
	return out
}

func aliasHasValue(values map[string]struct{}, v string) bool {
	if len(values) == 0 {
		return false
	}
	_, ok := values[v]
	return ok
}

func sanitizePackageName(s string) string {
	if s == emptyValue {
		return "unknown"
	}
	var b strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			if i == 0 && r >= '0' && r <= '9' {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			continue
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == emptyValue {
		return "unknown"
	}
	return out
}

func toConstToken(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(parts) == 0 {
		return "Value"
	}
	var b strings.Builder
	for _, p := range parts {
		if p == emptyValue {
			continue
		}
		if p[0] >= '0' && p[0] <= '9' {
			b.WriteString("N")
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(strings.ToLower(p[1:]))
		}
	}
	if b.Len() == 0 {
		return "Value"
	}
	return b.String()
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func enumTypeForField(fieldName string) string {
	if fieldName == enumCodegenFieldNameStatus {
		return enumCodegenTypeNameStatus
	}
	return toCamelCase(fieldName)
}

func specFilenameFromOntology(ontology string) string {
	return ontology + enumCodegenFileExtYAML
}

func ontologyFromSpecPath(specPath string) string {
	baseName := filepath.Base(specPath)
	return strings.TrimSuffix(strings.TrimSuffix(baseName, enumCodegenFileExtYAML), enumCodegenFileExtYML)
}

func loadSpecWithInheritanceByOntology(ontology string) (*objects.Spec, error) {
	return objects.GetGlobalSpecLoader().LoadSpecWithInheritance(specFilenameFromOntology(ontology))
}
