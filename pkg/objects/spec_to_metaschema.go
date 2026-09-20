package objects

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/dna"
)

// ToMetaSchema converts an objects.Spec into a canonical dna.MetaSchema representing
// the structural shape and behavioral contracts of this kind in the cellular membrane.
func (s *Spec) ToMetaSchema() *dna.MetaSchema {
	if s == nil {
		return nil
	}

	fieldSpecs := make(map[string]dna.FieldMetaSpec)
	requiredFields := make([]string, 0)

	// Populate field rules from resolved fields or direct fields
	fields := s.ResolvedFields
	if fields == nil {
		fields = s.Fields
	}

	for fieldName, rawDef := range fields {
		rule := dna.FieldMetaSpec{
			Name: fieldName,
			Type: dna.TypeString,
		}

		if defMap, ok := rawDef.(map[string]any); ok {
			if t, ok := defMap["type"].(string); ok {
				switch strings.ToLower(t) {
				case "string", "text":
					rule.Type = dna.TypeString
				case "int", "integer":
					rule.Type = dna.TypeInt
				case "bool", "boolean":
					rule.Type = dna.TypeBool
				case "list", "array":
					rule.Type = dna.TypeList
				case "map":
					rule.Type = dna.TypeMap
				case "urn":
					rule.Type = dna.TypeURN
				case "object":
					rule.Type = dna.TypeObject
				case "enum":
					rule.Type = dna.TypeEnum
				}
			}

			if valMap, ok := defMap["validation"].(map[string]any); ok {
				if req, ok := valMap["required"].(bool); ok && req {
					rule.Required = true
					rule.Validation.Required = true
					requiredFields = append(requiredFields, fieldName)
				}
				if minL, ok := valMap["min_length"].(int); ok {
					rule.MinLength = minL
					rule.Validation.MinLength = minL
				}
				if maxL, ok := valMap["max_length"].(int); ok {
					rule.MaxLength = maxL
					rule.Validation.MaxLength = maxL
				}
				if pat, ok := valMap["pattern"].(string); ok {
					rule.Pattern = pat
					rule.Validation.Pattern = pat
				}
				if enumVals, ok := valMap["enum"].([]any); ok {
					for _, ev := range enumVals {
						if sVal, ok := ev.(string); ok {
							rule.Enum = append(rule.Enum, sVal)
							rule.Validation.Enum = append(rule.Validation.Enum, sVal)
						}
					}
				}
			}

			// Default value extraction from checklist if present
			if chkMap, ok := defMap["checklist"].(map[string]any); ok {
				if defVal, ok := chkMap["default"]; ok {
					rule.Default = defVal
				}
			}

			// Relation parsing
			if relMap, ok := defMap["relation"].(map[string]any); ok {
				var rel dna.RelationSpec
				if tk, ok := relMap["target_kind"].(string); ok {
					rel.TargetKind = tk
				}
				if et, ok := relMap["edge_type"].(string); ok {
					rel.EdgeType = et
				}
				if cas, ok := relMap["cascade"].(string); ok {
					rel.Cascade = cas
				}
				if bidi, ok := relMap["bidirectional"].(bool); ok {
					rel.Bidirectional = bidi
				}
				if inv, ok := relMap["inverse_field"].(string); ok {
					rel.InverseField = inv
				}
				rule.Relation = &rel
			}

			if code, ok := defMap["field_profile_code"].(string); ok {
				rule.FieldProfileCode = code
			}
			if semType, ok := defMap["semantic_type"].(string); ok {
				rule.SemanticType = semType
			}
			if trList, ok := defMap["traits"].([]any); ok {
				for _, tr := range trList {
					if trStr, ok := tr.(string); ok {
						rule.Traits = append(rule.Traits, trStr)
					}
				}
			}

			if len(rule.Validation.Enum) > 0 {
				rule.EnumSpec = &dna.Enum{
					Name:   fieldName,
					Values: rule.Validation.Enum,
				}
			}

			if desc, ok := defMap["description"].(string); ok {
				rule.Description = desc
			}
		}

		fieldSpecs[fieldName] = rule
	}

	traits := s.ResolvedTraits
	if traits == nil {
		traits = s.Traits
	}

	meta := dna.NewMetaSchema(
		s.Ontology,
		s.SchemaVersion,
		[]dna.Plane{dna.PlaneDraft, dna.PlaneStaged, dna.PlanePromoted, dna.PlaneApoptotic},
		requiredFields,
	)

	meta.Extends = s.Extends
	meta.Composes = s.Composes
	meta.Traits = traits
	meta.StorageProfile = dna.StorageProfile(s.StorageProfile)
	if s.KernelCritical != nil {
		meta.KernelCritical = *s.KernelCritical
	}
	meta.Fields = fieldSpecs

	return meta
}
