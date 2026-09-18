package macro

import (
	"strings"

	"github.com/zqk-os/zqk/internal/codegen/generators"
	"github.com/zqk-os/zqk/pkg/dna"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// SchemaMacroExpander expands dynamic schema definitions into cellular entity specifications.
type SchemaMacroExpander struct{}

// NewSchemaMacroExpander creates a new SchemaMacroExpander instance.
func NewSchemaMacroExpander() *SchemaMacroExpander {
	return &SchemaMacroExpander{}
}

// ExpandMetaSchema converts a dna.MetaSchema into an EntitySpec suitable for code generation.
func (sme *SchemaMacroExpander) ExpandMetaSchema(pkgName string, schema *dna.MetaSchema) (*generators.EntitySpec, error) {
	if schema == nil {
		return nil, errfmt.Errorf("schema cannot be nil")
	}
	if strings.TrimSpace(schema.TargetKind) == "" {
		return nil, errfmt.Errorf("schema target kind is required")
	}

	entityName := toPascalCase(schema.TargetKind)
	var fields []generators.FieldDefinition

	for fieldName, rule := range schema.Fields {
		goType := "string"
		switch rule.Type {
		case dna.TypeInt:
			goType = "int64"
		case dna.TypeBool:
			goType = "bool"
		case dna.TypeList:
			goType = "[]string"
		case dna.TypeMap:
			goType = "map[string]any"
		case dna.TypeURN:
			goType = "dna.URN"
		case dna.TypeString:
			goType = "string"
		}

		fields = append(fields, generators.FieldDefinition{
			Name:     toPascalCase(fieldName),
			Type:     goType,
			JSONTag:  fieldName,
			YAMLTag:  fieldName,
			Required: rule.Required,
		})
	}

	return &generators.EntitySpec{
		PackageName:      pkgName,
		EntityName:       entityName,
		Kind:             schema.TargetKind,
		SchemaRef:        "1.0.0",
		IncludeAudit:     true,
		IncludeLifecycle: true,
		Fields:           fields,
	}, nil
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[0:1]))
			result.WriteString(part[1:])
		}
	}
	return result.String()
}
