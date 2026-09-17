package processing

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kindnames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	formatOperations     = "operations"
	formatData           = "data"
	formatTemplate       = kindnames.Template
	emptyRefFormatValue  = ""
	errReadReferenceFmt  = "failed to read reference file: %w"
	errParseReferenceFmt = "failed to parse reference file: %w"
	errUnknownFormat     = "reference file format could not be determined"
	errExpandTemplateFmt = "failed to expand template: %w"
	varPrefix            = "${"
	varSuffix            = "}"
	minVariableLen       = 3
)

func isTemplateVariable(value string) bool {
	return strings.HasPrefix(value, varPrefix) &&
		strings.HasSuffix(value, varSuffix) &&
		len(value) > minVariableLen
}

// ReferenceFile represents a reference file that contains data to be processed
type ReferenceFile struct {
	// Format specifies the format of the reference file
	Format string `yaml:"format"` // "operations", "data", "template"

	// Operations is a list of operations to perform (format: "operations")
	Operations []Operation `yaml:"operations,omitempty"`

	// Data is a list of data items to process (format: "data")
	Data []map[string]any `yaml:"data,omitempty"`

	// Template is a template with variables to be filled (format: "template")
	Template *Template `yaml:"template,omitempty"`

	// Metadata about the reference file
	Metadata map[string]any `yaml:"metadata,omitempty"`
}

// Operation represents a single operation to perform
type Operation struct {
	// Type is the operation type: "create", "update", "delete", "get", "list"
	Type string `yaml:"type"`

	// Kind is the object kind (required for create, update, list)
	Kind string `yaml:"kind,omitempty"`

	// ID is the object ID (required for update, delete, get)
	ID string `yaml:"id,omitempty"`

	// Object is the object data (for create, update)
	Object map[string]any `yaml:"object,omitempty"`

	// Updates is partial update data (for update)
	Updates map[string]any `yaml:"updates,omitempty"`

	// Filter is a filter for list operations
	Filter map[string]any `yaml:"filter,omitempty"`

	// ContinueOnError determines if processing should continue on error
	ContinueOnError bool `yaml:"continue_on_error,omitempty"`

	// Description is a human-readable description of the operation
	Description string `yaml:"description,omitempty"`
}

// Template represents a template with variables
type Template struct {
	// Variables is a list of variable values to substitute
	Variables []map[string]any `yaml:"variables"`

	// Template is the template object structure
	Template map[string]any `yaml:"template"`
}

// LoadReferenceFile loads a reference file from disk
func LoadReferenceFile(filePath string) (*ReferenceFile, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Errorf(errReadReferenceFmt, err)
	}

	var refFile ReferenceFile
	if err := yaml.Unmarshal(data, &refFile); err != nil {
		return nil, errfmt.Errorf(errParseReferenceFmt, err)
	}

	// Validate format
	if refFile.Format == emptyRefFormatValue {
		// Try to infer format
		if len(refFile.Operations) > 0 {
			refFile.Format = formatOperations
		} else if len(refFile.Data) > 0 {
			refFile.Format = formatData
		} else if refFile.Template != nil {
			refFile.Format = formatTemplate
		} else {
			return nil, errors.New(errUnknownFormat)
		}
	}

	return &refFile, nil
}

// ExpandTemplate expands a template with variables
func (t *Template) ExpandTemplate() ([]map[string]any, error) {
	var results []map[string]any

	for _, vars := range t.Variables {
		expanded := make(map[string]any)

		// Deep copy template
		if err := deepCopyMap(t.Template, expanded, vars); err != nil {
			return nil, errfmt.Errorf(errExpandTemplateFmt, err)
		}

		results = append(results, expanded)
	}

	return results, nil
}

// deepCopyMap performs a deep copy with variable substitution
func deepCopyMap(src map[string]any, dst map[string]any, vars map[string]any) error {
	for k, v := range src {
		switch val := v.(type) {
		case string:
			// Check if it's a variable reference (e.g., "${var_name}")
			if isTemplateVariable(val) {
				varName := val[len(varPrefix) : len(val)-len(varSuffix)]
				if varValue, ok := vars[varName]; ok {
					dst[k] = varValue
				} else {
					dst[k] = val // Keep original if variable not found
				}
			} else {
				dst[k] = val
			}
		case map[string]any:
			subMap := make(map[string]any)
			if err := deepCopyMap(val, subMap, vars); err != nil {
				return err
			}
			dst[k] = subMap
		case []any:
			subList := make([]any, len(val))
			for i, item := range val {
				switch itemVal := item.(type) {
				case string:
					if isTemplateVariable(itemVal) {
						varName := itemVal[len(varPrefix) : len(itemVal)-len(varSuffix)]
						if varValue, ok := vars[varName]; ok {
							subList[i] = varValue
						} else {
							subList[i] = itemVal
						}
					} else {
						subList[i] = itemVal
					}
				case map[string]any:
					subMap := make(map[string]any)
					if err := deepCopyMap(itemVal, subMap, vars); err != nil {
						return err
					}
					subList[i] = subMap
				default:
					subList[i] = item
				}
			}
			dst[k] = subList
		default:
			dst[k] = v
		}
	}
	return nil
}
